package settings

import "sync"

type RuntimeState struct {
	Path     string
	IP       string
	Port     string
	Ssl      bool
	SslPort  string
	HTTPAuth bool
	SearchWA bool
	PubIPv4  string
	PubIPv6  string
	TorAddr  string
	MaxSize  int64
}

type runtimeStateStore struct {
	mu    sync.RWMutex
	state RuntimeState
}

var defaultRuntimeStateStore runtimeStateStore

var (
	// DefaultRuntimeStateProvider is the legacy global runtime-state provider.
	// New composition code should pass it explicitly instead of calling GetRuntimeState from leaf packages.
	DefaultRuntimeStateProvider = GetRuntimeState

	// DefaultRuntimeStateUpdater is the legacy global runtime-state updater.
	// New composition code should pass it explicitly instead of calling UpdateRuntimeState from leaf packages.
	DefaultRuntimeStateUpdater = UpdateRuntimeState
)

func SetRuntimeState(state RuntimeState) {
	defaultRuntimeStateStore.set(state)
}

func GetRuntimeState() RuntimeState {
	return defaultRuntimeStateStore.get()
}

func ReplaceRuntimeStateForTests(state RuntimeState) func() {
	return defaultRuntimeStateStore.replaceForTests(state)
}

func UpdateRuntimeState(update func(*RuntimeState)) {
	defaultRuntimeStateStore.update(update)
}

func currentRuntimePath() string {
	return GetRuntimeState().PathConfig().Path
}

func (s *runtimeStateStore) set(state RuntimeState) {
	s.mu.Lock()
	s.state = state
	s.mu.Unlock()

	// Transitional compatibility for legacy call sites still reading package vars.
	syncLegacyRuntimeVars(state)
}

func (s *runtimeStateStore) get() RuntimeState {
	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()

	// Transitional fallback in case legacy code mutated package vars directly.
	if state == (RuntimeState{}) {
		state = legacyRuntimeStateSnapshot()
	}

	return state
}

func (s *runtimeStateStore) replaceForTests(state RuntimeState) func() {
	prev := s.get()
	s.set(state)

	return func() {
		s.set(prev)
	}
}

func (s *runtimeStateStore) update(update func(*RuntimeState)) {
	if update == nil {
		return
	}

	s.mu.Lock()
	state := s.state
	update(&state)
	s.state = state
	s.mu.Unlock()

	// Transitional compatibility for legacy call sites still reading package vars.
	syncLegacyRuntimeVars(state)
}
