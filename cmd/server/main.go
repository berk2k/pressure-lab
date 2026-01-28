package main

import (
	"log"
	"net/http"

	"pressure-lab/internal/queue"
	httpTransport "pressure-lab/internal/transport/http"
	"pressure-lab/internal/worker"
)

func main() {
	q := queue.New(5)

	w := worker.New(1)
	w.Start(q.Channel())

	handler := httpTransport.New(q)

	mux := http.NewServeMux()
	mux.HandleFunc("/submit", handler.Submit)

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
