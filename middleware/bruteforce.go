package middleware

import (
	"log"
	"sync"
	"time"
)

const cleanupPeriod = time.Minute

type BruteForce struct {
	maxAttempts int
	banDuration time.Duration

	mu       sync.Mutex
	attempts map[string]*attemptInfo
}

type attemptInfo struct {
	count    int
	lastTry  time.Time
	bannedAt time.Time
	banned   bool
}

func NewBruteForce(maxAttempts int, banDuration time.Duration) *BruteForce {
	bf := &BruteForce{
		maxAttempts: maxAttempts,
		banDuration: banDuration,
		attempts:    make(map[string]*attemptInfo),
	}
	go bf.cleanup()
	return bf
}

func (bf *BruteForce) IsBanned(ip string) bool {
	bf.mu.Lock()
	defer bf.mu.Unlock()

	info, exists := bf.attempts[ip]
	if !exists || !info.banned {
		return false
	}

	if time.Since(info.bannedAt) > bf.banDuration {
		delete(bf.attempts, ip)
		return false
	}
	return true
}

func (bf *BruteForce) RecordFailure(ip string) {
	bf.mu.Lock()
	defer bf.mu.Unlock()

	info, exists := bf.attempts[ip]
	if !exists {
		info = &attemptInfo{}
		bf.attempts[ip] = info
	}

	info.count++
	info.lastTry = time.Now()

	if !info.banned && info.count >= bf.maxAttempts {
		info.banned = true
		info.bannedAt = info.lastTry
		log.Printf("IP %s banned for %v after %d failed attempts", ip, bf.banDuration, info.count)
	}
}

func (bf *BruteForce) ResetFailures(ip string) {
	bf.mu.Lock()
	defer bf.mu.Unlock()

	delete(bf.attempts, ip)
}

func (bf *BruteForce) cleanup() {
	ticker := time.NewTicker(cleanupPeriod)
	defer ticker.Stop()

	for now := range ticker.C {
		bf.mu.Lock()
		for ip, info := range bf.attempts {
			expiredBan := info.banned && now.Sub(info.bannedAt) > bf.banDuration
			stale := now.Sub(info.lastTry) > bf.banDuration*2
			if expiredBan || stale {
				delete(bf.attempts, ip)
			}
		}
		bf.mu.Unlock()
	}
}
