package settings

import (
	"encoding/json"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"server/log"
)

func SetBTSets(sets *BTSets) {
	if IsReadOnlyMode() {
		return
	}

	input := *sets

	sets.CoreProfile = normalizeCoreProfile(sets.CoreProfile)
	if sets.CoreProfile != "custom" {
		applyCoreProfilePreset(sets, sets.CoreProfile)
		applyCoreProfileOverrides(sets, &input)
	}

	sets.validateAndNormalize()

	cacheCfg := sets.CacheConfig()
	if cacheCfg.UseDisk && cacheCfg.SavePath != "" {
		resolveTorrentsSavePath(sets)
	} else if cacheCfg.SavePath == "" {
		sets.UseDisk = false
	}

	defaultBTsetsStore.set(sets)

	buf, err := json.Marshal(sets)
	if err != nil {
		log.TLogln("Error marshal btsets", err)

		return
	}

	tdb.Set("Settings", "BitTorr", buf)
}

// resolveTorrentsSavePath searches for a .tsc directory within the configured
// TorrentsSavePath and updates the path if found. It returns true when a valid
// path was provided (regardless of whether .tsc was found).
func resolveTorrentsSavePath(sets *BTSets) bool {
	cacheCfg := sets.CacheConfig()
	if cacheCfg.SavePath == "" {
		return false
	}

	if err := filepath.WalkDir(cacheCfg.SavePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() && strings.ToLower(d.Name()) == ".tsc" {
			sets.TorrentsSavePath = path
			log.TLogln("Find directory \"" + sets.TorrentsSavePath + "\", use as cache dir")

			return io.EOF
		}

		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}

		return nil
	}); err != nil {
		log.TLogln("Error walking torrents save path:", err)
	}

	return true
}
