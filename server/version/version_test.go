package version

import (
	"runtime/debug"
	"strings"
	"testing"
)

func TestResolveInfoTaggedCleanBuild(t *testing.T) {
	t.Parallel()

	info := resolveInfo(buildInputs{
		version:    "v1.0.0-beta.1",
		commit:     "0123456789abcdef",
		buildTime:  "2026-07-14T17:00:00Z",
		dirtyState: "false",
		goVersion:  "go1.26.0",
		goos:       "linux",
		goarch:     "amd64",
	}, &debug.BuildInfo{
		GoVersion: "go1.26.0",
		Deps: []*debug.Module{
			{Path: torrentModulePath, Version: "v1.61.0"},
		},
	}, true)

	if info.Version != "v1.0.0-beta.1" || info.Commit != "0123456789abcdef" {
		t.Fatalf("release identity = (%q, %q)", info.Version, info.Commit)
	}

	if info.BuildTime != "2026-07-14T17:00:00Z" || info.Dirty != DirtyClean {
		t.Fatalf("build metadata = (%q, %q)", info.BuildTime, info.Dirty)
	}

	if info.GoVersion != "go1.26.0" || info.OS != "linux" || info.Arch != "amd64" {
		t.Fatalf("runtime metadata = (%q, %q, %q)", info.GoVersion, info.OS, info.Arch)
	}

	if got := info.TorrentEngine.EffectiveVersion(); got != "v1.61.0" {
		t.Fatalf("torrent version = %q", got)
	}
}

func TestStartupSummaryReportsCanonicalReleaseIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
		dirty   DirtyState
	}{
		{name: "development", version: "dev", dirty: DirtyUnknown},
		{name: "dirty development", version: "dev", dirty: DirtyModified},
		{name: "beta", version: "v1.0.0-beta.3", dirty: DirtyClean},
		{name: "release candidate", version: "v1.0.0-rc.1", dirty: DirtyClean},
		{name: "stable", version: "v1.0.0", dirty: DirtyClean},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			info := Info{
				Version:   test.version,
				Commit:    "0123456789abcdef",
				BuildTime: "2026-07-16T20:00:00Z",
				Dirty:     test.dirty,
				GoVersion: "go1.26.3",
				OS:        "darwin",
				Arch:      "arm64",
				TorrentEngine: ModuleInfo{
					Path:    torrentModulePath,
					Version: "v1.61.0",
				},
			}

			summary := StartupSummary(info)
			for _, required := range []string{
				`version="` + test.version + `"`,
				`commit="0123456789abcdef"`,
				`dirty="` + string(test.dirty) + `"`,
				`platform="darwin/arm64"`,
				`torrent_engine="v1.61.0"`,
			} {
				if !strings.Contains(summary, required) {
					t.Fatalf("startup summary %q does not contain %q", summary, required)
				}
			}

			if strings.Contains(summary, "2.0.0") {
				t.Fatalf("startup summary contains legacy version: %q", summary)
			}
		})
	}
}

func TestConciseUsesSharedSanitizedBinaryIdentity(t *testing.T) {
	t.Parallel()

	got := Concise("torrserver\nforged", Info{
		Version: "v1.0.0-beta.3",
		Commit:  "0123456789abcdef",
		OS:      "darwin",
		Arch:    "arm64",
	})
	const want = "torrserver forged v1.0.0-beta.3 (darwin/arm64, commit 0123456789ab)"
	if got != want {
		t.Fatalf("concise identity = %q, want %q", got, want)
	}
}

func TestStartupSummaryIsSingleLineAndHidesReplacementPath(t *testing.T) {
	t.Parallel()

	const localPath = "/Users/example/private/torrent"

	summary := StartupSummary(Info{
		Version:   "v1.0.0-beta.3\nforged-entry",
		Commit:    "abcdef\rsecret",
		BuildTime: "unknown",
		Dirty:     DirtyModified,
		GoVersion: "go1.26.3",
		OS:        "darwin",
		Arch:      "arm64",
		TorrentEngine: ModuleInfo{
			Path:             torrentModulePath,
			Version:          "v1.61.0",
			ReplacementPath:  localPath,
			LocalReplacement: true,
		},
	})

	if strings.ContainsAny(summary, "\r\n") {
		t.Fatalf("startup summary is not single-line: %q", summary)
	}

	if strings.Contains(summary, localPath) {
		t.Fatalf("startup summary exposed replacement path: %q", summary)
	}

	if !strings.Contains(summary, `version="v1.0.0-beta.3 forged-entry"`) {
		t.Fatalf("startup summary did not sanitize metadata: %q", summary)
	}
}

func TestResolveInfoDevelopmentBuildUsesVCSFallback(t *testing.T) {
	t.Parallel()

	info := resolveInfo(buildInputs{
		version:   "dev",
		goVersion: "go-fallback",
		goos:      "darwin",
		goarch:    "arm64",
	}, &debug.BuildInfo{
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abcdef123456"},
			{Key: "vcs.modified", Value: "true"},
		},
	}, true)

	if info.Version != developmentVersion || info.Commit != "abcdef123456" {
		t.Fatalf("development identity = (%q, %q)", info.Version, info.Commit)
	}

	if info.BuildTime != unknownValue || info.Dirty != DirtyModified {
		t.Fatalf("development metadata = (%q, %q)", info.BuildTime, info.Dirty)
	}
}

func TestResolveInfoUntaggedInjectedBuild(t *testing.T) {
	t.Parallel()

	info := resolveInfo(buildInputs{
		version:    "v1.0.0-beta.1-5-gabcdef",
		commit:     "abcdef",
		dirtyState: "dirty",
		goVersion:  "go1.26.0",
		goos:       "darwin",
		goarch:     "arm64",
	}, nil, false)

	if info.Version != "v1.0.0-beta.1-5-gabcdef" || info.Dirty != DirtyModified {
		t.Fatalf("untagged build = (%q, %q)", info.Version, info.Dirty)
	}
}

func TestResolveInfoMissingBuildInfoUsesDeterministicDefaults(t *testing.T) {
	t.Parallel()

	info := resolveInfo(buildInputs{}, nil, false)

	if info.Version != developmentVersion || info.Commit != unknownValue || info.BuildTime != unknownValue {
		t.Fatalf("defaults = (%q, %q, %q)", info.Version, info.Commit, info.BuildTime)
	}

	if info.Dirty != DirtyUnknown || info.GoVersion != unknownValue || info.OS != unknownValue || info.Arch != unknownValue {
		t.Fatalf("default runtime metadata = (%q, %q, %q, %q)", info.Dirty, info.GoVersion, info.OS, info.Arch)
	}

	if got := info.TorrentEngine.EffectiveVersion(); got != unknownValue {
		t.Fatalf("default torrent version = %q", got)
	}
}

func TestTorrentModuleInfoUsesRemoteReplacementVersion(t *testing.T) {
	t.Parallel()

	module := torrentModuleInfo(&debug.BuildInfo{
		Deps: []*debug.Module{
			{
				Path:    torrentModulePath,
				Version: "v1.61.0",
				Replace: &debug.Module{
					Path:    "github.com/example/torrent",
					Version: "v1.61.0-patched.1",
				},
			},
		},
	}, true)

	if module.ReplacementPath != "github.com/example/torrent" {
		t.Fatalf("replacement path = %q", module.ReplacementPath)
	}

	if got := module.EffectiveVersion(); got != "v1.61.0-patched.1" {
		t.Fatalf("effective replacement version = %q", got)
	}
}

func TestTorrentModuleInfoRedactsLocalReplacementPath(t *testing.T) {
	t.Parallel()

	localPath := "/Users/developer/private/torrent"
	module := torrentModuleInfo(&debug.BuildInfo{
		Deps: []*debug.Module{
			{
				Path:    torrentModulePath,
				Version: "v1.61.0",
				Replace: &debug.Module{Path: localPath},
			},
		},
	}, true)

	if !module.LocalReplacement || module.ReplacementPath != "" {
		t.Fatalf("local replacement metadata = %#v", module)
	}

	if strings.Contains(module.EffectiveVersion(), localPath) {
		t.Fatalf("effective version exposed local path: %q", module.EffectiveVersion())
	}
}

func TestResolveDirtyStateExplicitValueOverridesVCS(t *testing.T) {
	t.Parallel()

	if got := resolveDirtyState("clean", "true"); got != DirtyClean {
		t.Fatalf("dirty state = %q, want clean", got)
	}
}
