package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	buildversion "server/version"
)

func TestVersionCommandDevelopmentBuildHumanOutput(t *testing.T) {
	info := developmentBuildInfo()
	root := newRootCmdWithBuildInfo(info)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	root.SetArgs([]string{"version"})
	root.SetOut(&stdout)
	root.SetErr(&stderr)

	if err := root.Execute(); err != nil {
		t.Fatalf("execute version: %v", err)
	}

	if stderr.Len() != 0 {
		t.Fatalf("stderr is not empty: %s", stderr.String())
	}

	for _, expected := range []string{
		"Source: local binary",
		"Application version: dev",
		"Commit: unknown",
		"Build time: unknown",
		"Dirty: unknown",
		"Go: go1.26.0",
		"Platform: darwin/arm64",
		"Torrent engine: v1.61.0",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("version output does not contain %q:\n%s", expected, stdout.String())
		}
	}
}

func TestVersionCommandTaggedBuildJSONOutput(t *testing.T) {
	info := taggedBuildInfo()
	root := newRootCmdWithBuildInfo(info)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	root.SetArgs([]string{"version", "--output", "json"})
	root.SetOut(&stdout)
	root.SetErr(&stderr)

	if err := root.Execute(); err != nil {
		t.Fatalf("execute version JSON: %v", err)
	}

	if stderr.Len() != 0 {
		t.Fatalf("stderr is not empty: %s", stderr.String())
	}

	var envelope struct {
		OK   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}

	decodeSingleJSONValue(t, stdout.Bytes(), &envelope)

	if !envelope.OK {
		t.Fatal("version JSON has ok=false")
	}

	var view localVersionView
	if err := json.Unmarshal(envelope.Data, &view); err != nil {
		t.Fatalf("decode version data: %v", err)
	}

	if view.Source != localBinaryVersionSource || view.Version != info.Version {
		t.Fatalf("unexpected local version identity: %+v", view)
	}

	if view.Commit != info.Commit || view.BuildTime != info.BuildTime || view.Dirty != buildversion.DirtyClean {
		t.Fatalf("unexpected build metadata: %+v", view)
	}

	if view.TorrentEngine.Version != "v1.61.0" || view.TorrentEngine.Path != "github.com/anacrolix/torrent" {
		t.Fatalf("unexpected torrent engine metadata: %+v", view.TorrentEngine)
	}
}

func TestVersionFlagUsesConciseLocalBuildOutput(t *testing.T) {
	info := taggedBuildInfo()
	root := newRootCmdWithBuildInfo(info)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	root.SetArgs([]string{"--version"})
	root.SetOut(&stdout)
	root.SetErr(&stderr)

	if err := root.Execute(); err != nil {
		t.Fatalf("execute --version: %v", err)
	}

	want := "torrserver v1.0.0-beta.1 (linux/amd64, commit 0123456789ab)\n"
	if stdout.String() != want {
		t.Fatalf("--version output = %q, want %q", stdout.String(), want)
	}

	if stderr.Len() != 0 {
		t.Fatalf("stderr is not empty: %s", stderr.String())
	}
}

func developmentBuildInfo() buildversion.Info {
	return buildversion.Info{
		Version:   "dev",
		Commit:    "unknown",
		BuildTime: "unknown",
		Dirty:     buildversion.DirtyUnknown,
		GoVersion: "go1.26.0",
		OS:        "darwin",
		Arch:      "arm64",
		TorrentEngine: buildversion.ModuleInfo{
			Path:    "github.com/anacrolix/torrent",
			Version: "v1.61.0",
		},
	}
}

func taggedBuildInfo() buildversion.Info {
	return buildversion.Info{
		Version:   "v1.0.0-beta.1",
		Commit:    "0123456789abcdef0123456789abcdef01234567",
		BuildTime: "2026-07-14T18:00:00Z",
		Dirty:     buildversion.DirtyClean,
		GoVersion: "go1.26.0",
		OS:        "linux",
		Arch:      "amd64",
		TorrentEngine: buildversion.ModuleInfo{
			Path:    "github.com/anacrolix/torrent",
			Version: "v1.61.0",
		},
	}
}
