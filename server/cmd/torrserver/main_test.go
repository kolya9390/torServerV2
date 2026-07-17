package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"server/internal/daemon"
)

const binarySmokeTimeout = 20 * time.Second

var (
	testBinary          string
	compatibilityBinary string
	serverRoot          string
)

func TestMain(testMain *testing.M) {
	root, err := findServerRoot()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)

		os.Exit(1)
	}

	buildDir, err := os.MkdirTemp("", "torrserver-smoke-")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "create smoke build dir: %v\n", err)

		os.Exit(1)
	}

	serverRoot = root
	testBinary = filepath.Join(buildDir, executableName("torrserver"))
	compatibilityBinary = filepath.Join(buildDir, executableName("torrserver-compat"))
	if err := buildTestBinary(root, testBinary, "./cmd/torrserver"); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "build torrserver smoke binary: %v\n", err)
		_ = os.RemoveAll(buildDir)

		os.Exit(1)
	}

	if err := buildTestBinary(root, compatibilityBinary, "./cmd"); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "build compatibility smoke binary: %v\n", err)
		_ = os.RemoveAll(buildDir)

		os.Exit(1)
	}

	exitCode := testMain.Run()
	_ = os.RemoveAll(buildDir)
	os.Exit(exitCode)
}

func TestExecuteDelegatesToDaemonRunner(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), contextKey{}, "test")
	args := []string{"serve", "--port", "18090"}
	dependencies := daemon.Dependencies{StopTimeout: 7 * time.Second}
	var (
		stdout   bytes.Buffer
		stderr   bytes.Buffer
		captured daemon.Invocation
	)
	expectedErr := errors.New("runner failed")

	exitCode := execute(ctx, args, &stdout, &stderr, dependencies, func(
		invocation daemon.Invocation,
		deps daemon.Dependencies,
	) daemon.Result {
		captured = invocation
		if deps.StopTimeout != dependencies.StopTimeout {
			t.Fatalf("stop timeout = %v, want %v", deps.StopTimeout, dependencies.StopTimeout)
		}

		return daemon.Result{ExitCode: daemon.ExitFailure, Err: expectedErr}
	})

	if exitCode != daemon.ExitFailure || captured.Context != ctx {
		t.Fatalf("delegation result = exit:%d context:%v", exitCode, captured.Context)
	}

	if strings.Join(captured.Args, " ") != strings.Join(args, " ") || captured.Stdout != &stdout {
		t.Fatalf("captured invocation = %+v", captured)
	}

	if stderr.String() != expectedErr.Error()+"\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestTorrserverHelpDocumentsCanonicalServeInvocation(t *testing.T) {
	t.Parallel()

	result := runBinary(t, "serve", "--help")
	if result.exitCode != daemon.ExitOK || result.stderr != "" {
		t.Fatalf("help result = %+v", result)
	}

	if !strings.Contains(result.stdout, "Usage: torrserver serve") {
		t.Fatalf("help output does not document serve:\n%s", result.stdout)
	}
}

func TestTorrserverVersionUsesEmbeddedMetadataWithoutStartingDaemon(t *testing.T) {
	t.Parallel()

	result := runBinary(t, "--version")
	want := fmt.Sprintf(
		"torrserver v1.0.0-beta.test (%s/%s, commit 0123456789ab)\n",
		runtime.GOOS,
		runtime.GOARCH,
	)
	if result.exitCode != daemon.ExitOK || result.stdout != want || result.stderr != "" {
		t.Fatalf("version result = %+v, want stdout %q", result, want)
	}
}

func TestTorrserverRejectsInvalidArgumentsBeforeRuntimeInitialization(t *testing.T) {
	t.Parallel()

	result := runBinary(t, "--definitely-invalid")
	if result.exitCode != daemon.ExitUsage {
		t.Fatalf("invalid argument result = %+v", result)
	}

	if result.stderr != "torrserver: invalid daemon arguments; usage: torrserver serve [flags]; "+
		"run `torrserver serve --help`\n" {
		t.Fatalf("invalid argument stderr = %q", result.stderr)
	}
}

func TestTorrserverMigratesFormerManagementCommandWithoutNetworkAccess(t *testing.T) {
	t.Parallel()

	result := runBinary(t,
		"--server=https://user:password@example.test/private",
		"--token=do-not-print",
		"shutdown",
	)
	if result.exitCode != daemon.ExitUsage || result.stdout != "" {
		t.Fatalf("migration result = %+v", result)
	}

	const want = "torrserver shutdown: management commands moved to torrctl; use `torrctl shutdown`\n"
	if result.stderr != want {
		t.Fatalf("migration stderr = %q, want %q", result.stderr, want)
	}

	if strings.Contains(result.stderr, "password") || strings.Contains(result.stderr, "do-not-print") {
		t.Fatalf("migration stderr exposes credentials: %q", result.stderr)
	}
}

func TestCompatibilityEntryPointUsesDaemonMigrationContract(t *testing.T) {
	t.Parallel()

	result := runSelectedBinary(t, compatibilityBinary, "shutdown")
	if result.exitCode != daemon.ExitUsage || result.stdout != "" {
		t.Fatalf("compatibility migration result = %+v", result)
	}

	const want = "torrserver shutdown: management commands moved to torrctl; use `torrctl shutdown`\n"
	if result.stderr != want {
		t.Fatalf("compatibility migration stderr = %q, want %q", result.stderr, want)
	}
}

func TestTorrserverReportsConfigFailureWithoutStartingEngine(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	configPath := filepath.Join(runDir, "config.yml")
	if err := os.WriteFile(configPath, []byte("server: [invalid\n"), 0o600); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}

	result := runBinaryInDir(t, testBinary, runDir, []string{"TS_CONFIG=" + configPath}, "serve")
	if result.exitCode != daemon.ExitFailure {
		t.Fatalf("config failure result = %+v", result)
	}

	if !strings.Contains(result.stderr, "load config: failed to parse config file") {
		t.Fatalf("config failure stderr = %q", result.stderr)
	}
}

func TestTorrserverDependencyGraphExcludesManagementCLI(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), binarySmokeTimeout)
	defer cancel()

	command := exec.CommandContext(ctx, "go", "list", "-deps", "-f", "{{.ImportPath}}", "./cmd/torrserver")
	command.Dir = serverRoot
	command.Env = append(os.Environ(), "GOCACHE=/private/tmp/torserverv2-gocache")

	output, err := command.Output()
	if err != nil {
		t.Fatalf("inspect torrserver dependency graph: %v", err)
	}

	for _, dependency := range strings.Fields(string(output)) {
		switch dependency {
		case "server/internal/cliapp", "server/internal/apiclient", "github.com/spf13/cobra", "golang.org/x/term":
			t.Fatalf("torrserver links management dependency %q", dependency)
		}
	}
}

type contextKey struct{}

type binaryResult struct {
	stdout   string
	stderr   string
	exitCode int
}

func runBinary(t *testing.T, args ...string) binaryResult {
	t.Helper()

	return runSelectedBinary(t, testBinary, args...)
}

func runSelectedBinary(t *testing.T, binary string, args ...string) binaryResult {
	t.Helper()

	return runBinaryInDir(t, binary, t.TempDir(), nil, args...)
}

func runBinaryInDir(t *testing.T, binary, runDir string, extraEnv []string, args ...string) binaryResult {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), binarySmokeTimeout)
	defer cancel()

	command := exec.CommandContext(ctx, binary, args...)
	command.Dir = runDir
	command.Env = append(testEnvironment(runDir), extraEnv...)

	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	err := command.Run()
	exitCode := daemon.ExitOK
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run torrserver: %v", err)
		}

		exitCode = exitErr.ExitCode()
	}

	if ctx.Err() != nil {
		t.Fatalf("run torrserver timed out: %v", ctx.Err())
	}

	return binaryResult{stdout: stdout.String(), stderr: stderr.String(), exitCode: exitCode}
}

func testEnvironment(home string) []string {
	environment := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "TS_CONFIG=") && !strings.HasPrefix(entry, "HOME=") {
			environment = append(environment, entry)
		}
	}

	return append(environment,
		"GOCACHE=/private/tmp/torserverv2-gocache",
		"HOME="+home,
		"TS_CONFIG=",
	)
}

func buildTestBinary(root, output, packagePath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), binarySmokeTimeout)
	defer cancel()

	ldflags := strings.Join([]string{
		"-X server/version.version=v1.0.0-beta.test",
		"-X server/version.commit=0123456789abcdef",
		"-X server/version.buildTime=2026-07-17T00:00:00Z",
		"-X server/version.dirtyState=false",
	}, " ")
	command := exec.CommandContext(ctx, "go", "build", "-ldflags", ldflags, "-o", output, packagePath)
	command.Dir = root
	command.Env = append(os.Environ(), "GOCACHE=/private/tmp/torserverv2-gocache")

	combined, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, combined)
	}

	return nil
}

func findServerRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("resolve smoke test source path")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..")), nil
}

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}

	return name
}
