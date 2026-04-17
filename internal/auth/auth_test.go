package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func TestValidateBearerToken(t *testing.T) {
	tests := []struct {
		name          string
		providedToken string
		expectedToken string
		wantErr       error
	}{
		{
			name:          "valid token",
			providedToken: "valid-token-32-chars-long-abcdefg",
			expectedToken: "valid-token-32-chars-long-abcdefg",
			wantErr:       nil,
		},
		{
			name:          "empty token",
			providedToken: "",
			expectedToken: "expected-token-32-chars-long-abcdefg",
			wantErr:       ErrMissingAuthHeader,
		},
		{
			name:          "invalid token",
			providedToken: "wrong-token",
			expectedToken: "expected-token-32-chars-long-abcdefg",
			wantErr:       ErrInvalidToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBearerToken(tt.providedToken, tt.expectedToken)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Errorf("ValidateBearerToken() error = %v, wantErr %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("ValidateBearerToken() unexpected error = %v", err)
			}
		})
	}
}

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		wantToken  string
		wantErr    error
	}{
		{
			name:       "valid bearer token",
			authHeader: "Bearer my-token-32-chars-long-abcdefg",
			wantToken:  "my-token-32-chars-long-abcdefg",
			wantErr:    nil,
		},
		{
			name:       "empty header",
			authHeader: "",
			wantToken:  "",
			wantErr:    ErrMissingAuthHeader,
		},
		{
			name:       "malformed - missing Bearer prefix",
			authHeader: "my-token",
			wantToken:  "",
			wantErr:    ErrMalformedAuthHeader,
		},
		{
			name:       "malformed - wrong prefix",
			authHeader: "Basic my-token",
			wantToken:  "",
			wantErr:    ErrMalformedAuthHeader,
		},
		{
			name:       "empty token",
			authHeader: "Bearer ",
			wantToken:  "",
			wantErr:    ErrInvalidToken,
		},
		{
			name:       "extra spaces",
			authHeader: "Bearer  my-token",
			wantToken:  "my-token",
			wantErr:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := ExtractBearerToken(tt.authHeader)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Errorf("ExtractBearerToken() error = %v, wantErr %v", err, tt.wantErr)
				}
				if token != tt.wantToken {
					t.Errorf("ExtractBearerToken() token = %v, want %v", token, tt.wantToken)
				}
			} else {
				if err != nil {
					t.Errorf("ExtractBearerToken() unexpected error = %v", err)
				}
				if token != tt.wantToken {
					t.Errorf("ExtractBearerToken() token = %v, want %v", token, tt.wantToken)
				}
			}
		})
	}
}

func TestBuildCanonicalString(t *testing.T) {
	tests := []struct {
		name      string
		nonce     string
		timestamp string
		method    string
		path      string
		bodyHash  string
		want      string
	}{
		{
			name:      "full request",
			nonce:     "uuid-v4-nonce-1234567890",
			timestamp: "1735689600",
			method:    "POST",
			path:      "/api/v1/users",
			bodyHash:  "a1b2c3d4e5f6",
			want:      "uuid-v4-nonce-1234567890\n1735689600\nPOST\n/api/v1/users\na1b2c3d4e5f6",
		},
		{
			name:      "empty body hash",
			nonce:     "uuid-v4-nonce-1234567890",
			timestamp: "1735689600",
			method:    "GET",
			path:      "/api/v1/users",
			bodyHash:  "",
			want:      "uuid-v4-nonce-1234567890\n1735689600\nGET\n/api/v1/users\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildCanonicalString(tt.nonce, tt.timestamp, tt.method, tt.path, tt.bodyHash)
			if got != tt.want {
				t.Errorf("BuildCanonicalString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestComputeBodyHash(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		want string
	}{
		{
			name: "non-empty body",
			body: []byte(`{"user": "test"}`),
			want: "9a5f6c7c9d4e2a1b3c5d7e9f8a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b",
		},
		{
			name: "empty body",
			body: []byte{},
			want: "",
		},
		{
			name: "nil body",
			body: nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeBodyHash(tt.body)
			if tt.want == "" {
				if got != tt.want {
					t.Errorf("ComputeBodyHash() = %v, want %v", got, tt.want)
				}
			} else {
				expectedHash := sha256.New()
				_, _ = expectedHash.Write(tt.body)
				expected := hex.EncodeToString(expectedHash.Sum(nil))

				if got != expected {
					t.Errorf("ComputeBodyHash() = %v, want %v", got, expected)
				}
			}
		})
	}
}

func TestVerifySignature(t *testing.T) {
	secret := "test-secret-32-chars-long-abcdef"

	tests := []struct {
		name            string
		canonicalString string
		signature       string
		wantErr         error
	}{
		{
			name:            "valid signature",
			canonicalString: "nonce-123\n1735689600\nPOST\n/api/users\na1b2c3",
			signature:       computeTestSignature("nonce-123\n1735689600\nPOST\n/api/users\na1b2c3", secret),
			wantErr:         nil,
		},
		{
			name:            "invalid signature",
			canonicalString: "nonce-123\n1735689600\nPOST\n/api/users\na1b2c3",
			signature:       "invalid-signature-hex",
			wantErr:         ErrInvalidSignature,
		},
		{
			name:            "empty canonical string",
			canonicalString: "",
			signature:       computeTestSignature("nonce-123\n1735689600\nPOST\n/api/users\na1b2c3", secret),
			wantErr:         ErrInvalidSignature,
		},
		{
			name:            "empty signature",
			canonicalString: "nonce-123\n1735689600\nPOST\n/api/users\na1b2c3",
			signature:       "",
			wantErr:         ErrInvalidSignature,
		},
		{
			name:            "empty secret",
			canonicalString: "nonce-123\n1735689600\nPOST\n/api/users\na1b2c3",
			signature:       computeTestSignature("nonce-123\n1735689600\nPOST\n/api/users\na1b2c3", secret),
			wantErr:         ErrInvalidSignature,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testSecret := secret
			if tt.name == "empty secret" {
				testSecret = ""
			}

			err := VerifySignature(tt.canonicalString, tt.signature, testSecret)

			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Errorf("VerifySignature() error = %v, wantErr %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("VerifySignature() unexpected error = %v", err)
			}
		})
	}
}

func TestSignRequest(t *testing.T) {
	secret := "test-secret-32-chars-long-abcdef"
	canonicalString := "nonce-123\n1735689600\nPOST\n/api/users\na1b2c3"

	got := SignRequest(canonicalString, secret)
	expected := computeTestSignature(canonicalString, secret)

	if got != expected {
		t.Errorf("SignRequest() = %v, want %v", got, expected)
	}
}

func TestCheckTimestampSkew(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		timestamp time.Time
		wantErr   error
	}{
		{
			name:      "current time",
			timestamp: now,
			wantErr:   nil,
		},
		{
			name:      "within skew - 100s ago",
			timestamp: now.Add(-100 * time.Second),
			wantErr:   nil,
		},
		{
			name:      "within skew - 100s future",
			timestamp: now.Add(100 * time.Second),
			wantErr:   nil,
		},
		{
			name:      "exceeds skew - 400s ago",
			timestamp: now.Add(-400 * time.Second),
			wantErr:   ErrTimestampSkew,
		},
		{
			name:      "exceeds skew - 400s future",
			timestamp: now.Add(400 * time.Second),
			wantErr:   ErrTimestampSkew,
		},
		{
			name:      "exactly at skew limit",
			timestamp: now.Add(-299 * time.Second),
			wantErr:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckTimestampSkew(tt.timestamp)

			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Errorf("CheckTimestampSkew() error = %v, wantErr %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("CheckTimestampSkew() unexpected error = %v", err)
			}
		})
	}
}

func TestNonceCache(t *testing.T) {
	cache := NewNonceCache()

	t.Run("CheckAndStoreNonce - new nonce", func(t *testing.T) {
		nonce := "test-nonce-1"
		err := cache.CheckAndStoreNonce(nonce)
		if err != nil {
			t.Errorf("CheckAndStoreNonce() unexpected error = %v", err)
		}

		if cache.Size() != 1 {
			t.Errorf("Cache size = %d, want 1", cache.Size())
		}
	})

	t.Run("CheckAndStoreNonce - replay detection", func(t *testing.T) {
		nonce := "test-nonce-replay"
		_ = cache.CheckAndStoreNonce(nonce)

		err := cache.CheckAndStoreNonce(nonce)

		if err != ErrReplayDetected {
			t.Errorf("CheckAndStoreNonce() error = %v, wantErr %v", err, ErrReplayDetected)
		}
	})

	t.Run("CheckAndStoreNonce - multiple nonces", func(t *testing.T) {
		cache.Clear()

		for i := 0; i < 100; i++ {
			nonce := strings.Join([]string{"nonce", string(rune(i))}, "-")
			err := cache.CheckAndStoreNonce(nonce)
			if err != nil {
				t.Errorf("CheckAndStoreNonce() unexpected error = %v", err)
			}
		}

		if cache.Size() != 100 {
			t.Errorf("Cache size = %d, want 100", cache.Size())
		}
	})

	t.Run("Size", func(t *testing.T) {
		cache.Clear()

		if cache.Size() != 0 {
			t.Errorf("Size() = %d, want 0", cache.Size())
		}

		_ = cache.CheckAndStoreNonce("test-nonce")
		if cache.Size() != 1 {
			t.Errorf("Size() = %d, want 1", cache.Size())
		}
	})

	t.Run("Clear", func(t *testing.T) {
		_ = cache.CheckAndStoreNonce("test-nonce-1")
		_ = cache.CheckAndStoreNonce("test-nonce-2")

		cache.Clear()

		if cache.Size() != 0 {
			t.Errorf("After Clear(), Size() = %d, want 0", cache.Size())
		}
	})

	t.Run("HasNonce", func(t *testing.T) {
		cache.Clear()
		nonce := "test-nonce-has"

		if cache.HasNonce(nonce) {
			t.Error("HasNonce() = true, want false before storage")
		}

		_ = cache.CheckAndStoreNonce(nonce)

		if !cache.HasNonce(nonce) {
			t.Error("HasNonce() = false, want true after storage")
		}
	})
}

func TestNonceCacheLRU(t *testing.T) {
	cache := NewNonceCache()

	t.Run("LRU eviction at capacity", func(t *testing.T) {
		cache.Clear()

		for i := 0; i < MaxNonceCacheSize; i++ {
			nonce := strings.Join([]string{"nonce", string(rune(i))}, "-")
			err := cache.CheckAndStoreNonce(nonce)
			if err != nil {
				t.Fatalf("Failed to store nonce: %v", err)
			}
		}

		initialSize := cache.Size()
		if initialSize != MaxNonceCacheSize {
			t.Fatalf("Cache size = %d, want %d", initialSize, MaxNonceCacheSize)
		}

		err := cache.CheckAndStoreNonce("new-nonce")
		if err != nil {
			t.Errorf("CheckAndStoreNonce() error = %v", err)
		}

		if cache.Size() != MaxNonceCacheSize {
			t.Errorf("After eviction, cache size = %d, want %d", cache.Size(), MaxNonceCacheSize)
		}
	})
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name      string
		timestamp int64
		want      time.Time
	}{
		{
			name:      "valid timestamp",
			timestamp: 1735689600,
			want:      time.Unix(1735689600, 0),
		},
		{
			name:      "zero timestamp",
			timestamp: 0,
			want:      time.Unix(0, 0),
		},
		{
			name:      "negative timestamp",
			timestamp: -100,
			want:      time.Unix(-100, 0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseTimestamp(tt.timestamp)

			if !got.Equal(tt.want) {
				t.Errorf("ParseTimestamp() = %v, want %v", got, tt.want)
			}
		})
	}
}

func computeTestSignature(canonicalString, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonicalString))
	return hex.EncodeToString(mac.Sum(nil))
}
