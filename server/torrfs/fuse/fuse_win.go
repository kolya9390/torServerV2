//go:build windows

package fuse

import (
	"server/log"
	"server/settings"
)

func FuseAutoMount() error {
	return FuseAutoMountWithProviders(nil, nil)
}

func FuseAutoMountWithProviders(provider settings.SettingsProvider, argsProvider settings.ArgsProvider) error {
	args := newFuseRuntimeContext(provider, argsProvider).currentArgs()
	if args != nil && args.FusePath != "" {
		log.TLogln("Windows not support FUSE")
	}
	return nil
}

func FuseCleanup() {
}
