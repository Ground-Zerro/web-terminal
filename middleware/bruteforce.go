package middleware

import (
	"log"
	"sync"
	"time"
)

// BruteForce protects against brute force attacks
type BruteForce struct {
	maxAttempts int
	banDuration time.Duration
	attempts    map[string]*AttemptInfo
	mu          sync.RWMutex
}

// AttemptInfo tracks login attempts
type AttemptInfo struct {
	Count     int
	LastTry   time.Time
	BannedAt  time.Time
	IsBanned  bool
}

// NewBruteForce creates a new brute force protector
func NewBruteForce(maxAttempts int, banDuration time.Duration) *BruteForce {
	bf := &BruteForce{
		maxAttempts: maxAttempts,
		banDuration: banDuration,
		attempts:    make(map[string]*AttemptInfo),
	}

	// Start cleanup goroutine
	go bf.cleanup()

	return bf
}

// IsBanned checks if an IP is banned
func (bf *BruteForce) IsBanned(ip string) bool {
	bf.mu.RLock()
	defer bf.mu.RUnlock()

	info, exists := bf.attempts[ip]
	if !exists {
		return false
	}

	if info.IsBanned {
		// Check if ban has expired
		if time.Since(info.BannedAt) > bf.banDuration {
			return false
		}
		return true
	}

	return false
}

// RecordFailure records a failed login attempt
func (bf *BruteForce) RecordFailure(ip string) {
	bf.mu.Lock()
	defer bf.mu.Unlock()

	info, exists := bf.attempts[ip]
	if !exists {
		info = &AttemptInfo{}
		bf.attempts[ip] = info
	}

	info.Count++
	info.LastTry = time.Now()

	// Check if should ban
	if info.Count >= bf.maxAttempts {
		info.IsBanned = true
		info.BannedAt = time.Now()
		log.Printf("IP %s banned for %v after %d failed attempts", ip, bf.banDuration, info.Count)
	}
}

// ResetFailures resets failed attempts for an IP
func (bf *BruteForce) ResetFailures(ip string) {
	bf.mu.Lock()
	defer bf.mu.Unlock()

	delete(bf.attempts, ip)
}

// cleanup removes expired entries
func (bf *BruteForce) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		bf.mu.Lock()
		now := time.Now()
		for ip, info := range bf.attempts {
			// Remove expired bans
			if info.IsBanned && now.Sub(info.BannedAt) > bf.banDuration {
				delete(bf.attempts, ip)
				continue
			}
			// Remove old attempts
			if now.Sub(info.LastTry) > bf.banDuration*2 {
				delete(bf.attempts, ip)
			}
		}
		bf.mu.Unlock()
	}
}
