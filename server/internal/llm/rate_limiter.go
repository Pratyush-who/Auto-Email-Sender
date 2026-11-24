package llm

import (
	"context"
	"sync"
	"time"
)

// RateLimiter is a thread-safe token bucket rate limiter.
type RateLimiter struct {
	mu       sync.Mutex
	tokens   float64
	capacity float64
	refill   float64 // tokens per second
	last     time.Time
}

// NewRateLimiter creates a RateLimiter configured for a specific requests-per-minute (RPM).
func NewRateLimiter(rpm int) *RateLimiter {
	if rpm <= 0 {
		rpm = 30
	}
	refill := float64(rpm) / 60.0
	return &RateLimiter{
		tokens:   float64(rpm),
		capacity: float64(rpm),
		refill:   refill,
		last:     time.Now(),
	}
}

// Wait blocks until a token is available or context is cancelled.
func (rl *RateLimiter) Wait(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	for {
		rl.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(rl.last).Seconds()
		rl.tokens += elapsed * rl.refill
		if rl.tokens > rl.capacity {
			rl.tokens = rl.capacity
		}
		rl.last = now

		if rl.tokens >= 1.0 {
			rl.tokens -= 1.0
			rl.mu.Unlock()
			return nil
		}

		missing := 1.0 - rl.tokens
		waitDuration := time.Duration((missing / rl.refill) * float64(time.Second))
		rl.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDuration):
		}
	}
}
