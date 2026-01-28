package worker

import (
	"log"
	"time"

	"pressure-lab/internal/domain"
)

type Worker struct {
	id int
}

func New(id int) *Worker {
	return &Worker{id: id}
}

func (w *Worker) Start(tasks <-chan domain.Task) {
	go func() {
		for task := range tasks {
			log.Printf("[worker-%d] processing task %d\n", w.id, task.ID)
			time.Sleep(500 * time.Millisecond)
		}
	}()
}
