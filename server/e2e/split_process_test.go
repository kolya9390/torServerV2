//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	e2eShutdownToken = "e2e-shutdown-token-0123456789abcdef" // #nosec G101 -- isolated E2E fixture.
	e2eWrongToken    = "e2e-wrong-token-0123456789abcdef"    // #nosec G101 -- redaction fixture.
)

type splitScenario struct {
	t            *testing.T
	root         string
	serverBinary string
	cliBinary    string
	baseURL      string
	port         int
	contextPath  string
	cliWorkDir   string
	cliEnv       []string
	daemon       *daemonProcess
}

func TestSplitProcessWorkflow(t *testing.T) {
	scenario := newSplitScenario(t)
	scenario.verifyLocalVersionIdentity()
	scenario.verifyProcessBoundaries()
	scenario.startServer()
	scenario.verifyContextAndStatus()
	scenario.verifyTorrentWorkflow()
	scenario.verifySettingsAndShutdown()
	scenario.verifyFilesystemBoundary()
}

func newSplitScenario(t *testing.T) *splitScenario {
	t.Helper()

	root := t.TempDir()
	serverBinary, cliBinary := buildSplitBinaries(t, root)
	baseURL, port := reserveLoopbackPort(t)
	cliWorkDir := filepath.Join(root, "cli-work")
	cliHome := filepath.Join(root, "cli-home")
	cliTmp := filepath.Join(root, "cli-tmp")
	contextPath := filepath.Join(root, "cli-config", "contexts.json")
	for _, path := range []string{cliWorkDir, cliHome, cliTmp} {
		mustMkdirAll(t, path)
	}

	cliEnv := withEnv(os.Environ(), map[string]string{
		"HOME":              cliHome,
		"TMPDIR":            cliTmp,
		"TSCTL_CONFIG":      contextPath,
		"TS_CONTEXT":        "",
		"TS_SERVER":         "",
		"TS_USER":           "",
		"TS_PASSWORD":       "",
		"TS_SHUTDOWN_TOKEN": "",
	})

	return &splitScenario{
		t:            t,
		root:         root,
		serverBinary: serverBinary,
		cliBinary:    cliBinary,
		baseURL:      baseURL,
		port:         port,
		contextPath:  contextPath,
		cliWorkDir:   cliWorkDir,
		cliEnv:       cliEnv,
	}
}

func (scenario *splitScenario) verifyLocalVersionIdentity() {
	scenario.t.Helper()

	serverVersion := runProcess(scenario.t, scenario.serverBinary, scenario.cliWorkDir, scenario.cliEnv, "--version")
	if serverVersion.exitCode != 0 || serverVersion.stderr != "" {
		scenario.t.Fatalf("torrserver --version failed: %+v", serverVersion)
	}

	cliVersion := runProcess(
		scenario.t,
		scenario.cliBinary,
		scenario.cliWorkDir,
		scenario.cliEnv,
		"--output=json",
		"version",
	)
	local := decodeSuccess[struct {
		Source  string `json:"source"`
		Version string `json:"version"`
		Commit  string `json:"commit"`
	}](scenario.t, cliVersion)

	if local.Source != "local_binary" || local.Version != "v1.0.0-e2e" ||
		local.Commit != "0123456789abcdef0123456789abcdef01234567" {
		scenario.t.Fatalf("unexpected torrctl build identity: %+v", local)
	}

	for _, value := range []string{"torrserver v1.0.0-e2e", "commit 0123456789ab"} {
		if !strings.Contains(serverVersion.stdout, value) {
			scenario.t.Fatalf("torrserver version output missing %q: %s", value, serverVersion.stdout)
		}
	}
}

func (scenario *splitScenario) verifyProcessBoundaries() {
	scenario.t.Helper()

	cliSideEffects := filepath.Join(scenario.root, "cli-serve-side-effects")
	serverSideEffects := filepath.Join(scenario.root, "server-command-side-effects")
	mustMkdirAll(scenario.t, cliSideEffects)
	mustMkdirAll(scenario.t, serverSideEffects)
	_, unusedPort := reserveLoopbackPort(scenario.t)

	cliResult := runProcess(
		scenario.t,
		scenario.cliBinary,
		cliSideEffects,
		scenario.cliEnv,
		"serve",
		"--port",
		fmt.Sprintf("%d", unusedPort),
	)
	if cliResult.exitCode != 1 || !strings.Contains(cliResult.stderr, "unknown command \"serve\"") {
		scenario.t.Fatalf("torrctl engine boundary failed: %+v", cliResult)
	}
	assertDirectoryEmpty(scenario.t, cliSideEffects)
	assertPortReleased(scenario.t, unusedPort)

	serverResult := runProcess(
		scenario.t,
		scenario.serverBinary,
		serverSideEffects,
		scenario.cliEnv,
		"--server",
		"http://private-user:private-pass@127.0.0.1:1",
		"--token",
		e2eWrongToken,
		"status",
	)
	if serverResult.exitCode != 2 || !strings.Contains(serverResult.stderr, "management commands moved to torrctl") {
		scenario.t.Fatalf("former daemon CLI command was not rejected: %+v", serverResult)
	}
	assertNoSecrets(scenario.t, serverResult.stdout+serverResult.stderr, e2eWrongToken, "private-pass")
	assertDirectoryEmpty(scenario.t, serverSideEffects)

	unavailableURL, _ := reserveLoopbackPort(scenario.t)
	unavailable := runProcess(
		scenario.t,
		scenario.cliBinary,
		scenario.cliWorkDir,
		scenario.cliEnv,
		"--server",
		unavailableURL,
		"--timeout=300ms",
		"--output=json",
		"status",
	)
	failure := decodeFailure(scenario.t, unavailable)
	if failure.Error.Code != "compatibility_transport_error" ||
		!strings.Contains(failure.Error.Message, "cannot reach TorrServer") || unavailable.duration > 2*time.Second {
		scenario.t.Fatalf("unavailable daemon contract failed: duration=%s error=%+v", unavailable.duration, failure.Error)
	}
}

func (scenario *splitScenario) startServer() {
	scenario.t.Helper()

	daemonState := filepath.Join(scenario.root, "daemon-state")
	daemonWork := filepath.Join(scenario.root, "daemon-work")
	daemonHome := filepath.Join(scenario.root, "daemon-home")
	daemonTmp := filepath.Join(scenario.root, "daemon-tmp")
	configPath := filepath.Join(scenario.root, "release-config.yml")
	for _, path := range []string{daemonState, daemonWork, daemonHome, daemonTmp} {
		mustMkdirAll(scenario.t, path)
	}
	writeReleaseConfig(scenario.t, configPath, daemonState, scenario.port)

	daemonEnv := withEnv(os.Environ(), map[string]string{
		"HOME":              daemonHome,
		"TMPDIR":            daemonTmp,
		"TS_CONFIG":         configPath,
		"TS_SHUTDOWN_MODE":  "public",
		"TS_SHUTDOWN_TOKEN": e2eShutdownToken,
		"TS_MIGRATION_MODE": "",
	})
	scenario.daemon = startDaemon(
		scenario.t,
		scenario.serverBinary,
		daemonWork,
		daemonEnv,
		[]string{"serve", "--ip", "127.0.0.1", "--port", fmt.Sprintf("%d", scenario.port), "--path", daemonState},
	)
	scenario.waitUntilReady()
	assertPortInUse(scenario.t, scenario.port)
}

func (scenario *splitScenario) waitUntilReady() {
	scenario.t.Helper()

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-scenario.daemon.done:
			scenario.t.Fatalf("daemon exited during startup: %v\n%s", scenario.daemon.waitErr, scenario.daemon.logs())
		default:
		}

		status := runProcess(
			scenario.t,
			scenario.cliBinary,
			scenario.cliWorkDir,
			scenario.cliEnv,
			"--server",
			scenario.baseURL,
			"--timeout=500ms",
			"--output=json",
			"status",
		)
		if status.exitCode == 0 {
			return
		}

		time.Sleep(100 * time.Millisecond)
	}

	scenario.t.Fatalf("daemon did not become ready:\n%s", scenario.daemon.logs())
}

func (scenario *splitScenario) verifyContextAndStatus() {
	scenario.t.Helper()

	decodeSuccess[map[string]any](scenario.t, scenario.cliJSON(
		"context", "add", "--name", "e2e", "--server", scenario.baseURL,
	))
	decodeSuccess[map[string]any](scenario.t, scenario.cliJSON("context", "use", "--name", "e2e"))
	current := decodeSuccess[struct {
		Name   string `json:"name"`
		Server string `json:"server"`
	}](scenario.t, scenario.cliJSON("context", "current"))
	if current.Name != "e2e" || current.Server != scenario.baseURL {
		scenario.t.Fatalf("current context = %+v", current)
	}

	info, err := os.Stat(scenario.contextPath)
	if err != nil {
		scenario.t.Fatalf("stat context config: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		scenario.t.Fatalf("context config mode = %o, want 600", info.Mode().Perm())
	}

	human := scenario.cli("status")
	if human.exitCode != 0 || human.stderr != "" {
		scenario.t.Fatalf("human status failed: %+v", human)
	}
	for _, expected := range []string{
		"Server: " + scenario.baseURL,
		"Context: e2e",
		"Product: torrserver",
		"Application: v1.0.0-e2e",
		"API: v1",
		"Ready: ready",
	} {
		if !strings.Contains(human.stdout, expected) {
			scenario.t.Fatalf("human status missing %q:\n%s", expected, human.stdout)
		}
	}

	status := decodeSuccess[struct {
		Server  string `json:"server"`
		Version struct {
			Product     string `json:"product"`
			Application string `json:"application_version"`
			Current     string `json:"current"`
		} `json:"version"`
		Ready struct {
			Status  string `json:"status"`
			HTTP    bool   `json:"http"`
			Torrent bool   `json:"torrent"`
		} `json:"ready"`
	}](scenario.t, scenario.cliJSON("status"))
	if strings.TrimRight(status.Server, "/") != scenario.baseURL || status.Version.Product != "torrserver" ||
		status.Version.Application != "v1.0.0-e2e" || status.Version.Current != "v1" ||
		status.Ready.Status != "ready" || !status.Ready.HTTP || !status.Ready.Torrent {
		scenario.t.Fatalf("JSON status = %+v", status)
	}
}

func (scenario *splitScenario) verifyTorrentWorkflow() {
	scenario.t.Helper()

	empty := decodeSuccess[[]torrentView](scenario.t, scenario.cliJSON("torrents", "list"))
	if len(empty) != 0 {
		scenario.t.Fatalf("initial torrent list = %+v, want empty", empty)
	}

	magnet := "magnet:?xt=urn:btih:" + fixtureMagnetHash + "&dn=E2E%20Magnet"
	addedMagnet := decodeSuccess[torrentView](scenario.t, scenario.cliJSON(
		"torrents", "add", magnet, "--save", "--title", "E2E Magnet",
	))
	if !strings.EqualFold(addedMagnet.Hash, fixtureMagnetHash) || addedMagnet.Title != "E2E Magnet" {
		scenario.t.Fatalf("magnet add result = %+v", addedMagnet)
	}

	fixturePath := filepath.Join(scenario.root, "fixture.torrent")
	wantUploadHash := writeTorrentFixture(scenario.t, fixturePath)
	uploaded := decodeSuccess[torrentView](scenario.t, scenario.cliJSON(
		"torrents", "add", fixturePath, "--save", "--title", "E2E Upload",
	))
	if !strings.EqualFold(uploaded.Hash, wantUploadHash) || uploaded.Title != "E2E Upload" {
		scenario.t.Fatalf("torrent upload result = %+v, want hash=%s", uploaded, wantUploadHash)
	}

	torrents := decodeSuccess[[]torrentView](scenario.t, scenario.cliJSON("torrents", "list"))
	if len(torrents) != 2 {
		scenario.t.Fatalf("torrent list length = %d, want 2: %+v", len(torrents), torrents)
	}

	files := decodeSuccess[[]fileView](scenario.t, scenario.cliJSON("url", wantUploadHash, "--list"))
	if len(files) != 2 {
		scenario.t.Fatalf("uploaded torrent files = %+v", files)
	}
	movieID := findMovieFileID(scenario.t, files)

	automatic := decodeSuccess[streamURLView](scenario.t, scenario.cliJSON("url", wantUploadHash))
	assertStreamURL(scenario.t, automatic, scenario.baseURL, wantUploadHash, movieID)
	byName := decodeSuccess[streamURLView](scenario.t, scenario.cliJSON(
		"url", "E2E Upload", "--file", "movie.mkv",
	))
	assertStreamURL(scenario.t, byName, scenario.baseURL, wantUploadHash, movieID)

	humanURL := scenario.cli("url", wantUploadHash, "--file", fmt.Sprintf("%d", movieID))
	if humanURL.exitCode != 0 || strings.TrimSpace(humanURL.stdout) != automatic.URL || humanURL.stderr != "" {
		scenario.t.Fatalf("human stream URL failed: %+v", humanURL)
	}
}

func (scenario *splitScenario) verifySettingsAndShutdown() {
	scenario.t.Helper()

	settings := decodeSuccess[map[string]any](scenario.t, scenario.cliJSON("settings", "get"))
	if _, ok := settings["CacheSize"]; !ok {
		scenario.t.Fatalf("settings response has no CacheSize: %+v", settings)
	}

	wrongToken := withEnv(scenario.cliEnv, map[string]string{"TS_SHUTDOWN_TOKEN": e2eWrongToken})
	wrong := scenario.cliWithEnv(wrongToken, "--output=json", "shutdown", "--mode", "public")
	failure := decodeFailure(scenario.t, wrong)
	if failure.Error.Status != 401 {
		scenario.t.Fatalf("wrong shutdown token status = %d, want 401: %+v", failure.Error.Status, failure.Error)
	}
	assertNoSecrets(scenario.t, wrong.stdout+wrong.stderr+scenario.daemon.logs(), e2eWrongToken)

	correctEnv := withEnv(scenario.cliEnv, map[string]string{"TS_SHUTDOWN_TOKEN": e2eShutdownToken})
	shutdown := scenario.cliWithEnv(correctEnv, "--output=json", "shutdown", "--mode", "public")
	action := decodeSuccess[map[string]string](scenario.t, shutdown)
	if action["action"] != "shutdown_accepted" || action["mode"] != "public" {
		scenario.t.Fatalf("shutdown response = %+v", action)
	}
	assertNoSecrets(scenario.t, shutdown.stdout+shutdown.stderr+scenario.daemon.logs(), e2eShutdownToken)

	waitCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := scenario.daemon.wait(waitCtx); err != nil {
		scenario.t.Fatalf("graceful daemon exit: %v\n%s", err, scenario.daemon.logs())
	}
	if scenario.daemon.command.ProcessState == nil || !scenario.daemon.command.ProcessState.Exited() {
		scenario.t.Fatal("daemon process state does not report exit")
	}
	assertPortReleased(scenario.t, scenario.port)
}

func (scenario *splitScenario) verifyFilesystemBoundary() {
	scenario.t.Helper()

	assertTopLevelEntries(
		scenario.t,
		scenario.root,
		"bin",
		"cli-config",
		"cli-home",
		"cli-serve-side-effects",
		"cli-tmp",
		"cli-work",
		"daemon-home",
		"daemon-state",
		"daemon-tmp",
		"daemon-work",
		"fixture.torrent",
		"release-config.yml",
		"server-command-side-effects",
	)
}

func (scenario *splitScenario) cli(args ...string) processResult {
	scenario.t.Helper()

	return scenario.cliWithEnv(scenario.cliEnv, args...)
}

func (scenario *splitScenario) cliJSON(args ...string) processResult {
	scenario.t.Helper()

	return scenario.cli(append([]string{"--output=json"}, args...)...)
}

func (scenario *splitScenario) cliWithEnv(env []string, args ...string) processResult {
	scenario.t.Helper()

	return runProcess(scenario.t, scenario.cliBinary, scenario.cliWorkDir, env, args...)
}

func findMovieFileID(t *testing.T, files []fileView) int {
	t.Helper()

	for _, file := range files {
		if strings.HasSuffix(file.Path, fixtureMovie) && file.Length == fixtureMovieSize {
			return file.ID
		}
	}

	payload, _ := json.Marshal(files)
	t.Fatalf("movie fixture not found in file list: %s", payload)

	return 0
}

func assertNoSecrets(t *testing.T, output string, secrets ...string) {
	t.Helper()

	for _, secret := range secrets {
		if strings.Contains(output, secret) {
			t.Fatalf("process output leaked secret %q", secret)
		}
	}
}
