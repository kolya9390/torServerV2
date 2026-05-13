package settings

import "sync"

type btsetsStore struct {
	mu   sync.RWMutex
	sets *BTSets
}

var defaultBTsetsStore = &btsetsStore{}

func (s *btsetsStore) get() *BTSets {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.sets
}

func (s *btsetsStore) set(sets *BTSets) {
	s.mu.Lock()
	s.sets = sets
	s.mu.Unlock()
}

func (s *btsetsStore) replace(sets *BTSets) (prev *BTSets) {
	s.mu.Lock()
	prev = s.sets
	s.sets = sets
	s.mu.Unlock()

	return prev
}

// ReplaceSettingsForTests swaps the in-memory settings snapshot and returns a restore func.
func ReplaceSettingsForTests(sets *BTSets) func() {
	prev := defaultBTsetsStore.replace(sets)

	return func() {
		defaultBTsetsStore.set(prev)
	}
}

func currentStoredSettings() *BTSets {
	return defaultBTsetsStore.get()
}
