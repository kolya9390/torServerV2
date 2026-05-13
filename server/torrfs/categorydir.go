package torrfs

import (
	"io/fs"
	"time"

	"server/settings"
	"server/torr"
)

type CategoryDir struct {
	info     fs.FileInfo
	provider settings.SettingsProvider
}

func NewCategoryDir(category string, provider settings.SettingsProvider) *CategoryDir {
	if category == "" {
		category = "other"
	}

	if provider == nil {
		provider = settings.NewNoopSettingsProvider()
	}

	d := &CategoryDir{
		info: info{
			name:  category,
			size:  4096,
			mode:  0o555,
			mtime: time.Unix(477033666, 0),
			isDir: true,
		},
		provider: provider,
	}

	return d
}

func (d *CategoryDir) Stat() (fs.FileInfo, error) {
	return d.info, nil
}

func (d *CategoryDir) ReadDir(n int) ([]fs.DirEntry, error) {
	nodes := []fs.DirEntry{}

	torrs := getCatalog().ListTorrents()
	for _, t := range torrs {
		if t.Category == "" {
			t.Category = "other"
		}

		if t.Category == d.Name() {
			if d.provider.Get().DLNAConfig().ShowFSActiveTorr && !t.GotInfo() {
				continue
			}

			td := NewTorrDir(nil, t.Title, t, d.provider)
			nodes = append(nodes, td)
		}
	}

	return nodes, nil
}

// INode.
func (d *CategoryDir) Open(name string) (fs.File, error) { return Open(d, name) }
func (d *CategoryDir) Parent() INode                     { return nil }
func (d *CategoryDir) Torrent() *torr.Torrent            { return nil }
func (d *CategoryDir) SetTorrent(_ *torr.Torrent)        {}

// DirEntry.
func (d *CategoryDir) Name() string { return d.info.Name() }
func (d *CategoryDir) IsDir() bool  { return true }
func (d *CategoryDir) Type() fs.FileMode {
	return d.info.Mode()
}
func (d *CategoryDir) Info() (fs.FileInfo, error) { return d.info, nil }

// File.
func (d *CategoryDir) Read(bytes []byte) (int, error) { return 0, fs.ErrInvalid }
func (d *CategoryDir) Close() error                   { return nil }
