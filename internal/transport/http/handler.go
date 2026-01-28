package http

import (
	"net/http"
	"pressure-lab/internal/domain"
	"pressure-lab/internal/queue"
	"strconv"
	"time"
)

type Handler struct {
	queue *queue.InMemoryQueue
}

func New(q *queue.InMemoryQueue) *Handler {
	return &Handler{queue: q}
}

func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
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
