package version

import (
	"runtime/debug"

	"server/log"
)

// version is set at build time via -ldflags "-X server/version.version=<tag>".
var version = "dev"

// Version returns the current server version.
func Version() string {
	return version
}

// GetTorrentVersion returns the version of the underlying torrent library.
func GetTorrentVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		log.TLogln("Failed to read build info")

		return ""
	}

	for _, dep := range bi.Deps {
		if dep.Path == "github.com/anacrolix/torrent" {
			if dep.Replace != nil {
				return dep.Replace.Version
			}

			return dep.Version
		}
	}

	return ""
}
