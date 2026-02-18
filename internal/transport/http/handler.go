package http

import (
	"net/http"
	"pressure-lab/internal/domain"
	"pressure-lab/internal/idempotency"
	"pressure-lab/internal/queue"
	"pressure-lab/internal/ratelimit"
)

type Handler struct {
	queue   *queue.InMemoryQueue
	limiter *ratelimit.Limiter
	store   *idempotency.Store
}

func New(q *queue.InMemoryQueue, l *ratelimit.Limiter, s *idempotency.Store) *Handler {
	return &Handler{
		queue:   q,
		limiter: l,
		store:   s,
	}
}

func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {

	id := r.Header.Get("Idempotency-Key")
	if id == "" {
		http.Error(w, "missing Idempotency-Key", http.StatusBadRequest)
		return
	}

	if h.store.Exists(id) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("already processed\n"))
		return
	}

	if !h.limiter.Allow() {
		http.Error(w, "rate limit exceeded\n", http.StatusTooManyRequests)
		return
	}

	task := domain.Task{
		ID: id,
	}

	ok := h.queue.TryEnqueue(task)
	if !ok {
		http.Error(w, "queue full\n", http.StatusTooManyRequests)
		return
	}

	h.store.Mark(id)

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("accepted\n"))
}
