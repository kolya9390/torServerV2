package api

import "sync"

type servicesRegistry struct {
	mu       sync.RWMutex
	services *APIServices
}

var defaultServicesRegistry servicesRegistry

func (r *servicesRegistry) Set(s *APIServices) {
	if s == nil {
		return
	}

	s = withNoopFallbacks(s)

	r.mu.Lock()
	r.services = s
	r.mu.Unlock()
}

func (r *servicesRegistry) Get() *APIServices {
	r.mu.RLock()
	s := r.services
	r.mu.RUnlock()

	if s != nil {
		return withNoopFallbacks(s)
	}

	noop := newNoopServices()

	r.mu.Lock()
	if r.services == nil {
		r.services = noop
	} else {
		noop = withNoopFallbacks(r.services)
		r.services = noop
	}
	r.mu.Unlock()

	return noop
}

func (r *servicesRegistry) ReplaceForTests(s *APIServices) func() {
	r.mu.Lock()
	prev := r.services
	if s == nil {
		r.services = nil
	} else {
		r.services = withNoopFallbacks(s)
	}
	r.mu.Unlock()

	return func() {
		r.mu.Lock()
		r.services = prev
		r.mu.Unlock()
	}
}
