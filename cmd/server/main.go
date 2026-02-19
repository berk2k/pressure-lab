package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"pressure-lab/internal/idempotency"
	"pressure-lab/internal/queue"
	"pressure-lab/internal/ratelimit"
	httpTransport "pressure-lab/internal/transport/http"
	"pressure-lab/internal/worker"
)

func main() {

	q := queue.New(5)
	store := idempotency.New()
	limiter := ratelimit.New(3, time.Second)

	var wg sync.WaitGroup
	workerCount := 5
	wg.Add(workerCount)

	for i := 0; i < workerCount; i++ {
		w := worker.New(i+1, &wg, store)
		w.Start(q.Channel())
	}

	handler := httpTransport.New(q, limiter, store)

	mux := http.NewServeMux()
	mux.HandleFunc("/submit", handler.Submit)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	// Start server
	go func() {
		log.Println("listening on :8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen error: %v", err)
		}
	}()

	// Wait for shutdown signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop
	log.Println("shutdown signal received")

	// Stop HTTP admission
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("server shutdown error: %v\n", err)
	}

	// Close queue
	q.Close()

	// Wait workers
	wg.Wait()

	log.Println("server exited cleanly")
}
