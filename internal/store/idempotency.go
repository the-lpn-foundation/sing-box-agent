package store

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"
)

const (
	DefaultIdempotencyTTL     = 24 * time.Hour
	DefaultIdempotencyMaxKeys = 10_000
)

var (
	ErrIdempotencyConflict  = errors.New("idempotency key already used with different payload")
	ErrIdempotencyStoreFull = errors.New("idempotency store is full")
)

type StoredResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

type idempotencyEntry struct {
	requestHash string
	response    StoredResponse
	expiresAt   time.Time
}

type IdempotencyStore struct {
	mu      sync.Mutex
	entries map[string]idempotencyEntry
	ttl     time.Duration
	maxKeys int
	nowFn   func() time.Time
}

func NewIdempotencyStore() *IdempotencyStore {
	return NewIdempotencyStoreWithConfig(DefaultIdempotencyTTL, DefaultIdempotencyMaxKeys)
}

func NewIdempotencyStoreWithConfig(ttl time.Duration, maxKeys int) *IdempotencyStore {
	if ttl <= 0 {
		ttl = DefaultIdempotencyTTL
	}

	if maxKeys <= 0 {
		maxKeys = DefaultIdempotencyMaxKeys
	}

	return &IdempotencyStore{
		entries: make(map[string]idempotencyEntry),
		ttl:     ttl,
		maxKeys: maxKeys,
		nowFn:   time.Now,
	}
}

// SetTimeSource replaces the internal clock source under lock. Intended for tests.
func (s *IdempotencyStore) SetTimeSource(fn func() time.Time) {
	if fn == nil {
		return
	}
	s.mu.Lock()
	s.nowFn = fn
	s.mu.Unlock()
}

func (s *IdempotencyStore) Store(key, requestHash string, response StoredResponse) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cleanupLocked()

	if entry, found := s.entries[key]; found {
		if entry.requestHash != requestHash {
			return ErrIdempotencyConflict
		}
		return nil
	}

	if len(s.entries) >= s.maxKeys {
		return ErrIdempotencyStoreFull
	}

	s.entries[key] = idempotencyEntry{
		requestHash: requestHash,
		response:    cloneStoredResponse(response),
		expiresAt:   s.nowFn().Add(s.ttl),
	}

	return nil
}

func (s *IdempotencyStore) Get(key string) (string, StoredResponse, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, found := s.entries[key]
	if !found {
		return "", StoredResponse{}, false
	}

	if s.nowFn().After(entry.expiresAt) {
		delete(s.entries, key)
		return "", StoredResponse{}, false
	}

	return entry.requestHash, cloneStoredResponse(entry.response), true
}

func (s *IdempotencyStore) Cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cleanupLocked()
}

func (s *IdempotencyStore) StartCleanup(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = s.ttl
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Cleanup()
		}
	}
}

func (s *IdempotencyStore) cleanupLocked() {
	now := s.nowFn()
	for key, entry := range s.entries {
		if now.After(entry.expiresAt) {
			delete(s.entries, key)
		}
	}
}

func cloneStoredResponse(response StoredResponse) StoredResponse {
	cloned := StoredResponse{
		StatusCode: response.StatusCode,
		Headers:    cloneHeaders(response.Headers),
		Body:       append([]byte(nil), response.Body...),
	}

	return cloned
}

func cloneHeaders(headers http.Header) http.Header {
	if headers == nil {
		return http.Header{}
	}

	cloned := make(http.Header, len(headers))
	for key, values := range headers {
		cloned[key] = append([]string(nil), values...)
	}

	return cloned
}
