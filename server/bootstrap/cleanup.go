package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"server/log"
	"server/settings"
)

var (
	cleanupReadDir     = os.ReadDir
	cleanupRemove      = os.Remove
	cleanupListTorrent = settings.ListTorrent
)

func runCacheCleanup(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}

	cfg := settings.BTsets
	if cfg == nil || !cfg.UseDisk || cfg.TorrentsSavePath == "/" || cfg.TorrentsSavePath == "" {
		return
	}

	dirs, err := cleanupReadDir(cfg.TorrentsSavePath)
	if err != nil {
		log.TLogln("Cache cleanup: read dir error:", err)

		return
	}

	torrs := cleanupListTorrent()
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

		if err := removeAllFiles(ctx, filepath.Join(cfg.TorrentsSavePath, d.Name())); err != nil {
			if !errors.Is(err, context.Canceled) {
				log.TLogln("Cache cleanup: remove dir error:", err)
			}

			return
		}
	}
}

func removeAllFiles(ctx context.Context, path string) error {
	files, err := cleanupReadDir(path)
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
			if err := removeAllFiles(ctx, name); err != nil {
				return err
			}

			continue
		}

		if err := cleanupRemove(name); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	if err := cleanupRemove(path); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}
