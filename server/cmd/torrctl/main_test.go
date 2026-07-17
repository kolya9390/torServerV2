package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"server/internal/cliapp"
)

const smokeTimeout = 10 * time.Second

func TestTorrctlHelpIsClientOnly(t *testing.T) {
	t.Parallel()

	result := runCLI(t, "--help")
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

	version := runCLI(t, "--version")
	if version.exitCode != 0 || !strings.HasPrefix(version.stdout, "torrctl ") || version.stderr != "" {
		t.Fatalf("version result = %+v", version)
	}

	completion := runCLI(t, "completion", "--help")
	if completion.exitCode != 0 || completion.stderr != "" {
		t.Fatalf("completion result = %+v", completion)
	}

	if !strings.Contains(completion.stdout, "torrctl completion zsh") ||
		strings.Contains(completion.stdout, "torrserver completion") {
		t.Fatalf("completion identity is incorrect:\n%s", completion.stdout)
	}
}

func TestTorrctlLocalVersionDoesNotRequireServer(t *testing.T) {
	t.Parallel()

	result := runCLI(t, "--output=json", "version")
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

	result := runCLI(t, "unknown-command")
	if result.exitCode != 1 || result.stdout != "" {
		t.Fatalf("invalid command result = %+v", result)
	}

	if !strings.HasPrefix(result.stderr, `torrctl: error: unknown command "unknown-command"`) {
		t.Fatalf("invalid command stderr = %q", result.stderr)
	}
}

func TestTorrctlJSONErrorIsMachineReadable(t *testing.T) {
	t.Parallel()

	result := runCLI(t, "--output=json", "unknown-command")
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

	if response.OK || response.Error.Code != "command_error" ||
		!strings.Contains(response.Error.Message, "unknown command") {
		t.Fatalf("JSON error response = %+v", response)
	}

	if strings.Contains(result.stderr, "torrctl: error:") {
		t.Fatalf("JSON error contains human prefix: %s", result.stderr)
	}
}

func TestTorrctlUnreachableServerFailsFast(t *testing.T) {
	t.Parallel()

	result := runCLI(
		t,
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

func TestTorrctlDependencyGraphExcludesDaemonAndTorrentRuntime(t *testing.T) {
	t.Parallel()

	serverRoot, err := findServerRoot()
	if err != nil {
		t.Fatalf("find server root: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), smokeTimeout)
	defer cancel()

	command := exec.CommandContext(ctx, "go", "list", "-deps", "-f", "{{.ImportPath}}", "./cmd/torrctl")
	command.Dir = serverRoot

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

type cliResult struct {
	stdout   string
	stderr   string
	exitCode int
}

func runCLI(t *testing.T, args ...string) cliResult {
	t.Helper()

	home := t.TempDir()
	environment := map[string]string{
		"HOME":              home,
		"XDG_CONFIG_HOME":   filepath.Join(home, "config"),
		"TSCTL_CONFIG":      filepath.Join(home, "missing-context.json"),
		"TS_USER":           "",
		"TS_PASSWORD":       "",
		"TS_SHUTDOWN_TOKEN": "",
	}
	dependencies := cliapp.DefaultDependencies()
	dependencies.ProgramName = "torrctl"
	dependencies.Getenv = func(name string) string { return environment[name] }

	ctx, cancel := context.WithTimeout(context.Background(), smokeTimeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	exitCode := cliapp.Run(cliapp.Invocation{
		Context: ctx,
		Args:    args,
		Stdin:   bytes.NewReader(nil),
		Stdout:  &stdout,
		Stderr:  &stderr,
	}, dependencies)
	if ctx.Err() != nil {
		t.Fatalf("run torrctl timed out: %v", ctx.Err())
	}

	return cliResult{stdout: stdout.String(), stderr: stderr.String(), exitCode: exitCode}
}

func findServerRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("resolve test source path")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..")), nil
}
