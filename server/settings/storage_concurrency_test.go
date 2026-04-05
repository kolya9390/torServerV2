package settings

import (
	"sync"
	"testing"
)

func TestStoragePreferencesConcurrentAccess(t *testing.T) {
	tmp := t.TempDir()
	Path = tmp
	ReadOnly = false

	globalBboltDBMu.Lock()
	globalBboltDB = nil
	globalBboltDBMu.Unlock()
	globalJsonDBMu.Lock()
	globalJsonDB = nil
	globalJsonDBMu.Unlock()

	if err := InitSets(false, false); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	t.Cleanup(func() {
		CloseDB()
		globalBboltDBMu.Lock()
		globalBboltDB = nil
		globalBboltDBMu.Unlock()
		globalJsonDBMu.Lock()
		globalJsonDB = nil
		globalJsonDBMu.Unlock()

		BTsets = nil
	})

	var wg sync.WaitGroup

	wg.Go(func() {

		for i := range 10 {
			if err := SwitchViewedStorage(i%2 == 0); err != nil {
				t.Errorf("switch viewed storage failed: %v", err)

				return
			}
		}
	})

	for range 4 {

		wg.Go(func() {

			for range 200 {
				_ = GetStoragePreferences()
			}
		})
	}

	wg.Wait()
}
