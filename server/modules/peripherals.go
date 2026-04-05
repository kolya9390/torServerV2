package modules

import (
	"sync"

	"server/dlna"
	"server/log"
	"server/torrfs/fuse"
)

var (
	dlnaMu sync.Mutex
	fuseMu sync.Mutex
)

func RestartDLNA(enable bool) error {
	dlnaMu.Lock()
	defer dlnaMu.Unlock()

	dlna.Stop()

	if !enable {
		return nil
	}

	return startWithPolicy("dlna", dlna.Start, DefaultPolicy())
}

func StopDLNA() {
	dlnaMu.Lock()
	defer dlnaMu.Unlock()
	safeStop("dlna", dlna.Stop)
}

func StartFUSE() error {
	fuseMu.Lock()
	defer fuseMu.Unlock()

	return startWithPolicy("fuse", fuse.FuseAutoMount, DefaultPolicy())
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
