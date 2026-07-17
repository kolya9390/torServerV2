package cliapp

import (
	"github.com/spf13/cobra"

	buildversion "server/version"
)

const localBinaryVersionSource = "local_binary"

type localVersionView struct {
	Source        string                   `json:"source"`
	Version       string                   `json:"version"`
	Commit        string                   `json:"commit"`
	BuildTime     string                   `json:"build_time"`
	Dirty         buildversion.DirtyState  `json:"dirty"`
	GoVersion     string                   `json:"go_version"`
	OS            string                   `json:"os"`
	Arch          string                   `json:"arch"`
	TorrentEngine torrentEngineVersionView `json:"torrent_engine"`
}

type torrentEngineVersionView struct {
	Path               string `json:"path"`
	Version            string `json:"version"`
	ModuleVersion      string `json:"module_version,omitempty"`
	ReplacementPath    string `json:"replacement_path,omitempty"`
	ReplacementVersion string `json:"replacement_version,omitempty"`
	LocalReplacement   bool   `json:"local_replacement,omitempty"`
}

func newVersionCmd(opts *globalOptions, info buildversion.Info) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Показать информацию о локальном бинарнике",
		Long: "Показывает build information локального бинарника без подключения к серверу. " +
			"Для remote API и readiness используйте status.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWithOutput(cmd, opts, func(resolved globalOptions) error {
				return writeLocalVersion(resolved, info)
			})
		},
	}
}

func writeLocalVersion(opts globalOptions, info buildversion.Info) error {
	view := newLocalVersionView(info)
	if opts.Output == outputJSON {
		return writeJSONSuccess(opts.stdoutWriter(), view)
	}

	return writeTextLines(
		opts.stdoutWriter(),
		"Source: local binary",
		"Application version: "+view.Version,
		"Commit: "+view.Commit,
		"Build time: "+view.BuildTime,
		"Dirty: "+string(view.Dirty),
		"Go: "+view.GoVersion,
		"Platform: "+view.OS+"/"+view.Arch,
		"Torrent engine: "+view.TorrentEngine.Version,
	)
}

func newLocalVersionView(info buildversion.Info) localVersionView {
	return localVersionView{
		Source:    localBinaryVersionSource,
		Version:   info.Version,
		Commit:    info.Commit,
		BuildTime: info.BuildTime,
		Dirty:     info.Dirty,
		GoVersion: info.GoVersion,
		OS:        info.OS,
		Arch:      info.Arch,
		TorrentEngine: torrentEngineVersionView{
			Path:               info.TorrentEngine.Path,
			Version:            info.TorrentEngine.EffectiveVersion(),
			ModuleVersion:      info.TorrentEngine.Version,
			ReplacementPath:    info.TorrentEngine.ReplacementPath,
			ReplacementVersion: info.TorrentEngine.ReplacementVersion,
			LocalReplacement:   info.TorrentEngine.LocalReplacement,
		},
	}
}
