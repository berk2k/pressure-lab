package queue

import "pressure-lab/internal/domain"

type InMemoryQueue struct {
	ch chan domain.Task
}

func New(size int) *InMemoryQueue {
	return &InMemoryQueue{
		ch: make(chan domain.Task, size),
	}
}

// non-blocking enqueue
func (q *InMemoryQueue) TryEnqueue(task domain.Task) bool {
	select {
	case q.ch <- task:
		return true
	default:
		return false
	}
}

func (q *InMemoryQueue) Channel() <-chan domain.Task {
	return q.ch
}
