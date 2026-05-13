package settings

import (
	"sync"
	"testing"
)

func TestStoragePreferencesConcurrentAccess(t *testing.T) {
	tmp := t.TempDir()
	restoreRuntime := ReplaceRuntimeStateForTests(RuntimeState{Path: tmp})
	restoreReadOnly := ReplaceReadOnlyForTests(false)

	globalBboltDBMu.Lock()
	globalBboltDB = nil
	globalBboltDBMu.Unlock()
	globalJSONDBMu.Lock()
	globalJSONDB = nil
	globalJSONDBMu.Unlock()

	if err := InitSets(false, false); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	t.Cleanup(func() {
		restoreRuntime()
		restoreReadOnly()
		CloseDB()
		globalBboltDBMu.Lock()
		globalBboltDB = nil
		globalBboltDBMu.Unlock()
		globalJSONDBMu.Lock()
		globalJSONDB = nil
		globalJSONDBMu.Unlock()

		defaultBTsetsStore.set(nil)
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
