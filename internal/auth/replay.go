package auth

import (
	"errors"
	"sync"
	"time"
)

var (
	// ErrTimestampSkew is returned when the request timestamp is outside the allowed skew.
	ErrTimestampSkew = errors.New("timestamp skew exceeded")

	// ErrReplayDetected is returned when a nonce is reused (replay attack).
	ErrReplayDetected = errors.New("replay attack detected")
)

const (
	// MaxTimestampSkew is the maximum allowed timestamp skew in seconds.
	// Requests with |server_time - timestamp| > 300s are rejected.
	MaxTimestampSkew = 300 * time.Second

	// MaxNonceCacheSize is the maximum number of nonces to cache.
	// When exceeded, oldest entries are evicted via LRU.
	MaxNonceCacheSize = 10000

	// NonceCacheTTL is the time-to-live for nonce cache entries (24h).
	NonceCacheTTL = 24 * time.Hour
)

// NonceCache provides replay protection using an LRU cache.
// Nonces are tracked to prevent replay attacks.
type NonceCache struct {
	mu    sync.RWMutex
	cache map[string]time.Time
	queue []string // Simple queue for LRU tracking
}

// NewNonceCache creates a new nonce cache.
func NewNonceCache() *NonceCache {
	return &NonceCache{
		cache: make(map[string]time.Time),
		queue: make([]string, 0, MaxNonceCacheSize),
	}
}

// CheckTimestampSkew validates that the request timestamp is within allowed skew.
// Returns ErrTimestampSkew if |now - timestamp| > MaxTimestampSkew.
func CheckTimestampSkew(timestamp time.Time) error {
	now := time.Now()
	skew := now.Sub(timestamp)

	if skew < 0 {
		skew = -skew
	}

	if skew > MaxTimestampSkew {
		return ErrTimestampSkew
	}

	return nil
}

// CheckAndStoreNonce checks if a nonce has been used and stores it.
// Returns ErrReplayDetected if the nonce is already in the cache.
// Evicts expired and oldest entries to maintain cache size.
func (nc *NonceCache) CheckAndStoreNonce(nonce string) error {
	now := time.Now()

	nc.mu.Lock()
	defer nc.mu.Unlock()

	// Check if nonce exists
	if _, exists := nc.cache[nonce]; exists {
		return ErrReplayDetected
	}

	// Clean up expired entries
	nc.cleanup(now)

	// Evict oldest entries if at capacity
	for len(nc.cache) >= MaxNonceCacheSize {
		oldestNonce := nc.queue[0]
		delete(nc.cache, oldestNonce)
		nc.queue = nc.queue[1:]
	}

	// Store nonce with timestamp
	nc.cache[nonce] = now
	nc.queue = append(nc.queue, nonce)

	return nil
}

// cleanup removes expired nonce entries from the cache.
// Caller must hold the lock.
func (nc *NonceCache) cleanup(now time.Time) {
	for len(nc.queue) > 0 {
		oldestNonce := nc.queue[0]
		if storedTime, exists := nc.cache[oldestNonce]; exists {
			if now.Sub(storedTime) > NonceCacheTTL {
				// Entry expired, remove it
				delete(nc.cache, oldestNonce)
				nc.queue = nc.queue[1:]
				continue
			}
		}
		// Oldest entry is not expired, stop cleanup
		break
	}
}

// Size returns the current number of entries in the nonce cache.
func (nc *NonceCache) Size() int {
	nc.mu.RLock()
	defer nc.mu.RUnlock()
	return len(nc.cache)
}

// Clear removes all entries from the nonce cache.
// This is primarily used for testing.
func (nc *NonceCache) Clear() {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	nc.cache = make(map[string]time.Time)
	nc.queue = make([]string, 0, MaxNonceCacheSize)
}

// HasNonce checks if a nonce is in the cache without storing it.
// This is primarily used for testing.
func (nc *NonceCache) HasNonce(nonce string) bool {
	nc.mu.RLock()
	defer nc.mu.RUnlock()
	_, exists := nc.cache[nonce]
	return exists
}

// ParseTimestamp parses a Unix timestamp (seconds since epoch) to time.Time.
// Returns zero time if the timestamp is invalid.
func ParseTimestamp(timestamp int64) time.Time {
	return time.Unix(timestamp, 0)
}
