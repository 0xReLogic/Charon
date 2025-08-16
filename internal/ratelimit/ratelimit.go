package ratelimit

import (
	"sync"
	"time"
)

// TokenBucket implements token bucket rate limiting
type TokenBucket struct {
	capacity   int // maximum tokens
	tokens     int // current tokens
	refillRate int // tokens per second
	lastRefill time.Time
	mu         sync.Mutex
}

// NewTokenBucket creates a new token bucket rate limiter
func NewTokenBucket(capacity, refillRate int) *TokenBucket {
	return &TokenBucket{
		capacity:   capacity,
		tokens:     capacity, // start full
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow checks if a request is allowed (consumes 1 token if available)
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)

	// Refill tokens based on elapsed time
	if elapsed > 0 {
		tokensToAdd := int(elapsed.Seconds() * float64(tb.refillRate))
		tb.tokens += tokensToAdd
		if tb.tokens > tb.capacity {
			tb.tokens = tb.capacity
		}
		tb.lastRefill = now
	}

	// Try to consume 1 token
	if tb.tokens > 0 {
		tb.tokens--
		return true
	}
	return false
}

// RateLimiter manages multiple token buckets for different routes
type RateLimiter struct {
	buckets map[string]*TokenBucket
	mu      sync.RWMutex

	// Default settings
	defaultRPS   int
	defaultBurst int
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(defaultRPS, defaultBurst int) *RateLimiter {
	return &RateLimiter{
		buckets:      make(map[string]*TokenBucket),
		defaultRPS:   defaultRPS,
		defaultBurst: defaultBurst,
	}
}

// Allow checks if a request for the given route is allowed
func (rl *RateLimiter) Allow(route string) bool {
	rl.mu.RLock()
	bucket, exists := rl.buckets[route]
	rl.mu.RUnlock()

	if !exists {
		rl.mu.Lock()
		// Double-check after acquiring write lock
		if bucket, exists = rl.buckets[route]; !exists {
			bucket = NewTokenBucket(rl.defaultBurst, rl.defaultRPS)
			rl.buckets[route] = bucket
		}
		rl.mu.Unlock()
	}

	return bucket.Allow()
}
