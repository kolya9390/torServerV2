package modules

import (
	"sync"

	"server/dlna"
	"server/log"
	"server/settings"
	"server/torrfs/fuse"
)

var (
	dlnaMu sync.Mutex
	fuseMu sync.Mutex
)

func RestartDLNA(enable bool) error {
	return RestartDLNAWithProviders(enable, nil, nil)
}

func RestartDLNAWithProviders(enable bool, provider settings.SettingsProvider, argsProvider settings.ArgsProvider) error {
	dlnaMu.Lock()
	defer dlnaMu.Unlock()

	dlna.Stop()

	if !enable {
		return nil
	}

	return startWithPolicy("dlna", func() error {
		return dlna.StartWithProviders(provider, argsProvider)
	}, DefaultPolicy())
}

func StopDLNA() {
	dlnaMu.Lock()
	defer dlnaMu.Unlock()
	safeStop("dlna", dlna.Stop)
}

func StartFUSE() error {
	return StartFUSEWithProviders(nil, nil)
}

func StartFUSEWithProviders(provider settings.SettingsProvider, argsProvider settings.ArgsProvider) error {
	fuseMu.Lock()
	defer fuseMu.Unlock()

	return startWithPolicy("fuse", func() error {
		return fuse.FuseAutoMountWithProviders(provider, argsProvider)
	}, DefaultPolicy())
}

func StopFUSE() {
	fuseMu.Lock()
	defer fuseMu.Unlock()
	safeStop("fuse", fuse.FuseCleanup)
}

func LogPeripheralFailure(module string, err error) {
	if err != nil {
		log.TLogln("peripheral module degraded", module, "error", err)
	}
}
