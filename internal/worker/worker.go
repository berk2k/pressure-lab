package worker

import (
	"log"
	"pressure-lab/internal/domain"
	"pressure-lab/internal/idempotency"
	"sync"
	"time"
)

type Worker struct {
	id    int
	wg    *sync.WaitGroup
	store *idempotency.Store
}

func New(id int, wg *sync.WaitGroup, store *idempotency.Store) *Worker {
	return &Worker{
		id:    id,
		wg:    wg,
		store: store,
	}
}

func (w *Worker) Start(ch <-chan domain.Task) {
	go func() {
		defer w.wg.Done()

		for task := range ch {

			log.Printf("[worker-%d] processing task %s\n", w.id, task.ID)

			time.Sleep(500 * time.Millisecond)

			w.store.SetProcessed(task.ID)
		}

		log.Printf("[worker-%d] exiting\n", w.id)
	}()
}
