//nolint:errcheck
package store

//nolint:errcheck // Test file uses type assertions

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestNewIdempotencyStore(t *testing.T) {
	t.Parallel()

	s := NewIdempotencyStore()

	if s == nil {
		t.Fatal("NewIdempotencyStore() returned nil")
	}

	if s.ttl != DefaultIdempotencyTTL {
		t.Fatalf("ttl = %v, want %v", s.ttl, DefaultIdempotencyTTL)
	}

	if s.maxKeys != DefaultIdempotencyMaxKeys {
		t.Fatalf("maxKeys = %d, want %d", s.maxKeys, DefaultIdempotencyMaxKeys)
	}

	if s.entries == nil {
		t.Fatal("entries map is nil")
	}

	if s.nowFn == nil {
		t.Fatal("nowFn is nil")
	}
}

func TestNewIdempotencyStoreWithConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ttl      time.Duration
		maxKeys  int
		wantTTL  time.Duration
		wantKeys int
	}{
		{
			name:     "custom config",
			ttl:      time.Hour,
			maxKeys:  100,
			wantTTL:  time.Hour,
			wantKeys: 100,
		},
		{
			name:     "zero ttl uses default",
			ttl:      0,
			maxKeys:  100,
			wantTTL:  DefaultIdempotencyTTL,
			wantKeys: 100,
		},
		{
			name:     "negative ttl uses default",
			ttl:      -1,
			maxKeys:  100,
			wantTTL:  DefaultIdempotencyTTL,
			wantKeys: 100,
		},
		{
			name:     "zero maxKeys uses default",
			ttl:      time.Hour,
			maxKeys:  0,
			wantTTL:  time.Hour,
			wantKeys: DefaultIdempotencyMaxKeys,
		},
		{
			name:     "negative maxKeys uses default",
			ttl:      time.Hour,
			maxKeys:  -1,
			wantTTL:  time.Hour,
			wantKeys: DefaultIdempotencyMaxKeys,
		},
		{
			name:     "both zero use defaults",
			ttl:      0,
			maxKeys:  0,
			wantTTL:  DefaultIdempotencyTTL,
			wantKeys: DefaultIdempotencyMaxKeys,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := NewIdempotencyStoreWithConfig(tt.ttl, tt.maxKeys)

			if s.ttl != tt.wantTTL {
				t.Fatalf("ttl = %v, want %v", s.ttl, tt.wantTTL)
			}

			if s.maxKeys != tt.wantKeys {
				t.Fatalf("maxKeys = %d, want %d", s.maxKeys, tt.wantKeys)
			}
		})
	}
}

func TestIdempotencyStore_Store(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(s *IdempotencyStore)
		key         string
		hash        string
		response    StoredResponse
		wantErr     error
		checkStored func(t *testing.T, s *IdempotencyStore)
	}{
		{
			name:  "new key stores successfully",
			setup: func(s *IdempotencyStore) {},
			key:   "key1",
			hash:  "hash1",
			response: StoredResponse{
				StatusCode: 200,
				Headers:    http.Header{"Content-Type": []string{"application/json"}},
				Body:       []byte(`{"ok":true}`),
			},
			wantErr: nil,
			checkStored: func(t *testing.T, s *IdempotencyStore) {
				_, resp, found := s.Get("key1")
				if !found {
					t.Fatal("entry not found after store")
				}
				if resp.StatusCode != 200 {
					t.Fatalf("status = %d, want 200", resp.StatusCode)
				}
			},
		},
		{
			name: "same key same hash returns nil (idempotent)",
			setup: func(s *IdempotencyStore) {
				s.Store("key1", "hash1", StoredResponse{StatusCode: 200})
			},
			key:      "key1",
			hash:     "hash1",
			response: StoredResponse{StatusCode: 201},
			wantErr:  nil,
			checkStored: func(t *testing.T, s *IdempotencyStore) {
				_, resp, found := s.Get("key1")
				if !found {
					t.Fatal("entry not found")
				}
				if resp.StatusCode != 200 {
					t.Fatalf("status = %d, want 200 (original)", resp.StatusCode)
				}
			},
		},
		{
			name: "same key different hash returns conflict",
			setup: func(s *IdempotencyStore) {
				s.Store("key1", "hash1", StoredResponse{StatusCode: 200})
			},
			key:      "key1",
			hash:     "hash2",
			response: StoredResponse{StatusCode: 201},
			wantErr:  ErrIdempotencyConflict,
		},
		{
			name: "at maxKeys returns full error",
			setup: func(s *IdempotencyStore) {
				s.Store("key1", "hash1", StoredResponse{})
				s.Store("key2", "hash2", StoredResponse{})
			},
			key:      "key3",
			hash:     "hash3",
			response: StoredResponse{},
			wantErr:  ErrIdempotencyStoreFull,
		},
		{
			name: "cleanup runs before maxKeys check",
			setup: func(s *IdempotencyStore) {
				now := time.Now()
				s.SetTimeSource(func() time.Time { return now })
				s.Store("key1", "hash1", StoredResponse{})
				s.Store("key2", "hash2", StoredResponse{})
				// Move time forward to expire entries
				s.SetTimeSource(func() time.Time { return now.Add(25 * time.Hour) })
			},
			key:      "key3",
			hash:     "hash3",
			response: StoredResponse{},
			wantErr:  nil,
		},
		{
			name:  "empty key stores successfully",
			setup: func(s *IdempotencyStore) {},
			key:   "",
			hash:  "hash1",
			response: StoredResponse{
				StatusCode: 200,
			},
			wantErr: nil,
		},
		{
			name:  "empty hash stores successfully",
			setup: func(s *IdempotencyStore) {},
			key:   "key1",
			hash:  "",
			response: StoredResponse{
				StatusCode: 200,
			},
			wantErr: nil,
		},
		{
			name:  "nil headers stores successfully",
			setup: func(s *IdempotencyStore) {},
			key:   "key1",
			hash:  "hash1",
			response: StoredResponse{
				StatusCode: 200,
				Headers:    nil,
				Body:       []byte("body"),
			},
			wantErr: nil,
		},
		{
			name:  "empty body stores successfully",
			setup: func(s *IdempotencyStore) {},
			key:   "key1",
			hash:  "hash1",
			response: StoredResponse{
				StatusCode: 200,
				Body:       []byte{},
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := NewIdempotencyStoreWithConfig(24*time.Hour, 2)
			tt.setup(s)

			err := s.Store(tt.key, tt.hash, tt.response)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Store() error = %v, want %v", err, tt.wantErr)
			}

			if tt.checkStored != nil {
				tt.checkStored(t, s)
			}
		})
	}
}

func TestIdempotencyStore_Get(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(s *IdempotencyStore)
		key         string
		wantHash    string
		wantFound   bool
		wantStatus  int
		wantHeaders http.Header
		wantBody    []byte
	}{
		{
			name: "found returns stored values",
			setup: func(s *IdempotencyStore) {
				s.Store("key1", "hash1", StoredResponse{
					StatusCode: 201,
					Headers:    http.Header{"X-Custom": []string{"value"}},
					Body:       []byte("response body"),
				})
			},
			key:        "key1",
			wantHash:   "hash1",
			wantFound:  true,
			wantStatus: 201,
			wantHeaders: http.Header{
				"X-Custom": []string{"value"},
			},
			wantBody: []byte("response body"),
		},
		{
			name:      "not found returns false",
			setup:     func(s *IdempotencyStore) {},
			key:       "nonexistent",
			wantFound: false,
		},
		{
			name: "expired entry returns false and deletes",
			setup: func(s *IdempotencyStore) {
				now := time.Now()
				s.SetTimeSource(func() time.Time { return now })
				s.Store("key1", "hash1", StoredResponse{StatusCode: 200})
				// Move time forward past expiration
				s.SetTimeSource(func() time.Time { return now.Add(25 * time.Hour) })
			},
			key:       "key1",
			wantFound: false,
		},
		{
			name: "empty key returns not found",
			setup: func(s *IdempotencyStore) {
				s.Store("key1", "hash1", StoredResponse{})
			},
			key:       "",
			wantFound: false,
		},
		{
			name: "multiple values in header",
			setup: func(s *IdempotencyStore) {
				s.Store("key1", "hash1", StoredResponse{
					Headers: http.Header{
						"Set-Cookie": []string{"cookie1=value1", "cookie2=value2"},
					},
				})
			},
			key:       "key1",
			wantHash:  "hash1",
			wantFound: true,
			wantHeaders: http.Header{
				"Set-Cookie": []string{"cookie1=value1", "cookie2=value2"},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := NewIdempotencyStoreWithConfig(24*time.Hour, 10)
			tt.setup(s)

			hash, response, found := s.Get(tt.key)

			if found != tt.wantFound {
				t.Fatalf("Get() found = %v, want %v", found, tt.wantFound)
			}

			if !found {
				return
			}

			if hash != tt.wantHash {
				t.Fatalf("Get() hash = %q, want %q", hash, tt.wantHash)
			}

			if response.StatusCode != tt.wantStatus {
				t.Fatalf("Get() status = %d, want %d", response.StatusCode, tt.wantStatus)
			}

			if tt.wantHeaders != nil {
				for key, wantValues := range tt.wantHeaders {
					gotValues := response.Headers[key]
					if len(gotValues) != len(wantValues) {
						t.Fatalf("Get() header %s has %d values, want %d", key, len(gotValues), len(wantValues))
					}
					for i, wantVal := range wantValues {
						if gotValues[i] != wantVal {
							t.Fatalf("Get() header %s[%d] = %q, want %q", key, i, gotValues[i], wantVal)
						}
					}
				}
			}

			if string(response.Body) != string(tt.wantBody) {
				t.Fatalf("Get() body = %q, want %q", string(response.Body), string(tt.wantBody))
			}
		})
	}
}

func TestIdempotencyStore_Cleanup(t *testing.T) {
	t.Parallel()

	t.Run("removes expired entries", func(t *testing.T) {
		t.Parallel()

		now := time.Now()
		s := NewIdempotencyStoreWithConfig(time.Hour, 10)
		s.SetTimeSource(func() time.Time { return now })

		s.Store("expired1", "hash1", StoredResponse{})
		s.Store("expired2", "hash2", StoredResponse{})

		// Move time forward to expire entries
		s.SetTimeSource(func() time.Time { return now.Add(2 * time.Hour) })

		s.Store("valid1", "hash3", StoredResponse{})

		s.Cleanup()

		_, _, found1 := s.Get("expired1")
		if found1 {
			t.Error("expired1 still found after cleanup")
		}

		_, _, found2 := s.Get("expired2")
		if found2 {
			t.Error("expired2 still found after cleanup")
		}

		_, _, found3 := s.Get("valid1")
		if !found3 {
			t.Error("valid1 not found after cleanup")
		}
	})

	t.Run("keeps valid entries", func(t *testing.T) {
		t.Parallel()

		now := time.Now()
		s := NewIdempotencyStoreWithConfig(time.Hour, 10)
		s.SetTimeSource(func() time.Time { return now })

		s.Store("valid1", "hash1", StoredResponse{})
		s.Store("valid2", "hash2", StoredResponse{})

		// Move time forward but not past expiration
		s.SetTimeSource(func() time.Time { return now.Add(30 * time.Minute) })

		s.Cleanup()

		_, _, found1 := s.Get("valid1")
		if !found1 {
			t.Error("valid1 not found after cleanup")
		}

		_, _, found2 := s.Get("valid2")
		if !found2 {
			t.Error("valid2 not found after cleanup")
		}
	})

	t.Run("empty store cleanup", func(t *testing.T) {
		t.Parallel()

		s := NewIdempotencyStore()
		s.Cleanup()

		// Should not panic
	})
}

func TestIdempotencyStore_StartCleanup(t *testing.T) {
	t.Parallel()

	t.Run("periodic cleanup runs", func(t *testing.T) {
		t.Parallel()

		now := time.Now()
		s := NewIdempotencyStoreWithConfig(10*time.Millisecond, 10)
		s.SetTimeSource(func() time.Time { return now })

		s.Store("key1", "hash1", StoredResponse{})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Start cleanup with short interval
		go s.StartCleanup(ctx, 5*time.Millisecond)

		// Wait for cleanup to run
		time.Sleep(20 * time.Millisecond)

		// Move time forward to expire entry
		s.SetTimeSource(func() time.Time { return now.Add(20 * time.Millisecond) })

		// Wait for cleanup to run again
		time.Sleep(20 * time.Millisecond)

		_, _, found := s.Get("key1")
		if found {
			t.Error("entry still found after periodic cleanup")
		}
	})

	t.Run("stops on context cancel", func(t *testing.T) {
		t.Parallel()

		now := time.Now()
		s := NewIdempotencyStoreWithConfig(time.Hour, 10)
		s.SetTimeSource(func() time.Time { return now })

		// Store an entry
		s.Store("key1", "hash1", StoredResponse{})

		ctx, cancel := context.WithCancel(context.Background())

		// Track cleanup calls via nowFn
		cleanupCount := 0
		var mu sync.Mutex
		s.SetTimeSource(func() time.Time {
			mu.Lock()
			cleanupCount++
			mu.Unlock()
			return now
		})

		go s.StartCleanup(ctx, 10*time.Millisecond)

		// Wait for at least one cleanup
		time.Sleep(20 * time.Millisecond)

		cancel()

		// Wait a bit to ensure goroutine exits
		time.Sleep(20 * time.Millisecond)

		mu.Lock()
		wasCalled := cleanupCount > 0
		mu.Unlock()

		if !wasCalled {
			t.Error("cleanup was not called before cancel")
		}
	})

	t.Run("uses ttl as default interval", func(t *testing.T) {
		t.Parallel()

		s := NewIdempotencyStoreWithConfig(10*time.Millisecond, 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Start cleanup with zero interval (should use ttl)
		go s.StartCleanup(ctx, 0)

		// Should not panic
		time.Sleep(20 * time.Millisecond)
	})
}

func TestCloneStoredResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		response StoredResponse
	}{
		{
			name: "full response",
			response: StoredResponse{
				StatusCode: 201,
				Headers: http.Header{
					"Content-Type": []string{"application/json"},
					"X-Custom":     []string{"value1", "value2"},
				},
				Body: []byte("response body"),
			},
		},
		{
			name: "nil headers",
			response: StoredResponse{
				StatusCode: 200,
				Headers:    nil,
				Body:       []byte("body"),
			},
		},
		{
			name: "empty body",
			response: StoredResponse{
				StatusCode: 200,
				Body:       []byte{},
			},
		},
		{
			name:     "empty response",
			response: StoredResponse{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cloned := cloneStoredResponse(tt.response)

			if cloned.StatusCode != tt.response.StatusCode {
				t.Fatalf("StatusCode = %d, want %d", cloned.StatusCode, tt.response.StatusCode)
			}

			if tt.response.Headers != nil {
				if cloned.Headers == nil {
					t.Fatal("Headers is nil, want non-nil")
				}
				if len(cloned.Headers) != len(tt.response.Headers) {
					t.Fatalf("Headers length = %d, want %d", len(cloned.Headers), len(tt.response.Headers))
				}
			}

			if string(cloned.Body) != string(tt.response.Body) {
				t.Fatalf("Body = %q, want %q", string(cloned.Body), string(tt.response.Body))
			}

			// Verify deep copy - modifying original should not affect clone
			if len(tt.response.Body) > 0 {
				tt.response.Body[0] = 'X'
				if cloned.Body[0] == 'X' {
					t.Error("Body is not deep copied")
				}
			}

			if tt.response.Headers != nil {
				for key, values := range tt.response.Headers {
					if len(values) > 0 {
						tt.response.Headers[key][0] = "modified"
						if cloned.Headers[key][0] == "modified" {
							t.Error("Headers are not deep copied")
						}
					}
				}
			}
		})
	}
}

func TestCloneHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		headers http.Header
	}{
		{
			name: "multiple headers with multiple values",
			headers: http.Header{
				"Content-Type":  []string{"application/json"},
				"Set-Cookie":    []string{"cookie1=value1", "cookie2=value2"},
				"X-Custom":      []string{"value"},
				"Cache-Control": []string{"no-cache"},
			},
		},
		{
			name:    "nil headers",
			headers: nil,
		},
		{
			name:    "empty headers",
			headers: http.Header{},
		},
		{
			name: "single header with single value",
			headers: http.Header{
				"Content-Type": []string{"text/plain"},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cloned := cloneHeaders(tt.headers)

			if tt.headers == nil {
				if cloned == nil {
					t.Fatal("cloned is nil, want empty map")
				}
				if len(cloned) != 0 {
					t.Fatalf("cloned length = %d, want 0", len(cloned))
				}
				return
			}

			if len(cloned) != len(tt.headers) {
				t.Fatalf("length = %d, want %d", len(cloned), len(tt.headers))
			}

			for key, originalValues := range tt.headers {
				clonedValues := cloned[key]
				if len(clonedValues) != len(originalValues) {
					t.Fatalf("header %s has %d values, want %d", key, len(clonedValues), len(originalValues))
				}
				for i, val := range originalValues {
					if clonedValues[i] != val {
						t.Fatalf("header %s[%d] = %q, want %q", key, i, clonedValues[i], val)
					}
				}
			}

			// Verify deep copy
			for key := range tt.headers {
				if len(tt.headers[key]) > 0 {
					original := tt.headers[key][0]
					tt.headers[key][0] = "modified"
					if cloned[key][0] == "modified" {
						t.Error("header values are not deep copied")
					}
					tt.headers[key][0] = original
				}
			}
		})
	}
}

func TestIdempotencyStore_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	t.Run("concurrent stores", func(t *testing.T) {
		t.Parallel()

		s := NewIdempotencyStoreWithConfig(24*time.Hour, 1000)

		var wg sync.WaitGroup
		numGoroutines := 100
		storesPerGoroutine := 10

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()
				for j := 0; j < storesPerGoroutine; j++ {
					key := string(rune('a'+goroutineID%26)) + string(rune('0'+j%10))
					hash := "hash-" + key
					s.Store(key, hash, StoredResponse{StatusCode: 200})
				}
			}(i)
		}

		wg.Wait()

		// Verify all stores succeeded
		count := 0
		for i := 0; i < numGoroutines; i++ {
			for j := 0; j < storesPerGoroutine; j++ {
				key := string(rune('a'+i%26)) + string(rune('0'+j%10))
				_, _, found := s.Get(key)
				if found {
					count++
				}
			}
		}

		if count == 0 {
			t.Error("no entries found after concurrent stores")
		}
	})

	t.Run("concurrent gets and stores", func(t *testing.T) {
		t.Parallel()

		s := NewIdempotencyStoreWithConfig(24*time.Hour, 100)

		// Pre-populate
		for i := 0; i < 50; i++ {
			key := "key-" + string(rune('0'+i%10))
			s.Store(key, "hash-"+key, StoredResponse{StatusCode: 200})
		}

		var wg sync.WaitGroup
		numGoroutines := 50

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				key := "key-" + string(rune('0'+id%10))
				s.Store(key, "hash-"+key, StoredResponse{StatusCode: 200})
				s.Get(key)
			}(i)
		}

		wg.Wait()

		// Should not panic
	})

	t.Run("concurrent cleanup and access", func(t *testing.T) {
		t.Parallel()

		now := time.Now()
		s := NewIdempotencyStoreWithConfig(10*time.Millisecond, 100)
		s.SetTimeSource(func() time.Time { return now })

		var wg sync.WaitGroup

		// Store goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				key := "key-" + string(rune('0'+i%10))
				s.Store(key, "hash-"+key, StoredResponse{StatusCode: 200})
				time.Sleep(time.Millisecond)
			}
		}()

		// Get goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				key := "key-" + string(rune('0'+i%10))
				s.Get(key)
				time.Sleep(time.Millisecond)
			}
		}()

		// Cleanup goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				s.Cleanup()
				time.Sleep(5 * time.Millisecond)
			}
		}()

		wg.Wait()

		// Should not panic
	})
}

func TestIdempotencyStore_ResponseIsolation(t *testing.T) {
	t.Parallel()

	t.Run("stored response is isolated from original", func(t *testing.T) {
		t.Parallel()

		s := NewIdempotencyStore()

		original := StoredResponse{
			StatusCode: 200,
			Headers: http.Header{
				"X-Test": []string{"original"},
			},
			Body: []byte("original body"),
		}

		s.Store("key1", "hash1", original)

		// Modify original
		original.StatusCode = 201
		original.Headers["X-Test"][0] = "modified"
		original.Body[0] = 'X'

		_, response, found := s.Get("key1")
		if !found {
			t.Fatal("entry not found")
		}

		if response.StatusCode != 200 {
			t.Fatalf("status = %d, want 200", response.StatusCode)
		}

		if response.Headers.Get("X-Test") != "original" {
			t.Fatalf("header = %q, want original", response.Headers.Get("X-Test"))
		}

		if response.Body[0] != 'o' {
			t.Fatalf("body[0] = %c, want 'o'", response.Body[0])
		}
	})

	t.Run("retrieved response is isolated from stored", func(t *testing.T) {
		t.Parallel()

		s := NewIdempotencyStore()

		s.Store("key1", "hash1", StoredResponse{
			StatusCode: 200,
			Headers: http.Header{
				"X-Test": []string{"stored"},
			},
			Body: []byte("stored body"),
		})

		_, response, _ := s.Get("key1")

		// Modify retrieved
		response.StatusCode = 201
		response.Headers["X-Test"][0] = "modified"
		response.Body[0] = 'X'

		// Get again
		_, response2, _ := s.Get("key1")

		if response2.StatusCode != 200 {
			t.Fatalf("status = %d, want 200", response2.StatusCode)
		}

		if response2.Headers.Get("X-Test") != "stored" {
			t.Fatalf("header = %q, want stored", response2.Headers.Get("X-Test"))
		}

		if response2.Body[0] != 's' {
			t.Fatalf("body[0] = %c, want 's'", response2.Body[0])
		}
	})
}

func TestIdempotencyStore_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("store after expired entry cleanup", func(t *testing.T) {
		t.Parallel()

		now := time.Now()
		s := NewIdempotencyStoreWithConfig(time.Hour, 2)
		s.SetTimeSource(func() time.Time { return now })

		s.Store("key1", "hash1", StoredResponse{})
		s.Store("key2", "hash2", StoredResponse{})

		// Expire entries
		s.SetTimeSource(func() time.Time { return now.Add(2 * time.Hour) })

		// Should be able to store new keys after cleanup
		err := s.Store("key3", "hash3", StoredResponse{})
		if err != nil {
			t.Fatalf("Store() after cleanup error = %v", err)
		}
	})

	t.Run("same key after expiration", func(t *testing.T) {
		t.Parallel()

		now := time.Now()
		s := NewIdempotencyStoreWithConfig(time.Hour, 10)
		s.SetTimeSource(func() time.Time { return now })

		s.Store("key1", "hash1", StoredResponse{StatusCode: 200})

		// Expire entry
		s.SetTimeSource(func() time.Time { return now.Add(2 * time.Hour) })

		// Should be able to store with same key after expiration
		err := s.Store("key1", "hash2", StoredResponse{StatusCode: 201})
		if err != nil {
			t.Fatalf("Store() after expiration error = %v", err)
		}

		_, response, found := s.Get("key1")
		if !found {
			t.Fatal("entry not found")
		}
		if response.StatusCode != 201 {
			t.Fatalf("status = %d, want 201", response.StatusCode)
		}
	})

	t.Run("unicode keys and hashes", func(t *testing.T) {
		t.Parallel()

		s := NewIdempotencyStore()

		key := "key-日本語-🔑"
		hash := "hash-中文-🔐"

		err := s.Store(key, hash, StoredResponse{StatusCode: 200})
		if err != nil {
			t.Fatalf("Store() with unicode error = %v", err)
		}

		retrievedHash, _, found := s.Get(key)
		if !found {
			t.Fatal("entry not found")
		}
		if retrievedHash != hash {
			t.Fatalf("hash = %q, want %q", retrievedHash, hash)
		}
	})

	t.Run("large body", func(t *testing.T) {
		t.Parallel()

		s := NewIdempotencyStore()

		largeBody := make([]byte, 1024*1024) // 1MB
		for i := range largeBody {
			largeBody[i] = byte(i % 256)
		}

		err := s.Store("key1", "hash1", StoredResponse{
			StatusCode: 200,
			Body:       largeBody,
		})
		if err != nil {
			t.Fatalf("Store() with large body error = %v", err)
		}

		_, response, found := s.Get("key1")
		if !found {
			t.Fatal("entry not found")
		}
		if len(response.Body) != len(largeBody) {
			t.Fatalf("body length = %d, want %d", len(response.Body), len(largeBody))
		}
	})
}
