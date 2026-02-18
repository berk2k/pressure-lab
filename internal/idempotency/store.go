package idempotency

import "sync"

type Store struct {
	mu   sync.Mutex
	seen map[string]bool
}

func New() *Store {
	return &Store{
		seen: make(map[string]bool),
	}
}

func (s *Store) Exists(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seen[id]
}

func (s *Store) Mark(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen[id] = true
}
