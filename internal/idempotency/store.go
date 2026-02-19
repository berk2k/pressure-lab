package idempotency

import "sync"

type Status int

const (
	Processing Status = iota
	Processed
)

type Store struct {
	mu   sync.Mutex
	data map[string]Status
}

func New() *Store {
	return &Store{
		data: make(map[string]Status),
	}
}

func (s *Store) Get(id string) (Status, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	status, ok := s.data[id]
	return status, ok
}

func (s *Store) SetProcessing(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[id] = Processing
}

func (s *Store) SetProcessed(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[id] = Processed
}
