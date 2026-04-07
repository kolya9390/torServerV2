package torrfs

import (
	"io/fs"
	"strings"

	"server/torr"
)

type INode interface {
	fs.ReadDirFile
	fs.DirEntry

	Open(name string) (fs.File, error)

	Parent() INode

	Torrent() *torr.Torrent
	SetTorrent(torr *torr.Torrent)
}

func Open(d INode, name string) (fs.File, error) {
	trimPath := strings.TrimPrefix(name, d.Name())
	trimPath = strings.TrimSuffix(trimPath, "/")
	trimPath = strings.TrimPrefix(trimPath, "/")

	if trimPath == "" {
		return d, nil
	}

	arr := strings.Split(trimPath, "/")
	if len(arr) == 0 {
		return nil, fs.ErrNotExist
	}

	dirs, err := d.ReadDir(-1)
	if err != nil {
		return nil, fs.ErrNotExist
	}

	for _, dir := range dirs {
		if dir.Name() == arr[0] {
			if inode, ok := dir.(INode); ok {
				return inode.Open(trimPath)
			}

			return nil, fs.ErrNotExist
		}
	}

	return nil, fs.ErrNotExist
}
