package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"server/log"
	"server/settings"
)

type cacheCleanupDeps struct {
	settingsProvider settings.SettingsProvider
	readDir          func(string) ([]os.DirEntry, error)
	remove           func(string) error
	listTorrents     func() []*settings.TorrentDB
}

func defaultCacheCleanupDeps() cacheCleanupDeps {
	return cacheCleanupDeps{
		settingsProvider: settings.DefaultSettingsProvider,
		readDir:          os.ReadDir,
		remove:           os.Remove,
		listTorrents:     settings.ListTorrent,
	}
}

func runCacheCleanup(ctx context.Context) {
	runCacheCleanupWithDeps(ctx, defaultCacheCleanupDeps())
}

func runCacheCleanupWithDeps(ctx context.Context, deps cacheCleanupDeps) {
	if ctx == nil {
		ctx = context.Background()
	}

	if deps.settingsProvider == nil {
		deps.settingsProvider = settings.NewNoopSettingsProvider()
	}

	if deps.readDir == nil {
		deps.readDir = os.ReadDir
	}

	if deps.remove == nil {
		deps.remove = os.Remove
	}

	if deps.listTorrents == nil {
		deps.listTorrents = func() []*settings.TorrentDB { return nil }
	}

	cfg := deps.settingsProvider.Get()
	if cfg == nil || !cfg.UseDisk || cfg.TorrentsSavePath == "/" || cfg.TorrentsSavePath == "" {
		return
	}

	dirs, err := deps.readDir(cfg.TorrentsSavePath)
	if err != nil {
		log.TLogln("Cache cleanup: read dir error:", err)

		return
	}

	torrs := deps.listTorrents()
	active := make(map[string]struct{}, len(torrs))

	for _, t := range torrs {
		active[t.InfoHash.HexString()] = struct{}{}
	}

	log.TLogln("Remove unused cache in dir:", cfg.TorrentsSavePath)

	for _, d := range dirs {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if len(d.Name()) != 40 || !d.IsDir() {
			continue
		}

		shouldDelete := cfg.RemoveCacheOnDrop
		if !shouldDelete {
			_, exists := active[d.Name()]
			shouldDelete = !exists
		}

		if !shouldDelete {
			continue
		}

		log.TLogln("Remove unused cache:", d.Name())

		if err := removeAllFiles(ctx, filepath.Join(cfg.TorrentsSavePath, d.Name()), deps); err != nil {
			if !errors.Is(err, context.Canceled) {
				log.TLogln("Cache cleanup: remove dir error:", err)
			}

			return
		}
	}
}

func removeAllFiles(ctx context.Context, path string, deps cacheCleanupDeps) error {
	files, err := deps.readDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return err
	}

	for _, f := range files {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		name := filepath.Join(path, f.Name())
		if f.IsDir() {
			if err := removeAllFiles(ctx, name, deps); err != nil {
				return err
			}

			continue
		}

		if err := deps.remove(name); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	if err := deps.remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}
