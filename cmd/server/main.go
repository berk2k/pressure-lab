package main

import (
	"log"
	"net/http"
	"time"

	"pressure-lab/internal/idempotency"
	"pressure-lab/internal/queue"
	"pressure-lab/internal/ratelimit"
	httpTransport "pressure-lab/internal/transport/http"
	"pressure-lab/internal/worker"
)

func main() {
	q := queue.New(5)

	w := worker.New(1)
	w.Start(q.Channel())

	limiter := ratelimit.New(3, time.Second)
	store := idempotency.New()
	handler := httpTransport.New(q, limiter, store)

	mux := http.NewServeMux()
	mux.HandleFunc("/submit", handler.Submit)

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
