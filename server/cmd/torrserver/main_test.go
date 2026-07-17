package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"server/internal/daemon"
	buildversion "server/version"
)

const binarySmokeTimeout = 20 * time.Second

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

	result := runDaemon(t, "", "serve", "--help")
	if result.exitCode != daemon.ExitOK || result.stderr != "" {
		t.Fatalf("help result = %+v", result)
	}

	if !strings.Contains(result.stdout, "Usage: torrserver serve") {
		t.Fatalf("help output does not document serve:\n%s", result.stdout)
	}
}

func TestTorrserverVersionUsesCanonicalMetadataWithoutStartingDaemon(t *testing.T) {
	t.Parallel()

	result := runDaemon(t, "", "--version")
	want := buildversion.Concise("torrserver", buildversion.Current()) + "\n"
	if result.exitCode != daemon.ExitOK || result.stdout != want || result.stderr != "" {
		t.Fatalf("version result = %+v, want stdout %q", result, want)
	}
}

func TestTorrserverRejectsInvalidArgumentsBeforeRuntimeInitialization(t *testing.T) {
	t.Parallel()

	result := runDaemon(t, "", "--definitely-invalid")
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

	result := runDaemon(t,
		"",
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

func TestTorrserverReportsConfigFailureWithoutStartingEngine(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	configPath := filepath.Join(runDir, "config.yml")
	if err := os.WriteFile(configPath, []byte("server: [invalid\n"), 0o600); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}

	result := runDaemon(t, configPath, "serve")
	if result.exitCode != daemon.ExitFailure {
		t.Fatalf("config failure result = %+v", result)
	}

	if !strings.Contains(result.stderr, "load config: failed to parse config file") {
		t.Fatalf("config failure stderr = %q", result.stderr)
	}
}

func TestTorrserverDependencyGraphExcludesManagementCLI(t *testing.T) {
	t.Parallel()

	serverRoot, err := findServerRoot()
	if err != nil {
		t.Fatalf("find server root: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), binarySmokeTimeout)
	defer cancel()

	command := exec.CommandContext(ctx, "go", "list", "-deps", "-f", "{{.ImportPath}}", "./cmd/torrserver")
	command.Dir = serverRoot

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

type daemonResult struct {
	stdout   string
	stderr   string
	exitCode int
}

type testLogger struct{}

func (testLogger) Init(string, string) {}

func (testLogger) Info(...any) {}

func (testLogger) Close() {}

func runDaemon(t *testing.T, configPath string, args ...string) daemonResult {
	t.Helper()

	dependencies := daemon.DefaultDependencies()
	dependencies.Getenv = func(name string) string {
		if name == "TS_CONFIG" {
			return configPath
		}

		return ""
	}
	dependencies.Logger = testLogger{}

	ctx, cancel := context.WithTimeout(context.Background(), binarySmokeTimeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	exitCode := execute(ctx, args, &stdout, &stderr, dependencies, daemon.Run)
	if ctx.Err() != nil {
		t.Fatalf("run torrserver timed out: %v", ctx.Err())
	}

	return daemonResult{stdout: stdout.String(), stderr: stderr.String(), exitCode: exitCode}
}

func findServerRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("resolve test source path")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..")), nil
}
