// Package ratelimit provides a per-domain token-bucket rate limiter.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter controls request rates per domain using a token-bucket algorithm.
type Limiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	rate     time.Duration // minimum interval between requests per domain
	burst    int           // max burst tokens
}

type bucket struct {
	tokens   int
	lastTime time.Time
}

// New creates a Limiter that allows at most one request per `rate` duration
// per domain, with an initial burst capacity.
func New(rate time.Duration, burst int) *Limiter {
	return &Limiter{
		buckets: make(map[string]*bucket),
		rate:    rate,
		burst:   burst,
	}
}

// Wait blocks until a request to the given domain is permitted.
func (l *Limiter) Wait(domain string) {
	l.mu.Lock()

	b, ok := l.buckets[domain]
	if !ok {
		b = &bucket{tokens: l.burst, lastTime: time.Now()}
		l.buckets[domain] = b
	}

	// Refill tokens based on elapsed time.
	now := time.Now()
	elapsed := now.Sub(b.lastTime)
	refill := int(elapsed / l.rate)
	if refill > 0 {
		b.tokens += refill
		if b.tokens > l.burst {
			b.tokens = l.burst
		}
		b.lastTime = now
	}

	if b.tokens > 0 {
		b.tokens--
		l.mu.Unlock()
		return
	}

	// Calculate wait time until the next token.
	waitUntil := b.lastTime.Add(l.rate)
	l.mu.Unlock()

	time.Sleep(time.Until(waitUntil))

	l.mu.Lock()
	b.tokens = 0
	b.lastTime = time.Now()
	l.mu.Unlock()
}

// Stats returns the number of tracked domains.
func (l *Limiter) Stats() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}
