package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"unicode"
)

const (
	developmentVersion = "dev"
	unknownValue       = "unknown"
	torrentModulePath  = "github.com/anacrolix/torrent"
)

// These values are build inputs set through -ldflags -X. Git tags remain the
// release version source of truth; empty values are normalized by Current.
var (
	version    = developmentVersion
	commit     string
	buildTime  string
	dirtyState string
)

// DirtyState describes whether the source tree had uncommitted changes.
type DirtyState string

const (
	DirtyUnknown  DirtyState = "unknown"
	DirtyClean    DirtyState = "clean"
	DirtyModified DirtyState = "modified"
)

// ModuleInfo identifies a dependency without exposing local replacement paths.
type ModuleInfo struct {
	Path               string `json:"path"`
	Version            string `json:"version"`
	ReplacementPath    string `json:"replacement_path,omitempty"`
	ReplacementVersion string `json:"replacement_version,omitempty"`
	LocalReplacement   bool   `json:"local_replacement,omitempty"`
}

// EffectiveVersion returns the dependency version used by the build.
func (m ModuleInfo) EffectiveVersion() string {
	if m.ReplacementVersion != "" {
		return m.ReplacementVersion
	}

	if m.Version != "" {
		if m.LocalReplacement {
			return m.Version + " (local replacement)"
		}

		return m.Version
	}

	if m.LocalReplacement {
		return "local replacement"
	}

	return unknownValue
}

// Info is an immutable-by-convention build snapshot returned by value.
// Application Version is independent from HTTP API and dependency versions.
type Info struct {
	Version       string     `json:"version"`
	Commit        string     `json:"commit"`
	BuildTime     string     `json:"build_time"`
	Dirty         DirtyState `json:"dirty"`
	GoVersion     string     `json:"go_version"`
	OS            string     `json:"os"`
	Arch          string     `json:"arch"`
	TorrentEngine ModuleInfo `json:"torrent_engine"`
}

type buildInputs struct {
	version    string
	commit     string
	buildTime  string
	dirtyState string
	goVersion  string
	goos       string
	goarch     string
}

// Current returns a normalized snapshot of the running binary build.
func Current() Info {
	buildInfo, ok := debug.ReadBuildInfo()

	return resolveInfo(buildInputs{
		version:    version,
		commit:     commit,
		buildTime:  buildTime,
		dirtyState: dirtyState,
		goVersion:  runtime.Version(),
		goos:       runtime.GOOS,
		goarch:     runtime.GOARCH,
	}, buildInfo, ok)
}

// Version preserves the existing lightweight application-version API.
func Version() string {
	return Current().Version
}

// GetTorrentVersion preserves the existing dependency-version API.
func GetTorrentVersion() string {
	return Current().TorrentEngine.EffectiveVersion()
}

// Concise renders the shared human-readable identity used by product binaries.
func Concise(programName string, info Info) string {
	commit := safeSummaryValue(info.Commit)
	if len(commit) > 12 {
		commit = commit[:12]
	}

	return fmt.Sprintf(
		"%s %s (%s/%s, commit %s)",
		safeSummaryValue(programName),
		safeSummaryValue(info.Version),
		safeSummaryValue(info.OS),
		safeSummaryValue(info.Arch),
		commit,
	)
}

// StartupSummary renders canonical, single-line daemon build metadata without
// exposing module replacement paths or other local build paths.
func StartupSummary(info Info) string {
	platform := safeSummaryValue(info.OS) + "/" + safeSummaryValue(info.Arch)

	return fmt.Sprintf(
		"Start TorrServer version=%q commit=%q build_time=%q dirty=%q go=%q platform=%q torrent_engine=%q",
		safeSummaryValue(info.Version),
		safeSummaryValue(info.Commit),
		safeSummaryValue(info.BuildTime),
		safeSummaryValue(string(info.Dirty)),
		safeSummaryValue(info.GoVersion),
		platform,
		safeSummaryValue(info.TorrentEngine.EffectiveVersion()),
	)
}

func safeSummaryValue(value string) string {
	value = strings.Map(func(symbol rune) rune {
		if unicode.IsControl(symbol) {
			return ' '
		}

		return symbol
	}, value)

	return valueOrDefault(strings.Join(strings.Fields(value), " "), unknownValue)
}

func resolveInfo(inputs buildInputs, buildInfo *debug.BuildInfo, buildInfoOK bool) Info {
	vcs := vcsSettings(buildInfo, buildInfoOK)

	return Info{
		Version:       valueOrDefault(inputs.version, developmentVersion),
		Commit:        firstKnownValue(inputs.commit, vcs["vcs.revision"]),
		BuildTime:     valueOrDefault(inputs.buildTime, unknownValue),
		Dirty:         resolveDirtyState(inputs.dirtyState, vcs["vcs.modified"]),
		GoVersion:     resolveGoVersion(inputs.goVersion, buildInfo, buildInfoOK),
		OS:            valueOrDefault(inputs.goos, unknownValue),
		Arch:          valueOrDefault(inputs.goarch, unknownValue),
		TorrentEngine: torrentModuleInfo(buildInfo, buildInfoOK),
	}
}

func resolveGoVersion(fallback string, buildInfo *debug.BuildInfo, buildInfoOK bool) string {
	if buildInfoOK && buildInfo != nil && strings.TrimSpace(buildInfo.GoVersion) != "" {
		return strings.TrimSpace(buildInfo.GoVersion)
	}

	return valueOrDefault(fallback, unknownValue)
}

func resolveDirtyState(explicit, vcsModified string) DirtyState {
	for _, value := range []string{explicit, vcsModified} {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "1", "true", "dirty", "modified":
			return DirtyModified
		case "0", "false", "clean":
			return DirtyClean
		}
	}

	return DirtyUnknown
}

func vcsSettings(buildInfo *debug.BuildInfo, ok bool) map[string]string {
	settings := make(map[string]string)
	if !ok || buildInfo == nil {
		return settings
	}

	for _, setting := range buildInfo.Settings {
		if strings.HasPrefix(setting.Key, "vcs.") {
			settings[setting.Key] = setting.Value
		}
	}

	return settings
}

func torrentModuleInfo(buildInfo *debug.BuildInfo, ok bool) ModuleInfo {
	module := ModuleInfo{Path: torrentModulePath}
	if !ok || buildInfo == nil {
		return module
	}

	for _, dependency := range buildInfo.Deps {
		if dependency == nil || dependency.Path != torrentModulePath {
			continue
		}

		module.Version = strings.TrimSpace(dependency.Version)
		if dependency.Replace == nil {
			return module
		}

		module.ReplacementVersion = strings.TrimSpace(dependency.Replace.Version)
		if isLocalReplacement(dependency.Replace.Path, module.ReplacementVersion) {
			module.LocalReplacement = true

			return module
		}

		module.ReplacementPath = strings.TrimSpace(dependency.Replace.Path)

		return module
	}

	return module
}

func isLocalReplacement(path, replacementVersion string) bool {
	if strings.TrimSpace(replacementVersion) != "" {
		return false
	}

	path = strings.TrimSpace(path)

	return strings.HasPrefix(path, ".") || strings.HasPrefix(path, "/") || strings.Contains(path, `:\`)
}

func firstKnownValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}

	return unknownValue
}

func valueOrDefault(value, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}

	return fallback
}
