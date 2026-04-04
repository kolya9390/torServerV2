//go:build windows
// +build windows

package fuse

import (
	"server/log"
	"server/settings"
)

func FuseAutoMount() error {
	args := settings.GetArgs()
	if args != nil && args.FusePath != "" {
		log.TLogln("Windows not support FUSE")
	}
	return nil
}

func FuseCleanup() {
}
