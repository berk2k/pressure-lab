package ratelimit

import (
	"sync"
	"time"
)

type Limiter struct {
	mu        sync.Mutex
	tokens    int
	maxTokens int
	refill    time.Duration
}

func New(maxTokens int, refill time.Duration) *Limiter {
	l := &Limiter{
		tokens:    maxTokens,
		maxTokens: maxTokens,
		refill:    refill,
	}

	go l.refillLoop()

	return l
}

func (l *Limiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock() // execute after func is done

	if l.tokens > 0 {
		l.tokens--
		return true
	}

	return false
}

func (l *Limiter) refillLoop() {
	ticker := time.NewTicker(l.refill)
	for range ticker.C {
		l.mu.Lock()
		l.tokens = l.maxTokens
		l.mu.Unlock()
	}
}
