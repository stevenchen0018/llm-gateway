package redis

import "sync"

// reservationStore is a process-local map from opaque reservation ID to the
// Redis bucket key + amount it reserved. It only needs to survive the
// lifetime of a single in-flight request (reserve -> settle/release happens
// within one handler call), so it does not need to be shared across
// instances or survive a restart.
type reservationStore struct {
	mu    sync.Mutex
	items map[string]reservation
}

func newReservationStore() *reservationStore {
	return &reservationStore{items: make(map[string]reservation)}
}

func (s *reservationStore) put(id string, r reservation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[id] = r
}

func (s *reservationStore) take(id string) (reservation, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.items[id]
	if ok {
		delete(s.items, id)
	}
	return r, ok
}
