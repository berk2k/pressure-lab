package http

import (
	"net/http"
	"pressure-lab/internal/domain"
	"pressure-lab/internal/queue"
	"pressure-lab/internal/ratelimit"
	"strconv"
	"time"
)

type Handler struct {
	queue   *queue.InMemoryQueue
	limiter *ratelimit.Limiter
}

func New(q *queue.InMemoryQueue, l *ratelimit.Limiter) *Handler {
	return &Handler{
		queue:   q,
		limiter: l,
	}
}

func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {

	if !h.limiter.Allow() {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("rate limit exceeded\n"))
		return
	}

	task := domain.Task{
		ID: time.Now().UnixNano(),
	}

	ok := h.queue.TryEnqueue(task)
	if !ok {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("queue full\n"))
		return
	}

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("accepted task " + strconv.FormatInt(task.ID, 10) + "\n"))

}
