//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	commandTimeout  = 10 * time.Second
	processLogLimit = 1 << 20
)

type processResult struct {
	stdout   string
	stderr   string
	exitCode int
	duration time.Duration
	timedOut bool
}

type lockedBuffer struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	remaining int
}

func newLockedBuffer(limit int) *lockedBuffer {
	return &lockedBuffer{remaining: limit}
}

func (buf *lockedBuffer) Write(data []byte) (int, error) {
	buf.mu.Lock()
	defer buf.mu.Unlock()

	written := len(data)
	if buf.remaining <= 0 {
		return written, nil
	}

	data = data[:min(len(data), buf.remaining)]
	buf.remaining -= len(data)
	_, _ = buf.buffer.Write(data)

	return written, nil
}

func (buf *lockedBuffer) String() string {
	buf.mu.Lock()
	defer buf.mu.Unlock()

	return buf.buffer.String()
}

type daemonProcess struct {
	command *exec.Cmd
	stdout  *lockedBuffer
	stderr  *lockedBuffer
	done    chan struct{}

	waitMu  sync.Mutex
	waitErr error
}

func startDaemon(t *testing.T, binary, workDir string, env, args []string) *daemonProcess {
	t.Helper()

	stdout := newLockedBuffer(processLogLimit)
	stderr := newLockedBuffer(processLogLimit)
	command := exec.Command(binary, args...)
	command.Dir = workDir
	command.Env = env
	command.Stdout = stdout
	command.Stderr = stderr

	if err := command.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	process := &daemonProcess{
		command: command,
		stdout:  stdout,
		stderr:  stderr,
		done:    make(chan struct{}),
	}
	go func() {
		err := command.Wait()

		process.waitMu.Lock()
		process.waitErr = err
		process.waitMu.Unlock()
		close(process.done)
	}()

	t.Cleanup(func() {
		process.killAndWait(t)
	})

	return process
}

func (process *daemonProcess) wait(ctx context.Context) error {
	select {
	case <-process.done:
		process.waitMu.Lock()
		defer process.waitMu.Unlock()

		return process.waitErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (process *daemonProcess) killAndWait(t *testing.T) {
	t.Helper()

	select {
	case <-process.done:
		return
	default:
	}

	if process.command.Process != nil {
		_ = process.command.Process.Kill()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := process.wait(ctx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Logf("daemon cleanup: %v", err)
	}
}

func (process *daemonProcess) logs() string {
	return process.stdout.String() + "\n" + process.stderr.String()
}

func runProcess(t *testing.T, binary, workDir string, env []string, args ...string) processResult {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	started := time.Now()
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir = workDir
	command.Env = env

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()

	result := processResult{
		stdout:   stdout.String(),
		stderr:   stderr.String(),
		duration: time.Since(started),
		timedOut: errors.Is(ctx.Err(), context.DeadlineExceeded),
	}
	if err == nil {
		return result
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.exitCode = exitErr.ExitCode()

		return result
	}

	result.exitCode = -1
	result.stderr += fmt.Sprintf("\nexecute process: %v", err)

	return result
}

func buildSplitBinaries(t *testing.T, root string) (string, string) {
	t.Helper()

	binDir := filepath.Join(root, "bin")
	mustMkdirAll(t, binDir)

	ldflags := strings.Join([]string{
		"-X server/version.version=v1.0.0-e2e",
		"-X server/version.commit=0123456789abcdef0123456789abcdef01234567",
		"-X server/version.buildTime=2026-01-02T03:04:05Z",
		"-X server/version.dirtyState=clean",
	}, " ")
	env := os.Environ()

	serverBinary := filepath.Join(binDir, "torrserver")
	cliBinary := filepath.Join(binDir, "torrctl")
	buildBinary(t, env, serverBinary, "./cmd/torrserver", ldflags)
	buildBinary(t, env, cliBinary, "./cmd/torrctl", ldflags)

	return serverBinary, cliBinary
}

func buildBinary(t *testing.T, env []string, output, packagePath, ldflags string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	command := exec.CommandContext(
		ctx,
		"go",
		"build",
		"-trimpath",
		"-buildvcs=false",
		"-ldflags",
		ldflags,
		"-o",
		output,
		packagePath,
	)
	command.Dir = moduleRoot()
	command.Env = env

	outputBytes, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("build %s: %v\n%s", packagePath, err, outputBytes)
	}
}

func moduleRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve E2E source path")
	}

	return filepath.Dir(filepath.Dir(file))
}

func reserveLoopbackPort(t *testing.T) (string, int) {
	t.Helper()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback port: %v", err)
	}

	address := listener.Addr().(*net.TCPAddr)
	port := address.Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release loopback port: %v", err)
	}

	return "http://127.0.0.1:" + strconv.Itoa(port), port
}

func assertPortInUse(t *testing.T, port int) {
	t.Helper()

	listener, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(port))
	if err == nil {
		_ = listener.Close()
		t.Fatalf("daemon port %d can be rebound while server is running", port)
	}
}

func assertPortReleased(t *testing.T, port int) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		listener, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(port))
		if err == nil {
			_ = listener.Close()

			return
		}

		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("daemon port %d was not released", port)
}

func withEnv(base []string, overrides map[string]string) []string {
	keys := make(map[string]struct{}, len(overrides))
	for key := range overrides {
		keys[key] = struct{}{}
	}

	out := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key, _, found := strings.Cut(entry, "=")
		if _, overridden := keys[key]; found && overridden {
			continue
		}

		out = append(out, entry)
	}

	for key, value := range overrides {
		out = append(out, key+"="+value)
	}

	return out
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("create directory %s: %v", path, err)
	}
}
