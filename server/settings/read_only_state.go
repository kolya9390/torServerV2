package settings

import "sync"

type readOnlyStateStore struct {
	mu       sync.RWMutex
	readOnly bool
}

var defaultReadOnlyStateStore readOnlyStateStore

func SetReadOnly(readOnly bool) {
	defaultReadOnlyStateStore.set(readOnly)
}

func IsReadOnlyMode() bool {
	return defaultReadOnlyStateStore.get()
}

func ReplaceReadOnlyForTests(readOnly bool) func() {
	return defaultReadOnlyStateStore.replaceForTests(readOnly)
}

func (s *readOnlyStateStore) set(readOnly bool) {
	s.mu.Lock()
	s.readOnly = readOnly
	s.mu.Unlock()
}

func (s *readOnlyStateStore) get() bool {
	s.mu.RLock()
	readOnly := s.readOnly
	s.mu.RUnlock()

	return readOnly
}

func (s *readOnlyStateStore) replaceForTests(readOnly bool) func() {
	prev := s.get()
	s.set(readOnly)

	return func() {
		s.set(prev)
	}
}
