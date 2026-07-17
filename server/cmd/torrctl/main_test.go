package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const smokeTimeout = 10 * time.Second

var (
	torrctlBinary    string
	torrserverBinary string
	serverRoot       string
	originalGoPath   string
)

func TestMain(testMain *testing.M) {
	root, err := findServerRoot()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)

		os.Exit(1)
	}

	buildDir, err := os.MkdirTemp("", "torrctl-smoke-")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "create smoke build dir: %v\n", err)

		os.Exit(1)
	}
	serverRoot = root
	originalGoPath = resolveGoPath()
	torrctlBinary = filepath.Join(buildDir, executableName("torrctl"))
	torrserverBinary = filepath.Join(buildDir, executableName("torrserver"))

	if err := buildBinary(root, torrctlBinary, "./cmd/torrctl"); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "build torrctl smoke binary: %v\n", err)
		_ = os.RemoveAll(buildDir)

		os.Exit(1)
	}

	if err := buildBinary(root, torrserverBinary, "./cmd/torrserver"); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "build torrserver metadata binary: %v\n", err)
		_ = os.RemoveAll(buildDir)

		os.Exit(1)
	}

	exitCode := testMain.Run()
	_ = os.RemoveAll(buildDir)
	os.Exit(exitCode)
}

func TestTorrctlHelpIsClientOnly(t *testing.T) {
	t.Parallel()

	result := runBinary(t, torrctlBinary, "--help")
	if result.exitCode != 0 || result.stderr != "" {
		t.Fatalf("help result = %+v", result)
	}

	for _, required := range []string{"Usage:\n  torrctl [command]", "torrctl torrents list", "help for torrctl"} {
		if !strings.Contains(result.stdout, required) {
			t.Fatalf("help output does not contain %q:\n%s", required, result.stdout)
		}
	}

	if strings.Contains(result.stdout, "Без аргументов запускает сервер") {
		t.Fatalf("torrctl help promises daemon startup:\n%s", result.stdout)
	}
}

func TestTorrctlVersionAndCompletionUseBinaryIdentity(t *testing.T) {
	t.Parallel()

	version := runBinary(t, torrctlBinary, "--version")
	if version.exitCode != 0 || !strings.HasPrefix(version.stdout, "torrctl ") || version.stderr != "" {
		t.Fatalf("version result = %+v", version)
	}

	completion := runBinary(t, torrctlBinary, "completion", "--help")
	if completion.exitCode != 0 || completion.stderr != "" {
		t.Fatalf("completion result = %+v", completion)
	}

	if !strings.Contains(completion.stdout, "torrctl completion zsh") || strings.Contains(completion.stdout, "torrserver completion") {
		t.Fatalf("completion identity is incorrect:\n%s", completion.stdout)
	}
}

func TestTorrctlLocalVersionDoesNotRequireServer(t *testing.T) {
	t.Parallel()

	result := runBinary(t, torrctlBinary, "--output=json", "version")
	if result.exitCode != 0 || result.stderr != "" {
		t.Fatalf("local version result = %+v", result)
	}

	var response struct {
		OK   bool `json:"ok"`
		Data struct {
			Source string `json:"source"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &response); err != nil {
		t.Fatalf("decode local version: %v\n%s", err, result.stdout)
	}

	if !response.OK || response.Data.Source != "local_binary" {
		t.Fatalf("local version response = %+v", response)
	}
}

func TestTorrctlInvalidCommandHasDeterministicHumanError(t *testing.T) {
	t.Parallel()

	result := runBinary(t, torrctlBinary, "unknown-command")
	if result.exitCode != 1 || result.stdout != "" {
		t.Fatalf("invalid command result = %+v", result)
	}

	if !strings.HasPrefix(result.stderr, `torrctl: error: unknown command "unknown-command"`) {
		t.Fatalf("invalid command stderr = %q", result.stderr)
	}
}

func TestTorrctlJSONErrorIsMachineReadable(t *testing.T) {
	t.Parallel()

	result := runBinary(t, torrctlBinary, "--output=json", "unknown-command")
	if result.exitCode != 1 || result.stdout != "" {
		t.Fatalf("JSON error result = %+v", result)
	}

	var response struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.stderr), &response); err != nil {
		t.Fatalf("decode JSON error: %v\n%s", err, result.stderr)
	}

	if response.OK || response.Error.Code != "command_error" || !strings.Contains(response.Error.Message, "unknown command") {
		t.Fatalf("JSON error response = %+v", response)
	}

	if strings.Contains(result.stderr, "torrctl: error:") {
		t.Fatalf("JSON error contains human prefix: %s", result.stderr)
	}
}

func TestTorrctlUnreachableServerFailsFast(t *testing.T) {
	t.Parallel()

	result := runBinary(
		t,
		torrctlBinary,
		"--server=http://127.0.0.1:1",
		"--timeout=100ms",
		"status",
	)
	if result.exitCode != 1 || result.stdout != "" {
		t.Fatalf("unreachable result = %+v", result)
	}

	const expected = "torrctl: error: cannot reach TorrServer; verify torrserver is running and --server/context is correct\n"
	if result.stderr != expected {
		t.Fatalf("unreachable stderr = %q", result.stderr)
	}
}

func TestTorrctlAndTorrserverBuildMetadataMatch(t *testing.T) {
	t.Parallel()

	ctlVersion := runBinary(t, torrctlBinary, "--version")
	serverVersion := runBinary(t, torrserverBinary, "--version")
	if ctlVersion.exitCode != 0 || serverVersion.exitCode != 0 {
		t.Fatalf("version results = torrctl:%+v torrserver:%+v", ctlVersion, serverVersion)
	}

	_, ctlMetadata, ctlFound := strings.Cut(strings.TrimSpace(ctlVersion.stdout), " ")
	_, serverMetadata, serverFound := strings.Cut(strings.TrimSpace(serverVersion.stdout), " ")
	if !ctlFound || !serverFound || ctlMetadata != serverMetadata {
		t.Fatalf("build metadata differs: torrctl=%q torrserver=%q", ctlMetadata, serverMetadata)
	}
}

func TestTorrctlDependencyGraphExcludesDaemonAndTorrentRuntime(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), smokeTimeout)
	defer cancel()

	command := exec.CommandContext(ctx, "go", "list", "-deps", "-f", "{{.ImportPath}}", "./cmd/torrctl")
	command.Dir = serverRoot
	command.Env = testProcessEnvironment(t.TempDir())

	output, err := command.Output()
	if err != nil {
		t.Fatalf("inspect torrctl dependency graph: %v", err)
	}

	for _, dependency := range strings.Fields(string(output)) {
		if forbiddenTorrctlDependency(dependency) {
			t.Fatalf("torrctl links forbidden dependency %q", dependency)
		}
	}
}

func forbiddenTorrctlDependency(dependency string) bool {
	for _, prefix := range []string{
		"server/bootstrap",
		"server/config",
		"server/dlna",
		"server/internal/daemon",
		"server/log",
		"server/metrics",
		"server/settings",
		"server/torr",
		"server/web",
		"github.com/anacrolix/torrent",
	} {
		if dependency == prefix || strings.HasPrefix(dependency, prefix+"/") {
			return true
		}
	}

	return false
}

type binaryResult struct {
	stdout   string
	stderr   string
	exitCode int
}

func runBinary(t *testing.T, binary string, args ...string) binaryResult {
	t.Helper()

	runDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), smokeTimeout)
	defer cancel()

	command := exec.CommandContext(ctx, binary, args...)
	command.Dir = runDir
	command.Env = testProcessEnvironment(runDir)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)
	command.Stdout = &stdout
	command.Stderr = &stderr

	err := command.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run %s: %v", filepath.Base(binary), err)
		}

		exitCode = exitErr.ExitCode()
	}

	if ctx.Err() != nil {
		t.Fatalf("run %s timed out: %v", filepath.Base(binary), ctx.Err())
	}

	return binaryResult{stdout: stdout.String(), stderr: stderr.String(), exitCode: exitCode}
}

func testProcessEnvironment(home string) []string {
	return append(os.Environ(),
		"GOPATH="+originalGoPath,
		"GOMODCACHE="+filepath.Join(originalGoPath, "pkg", "mod"),
		"HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, "config"),
		"TSCTL_CONFIG="+filepath.Join(home, "missing-context.json"),
		"TS_USER=",
		"TS_PASSWORD=",
		"TS_SHUTDOWN_TOKEN=",
	)
}

func resolveGoPath() string {
	if configured := strings.TrimSpace(os.Getenv("GOPATH")); configured != "" {
		paths := filepath.SplitList(configured)
		if len(paths) > 0 {
			return paths[0]
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "go")
	}

	return filepath.Join(home, "go")
}

func buildBinary(root, output, packagePath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*smokeTimeout)
	defer cancel()

	command := exec.CommandContext(ctx, "go", "build", "-o", output, packagePath)
	command.Dir = root

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
