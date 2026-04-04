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

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			if err := SwitchViewedStorage(i%2 == 0); err != nil {
				t.Errorf("switch viewed storage failed: %v", err)
				return
			}
		}
	}()

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = GetStoragePreferences()
			}
		}()
	}

	wg.Wait()
}
