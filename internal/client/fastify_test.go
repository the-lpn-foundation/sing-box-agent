//nolint:errcheck
package client

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFastifyClient(t *testing.T) {
	tests := []struct {
		name    string
		opts    Options
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid options",
			opts: Options{
				BaseURL: "https://api.example.com",
				Token:   "test-token",
				Secret:  "test-secret",
			},
			wantErr: false,
		},
		{
			name: "missing base URL",
			opts: Options{
				Token:  "test-token",
				Secret: "test-secret",
			},
			wantErr: true,
			errMsg:  "base URL is required",
		},
		{
			name: "empty base URL",
			opts: Options{
				BaseURL: "",
				Token:   "test-token",
				Secret:  "test-secret",
			},
			wantErr: true,
			errMsg:  "base URL is required",
		},
		{
			name: "whitespace only base URL",
			opts: Options{
				BaseURL: "   ",
				Token:   "test-token",
				Secret:  "test-secret",
			},
			wantErr: true,
			errMsg:  "base URL is required",
		},
		{
			name: "missing token",
			opts: Options{
				BaseURL: "https://api.example.com",
				Secret:  "test-secret",
			},
			wantErr: true,
			errMsg:  "token is required",
		},
		{
			name: "empty token",
			opts: Options{
				BaseURL: "https://api.example.com",
				Token:   "",
				Secret:  "test-secret",
			},
			wantErr: true,
			errMsg:  "token is required",
		},
		{
			name: "whitespace only token",
			opts: Options{
				BaseURL: "https://api.example.com",
				Token:   "  ",
				Secret:  "test-secret",
			},
			wantErr: true,
			errMsg:  "token is required",
		},
		{
			name: "missing secret",
			opts: Options{
				BaseURL: "https://api.example.com",
				Token:   "test-token",
			},
			wantErr: true,
			errMsg:  "secret is required",
		},
		{
			name: "empty secret",
			opts: Options{
				BaseURL: "https://api.example.com",
				Token:   "test-token",
				Secret:  "",
			},
			wantErr: true,
			errMsg:  "secret is required",
		},
		{
			name: "whitespace only secret",
			opts: Options{
				BaseURL: "https://api.example.com",
				Token:   "test-token",
				Secret:  "  ",
			},
			wantErr: true,
			errMsg:  "secret is required",
		},
		{
			name: "base URL with trailing slash",
			opts: Options{
				BaseURL: "https://api.example.com/",
				Token:   "test-token",
				Secret:  "test-secret",
			},
			wantErr: false,
		},
		{
			name: "base URL with multiple trailing slashes",
			opts: Options{
				BaseURL: "https://api.example.com///",
				Token:   "test-token",
				Secret:  "test-secret",
			},
			wantErr: false,
		},
		{
			name: "with custom timeout",
			opts: Options{
				BaseURL: "https://api.example.com",
				Token:   "test-token",
				Secret:  "test-secret",
				Timeout: 30 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "with retry config",
			opts: Options{
				BaseURL: "https://api.example.com",
				Token:   "test-token",
				Secret:  "test-secret",
				Retry: RetryConfig{
					MaxRetries: 5,
					BaseDelay:  2 * time.Second,
					MaxDelay:   60 * time.Second,
				},
			},
			wantErr: false,
		},
		{
			name: "with custom HTTP client",
			opts: Options{
				BaseURL:    "https://api.example.com",
				Token:      "test-token",
				Secret:     "test-secret",
				HTTPClient: &http.Client{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewFastifyClient(tt.opts)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewFastifyClient() expected error, got nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("NewFastifyClient() error = %v, want error containing %q", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("NewFastifyClient() unexpected error = %v", err)
				return
			}

			if client == nil {
				t.Errorf("NewFastifyClient() returned nil client")
				return
			}

			// Verify base URL is trimmed
			expectedBaseURL := strings.TrimRight(tt.opts.BaseURL, "/")
			if client.baseURL != expectedBaseURL {
				t.Errorf("Client.baseURL = %q, want %q", client.baseURL, expectedBaseURL)
			}

			// Verify token and secret are set
			if client.token != tt.opts.Token {
				t.Errorf("Client.token = %q, want %q", client.token, tt.opts.Token)
			}
			if client.secret != tt.opts.Secret {
				t.Errorf("Client.secret = %q, want %q", client.secret, tt.opts.Secret)
			}

			// Verify HTTP client is set
			if client.httpClient == nil {
				t.Errorf("Client.httpClient is nil")
			}

			// Verify retry config has defaults
			if client.retry.MaxRetries == 0 {
				t.Errorf("Client.retry.MaxRetries should have default value")
			}
		})
	}
}

func TestBuildHTTPClient(t *testing.T) {
	tests := []struct {
		name    string
		opts    Options
		wantErr bool
		errMsg  string
	}{
		{
			name: "default options",
			opts: Options{
				BaseURL: "https://api.example.com",
				Token:   "test-token",
				Secret:  "test-secret",
			},
			wantErr: false,
		},
		{
			name: "with custom HTTP client",
			opts: Options{
				BaseURL:    "https://api.example.com",
				Token:      "test-token",
				Secret:     "test-secret",
				HTTPClient: &http.Client{Timeout: 5 * time.Minute},
			},
			wantErr: false,
		},
		{
			name: "with custom timeout",
			opts: Options{
				BaseURL: "https://api.example.com",
				Token:   "test-token",
				Secret:  "test-secret",
				Timeout: 45 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "with zero timeout (should use default)",
			opts: Options{
				BaseURL: "https://api.example.com",
				Token:   "test-token",
				Secret:  "test-secret",
				Timeout: 0,
			},
			wantErr: false,
		},
		{
			name: "with negative timeout (should use default)",
			opts: Options{
				BaseURL: "https://api.example.com",
				Token:   "test-token",
				Secret:  "test-secret",
				Timeout: -10 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "with server name",
			opts: Options{
				BaseURL:    "https://api.example.com",
				Token:      "test-token",
				Secret:     "test-secret",
				ServerName: "custom.server.com",
			},
			wantErr: false,
		},
		{
			name: "with whitespace server name",
			opts: Options{
				BaseURL:    "https://api.example.com",
				Token:      "test-token",
				Secret:     "test-secret",
				ServerName: "  custom.server.com  ",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := buildHTTPClient(tt.opts)
			if tt.wantErr {
				if err == nil {
					t.Errorf("buildHTTPClient() expected error, got nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("buildHTTPClient() error = %v, want error containing %q", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("buildHTTPClient() unexpected error = %v", err)
				return
			}

			if client == nil {
				t.Errorf("buildHTTPClient() returned nil client")
				return
			}

			// Skip all checks for custom HTTPClient
			if tt.opts.HTTPClient != nil {
				// Custom client returned as-is, nothing more to verify
				return
			}

			// Verify timeout
			expectedTimeout := tt.opts.Timeout
			if expectedTimeout <= 0 {
				expectedTimeout = defaultTimeout
			}
			if client.Timeout != expectedTimeout {
				t.Errorf("HTTPClient.Timeout = %v, want %v", client.Timeout, expectedTimeout)
			}

			// Verify transport is set

			// Verify transport is set
			if client.Transport == nil {
				t.Errorf("Transport is nil")
				return
			}

			transport, ok := client.Transport.(*http.Transport)
			if !ok {
				t.Errorf("Transport is not *http.Transport")
				return
			}

			// Verify TLS config
			if transport.TLSClientConfig == nil {
				t.Errorf("Transport.TLSClientConfig is nil")
				return
			}

			if transport.TLSClientConfig.MinVersion != tls.VersionTLS12 {
				t.Errorf("TLSClientConfig.MinVersion = %v, want %v", transport.TLSClientConfig.MinVersion, tls.VersionTLS12)
			}

			// Verify server name if provided
			if tt.opts.ServerName != "" {
				expectedServerName := strings.TrimSpace(tt.opts.ServerName)
				if transport.TLSClientConfig.ServerName != expectedServerName {
					t.Errorf("TLSClientConfig.ServerName = %q, want %q", transport.TLSClientConfig.ServerName, expectedServerName)
				}
			}
		})
	}
}

// Note: TestBuildHTTPClient tests all cases. When HTTPClient is provided,
// the function returns it directly without modification.
// The transport/TLS checks only apply when building a new client.

func TestBuildHTTPClientWithCACert(t *testing.T) {
	// Create a temporary CA cert file
	tmpDir := t.TempDir()
	caCertPath := tmpDir + "/ca.crt"

	// Write a valid PEM certificate
	validPEM := `-----BEGIN CERTIFICATE-----
MIIBkTCB+wIJAKHHCgVZU1BAMA0GCSqGSIb3DQEBCwUAMBExDzANBgNVBAMMBnRl
c3RjYTAeFw0yNDAxMDEwMDAwMDBaFw0yNTAxMDEwMDAwMDBaMBExDzANBgNVBAMM
BnRlc3RjYTCBnzANBgkqhkiG9w0BAQEFAAOBjQAwgYkCgYEAwT8kqCEm4Y5lqZ5p
-----END CERTIFICATE-----`

	if err := os.WriteFile(caCertPath, []byte(validPEM), 0o644); err != nil {
		t.Fatalf("Failed to write CA cert: %v", err)
	}

	t.Run("with invalid CA cert path", func(t *testing.T) {
		opts := Options{
			BaseURL:    "https://api.example.com",
			Token:      "test-token",
			Secret:     "test-secret",
			CACertPath: "/nonexistent/path/to/ca.crt",
		}

		_, err := buildHTTPClient(opts)
		if err == nil {
			t.Errorf("buildHTTPClient() expected error for invalid CA cert path, got nil")
			return
		}

		if !strings.Contains(err.Error(), "read custom CA certificate") {
			t.Errorf("buildHTTPClient() error = %v, want error containing 'read custom CA certificate'", err)
		}
	})

	t.Run("with invalid PEM format", func(t *testing.T) {
		invalidPEMPath := tmpDir + "/invalid.crt"
		if err := os.WriteFile(invalidPEMPath, []byte("not a valid PEM"), 0o644); err != nil {
			t.Fatalf("Failed to write invalid PEM: %v", err)
		}

		opts := Options{
			BaseURL:    "https://api.example.com",
			Token:      "test-token",
			Secret:     "test-secret",
			CACertPath: invalidPEMPath,
		}

		_, err := buildHTTPClient(opts)
		if err == nil {
			t.Errorf("buildHTTPClient() expected error for invalid PEM, got nil")
			return
		}

		if !strings.Contains(err.Error(), "parse custom CA certificate") {
			t.Errorf("buildHTTPClient() error = %v, want error containing 'parse custom CA certificate'", err)
		}
	})
}

func TestHeartbeat(t *testing.T) {
	tests := []struct {
		name           string
		responseStatus int
		responseBody   interface{}
		wantErr        bool
		errMsg         string
	}{
		{
			name:           "success",
			responseStatus: http.StatusOK,
			responseBody:   envelope[map[string]time.Time]{Success: true, Data: map[string]time.Time{}},
			wantErr:        false,
		},
		{
			name:           "success with data",
			responseStatus: http.StatusOK,
			responseBody:   envelope[map[string]time.Time]{Success: true, Data: map[string]time.Time{"timestamp": time.Now().UTC()}},
			wantErr:        false,
		},

		{
			name:           "HTTP 400",
			responseStatus: http.StatusBadRequest,
			responseBody:   "bad request",
			wantErr:        true,
			errMsg:         "request failed with status 400",
		},
		{
			name:           "HTTP 401",
			responseStatus: http.StatusUnauthorized,
			responseBody:   "unauthorized",
			wantErr:        true,
			errMsg:         "request failed with status 401",
		},
		{
			name:           "HTTP 404",
			responseStatus: http.StatusNotFound,
			responseBody:   "not found",
			wantErr:        true,
			errMsg:         "request failed with status 404",
		},
		{
			name:           "HTTP 500 (retryable)",
			responseStatus: http.StatusInternalServerError,
			responseBody:   "internal server error",
			wantErr:        true,
			errMsg:         "retryable server status 500",
		},
		{
			name:           "HTTP 502 (retryable)",
			responseStatus: http.StatusBadGateway,
			responseBody:   "bad gateway",
			wantErr:        true,
			errMsg:         "retryable server status 502",
		},
		{
			name:           "HTTP 503 (retryable)",
			responseStatus: http.StatusServiceUnavailable,
			responseBody:   "service unavailable",
			wantErr:        true,
			errMsg:         "retryable server status 503",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify method
				if r.Method != http.MethodPost {
					t.Errorf("Expected POST request, got %s", r.Method)
				}

				// Verify path
				if r.URL.Path != "/api/agents/heartbeat" {
					t.Errorf("Expected path /api/agents/heartbeat, got %s", r.URL.Path)
				}

				// Verify headers
				auth := r.Header.Get("Authorization")
				if !strings.HasPrefix(auth, "Bearer ") {
					t.Errorf("Expected Authorization header to start with 'Bearer ', got %q", auth)
				}

				secret := r.Header.Get("X-Agent-Secret")
				if secret != "test-secret" {
					t.Errorf("Expected X-Agent-Secret header to be 'test-secret', got %q", secret)
				}

				accept := r.Header.Get("Accept")
				if accept != "application/json" {
					t.Errorf("Expected Accept header to be 'application/json', got %q", accept)
				}

				contentType := r.Header.Get("Content-Type")
				if contentType != "application/json" {
					t.Errorf("Expected Content-Type header to be 'application/json', got %q", contentType)
				}

				// Verify request body contains timestamp
				body, _ := io.ReadAll(r.Body)
				var reqBody map[string]time.Time
				if err := json.Unmarshal(body, &reqBody); err != nil {
					t.Errorf("Failed to unmarshal request body: %v", err)
				}
				if _, ok := reqBody["timestamp"]; !ok {
					t.Errorf("Expected request body to contain 'timestamp' field")
				}

				// Send response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.responseStatus)
				json.NewEncoder(w).Encode(tt.responseBody)
			}))
			defer server.Close()

			client, err := NewFastifyClient(Options{
				BaseURL: server.URL,
				Token:   "test-token",
				Secret:  "test-secret",
				Retry: RetryConfig{
					MaxRetries: 0, // Disable retries for faster tests
				},
			})
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			ctx := context.Background()
			err = client.Heartbeat(ctx)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Heartbeat() expected error, got nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Heartbeat() error = %v, want error containing %q", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("Heartbeat() unexpected error = %v", err)
			}
		})
	}
}

func TestHeartbeatContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(envelope[map[string]time.Time]{Success: true})
	}))
	defer server.Close()

	client, err := NewFastifyClient(Options{
		BaseURL: server.URL,
		Token:   "test-token",
		Secret:  "test-secret",
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err = client.Heartbeat(ctx)
	if err == nil {
		t.Errorf("Heartbeat() expected error for cancelled context, got nil")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("Heartbeat() error = %v, want error containing 'context canceled'", err)
	}
}

func TestFetchConfig(t *testing.T) {
	tests := []struct {
		name           string
		responseStatus int
		responseBody   interface{}
		wantErr        bool
		errMsg         string
		validateConfig func(*testing.T, *Config)
	}{
		{
			name:           "success with full config",
			responseStatus: http.StatusOK,
			responseBody: envelope[Config]{
				Success: true,
				Data: Config{
					Version:   1,
					Timestamp: time.Now().UTC(),
				},
			},
			wantErr: false,
			validateConfig: func(t *testing.T, cfg *Config) {
				if cfg.Version != 1 {
					t.Errorf("Config.Version = %d, want 1", cfg.Version)
				}
			},
		},
		{
			name:           "success with empty data",
			responseStatus: http.StatusOK,
			responseBody:   envelope[Config]{Success: true},
			wantErr:        false,
			validateConfig: func(t *testing.T, cfg *Config) {
				if cfg == nil {
					t.Errorf("Config is nil")
				}
			},
		},
		{
			name:           "API error",
			responseStatus: http.StatusOK,
			responseBody: envelope[Config]{
				Success: false,
				Error:   &apiError{Code: "ERR_CONFIG", Message: "config not found"},
			},
			wantErr: true,
			errMsg:  "API error ERR_CONFIG: config not found",
		},
		{
			name:           "HTTP 404",
			responseStatus: http.StatusNotFound,
			responseBody:   "not found",
			wantErr:        true,
			errMsg:         "request failed with status 404",
		},
		{
			name:           "invalid JSON response",
			responseStatus: http.StatusOK,
			responseBody:   "not json",
			wantErr:        true,
			errMsg:         "decode envelope",
		},
		{
			name:           "invalid config data",
			responseStatus: http.StatusOK,
			responseBody:   `{"success": true, "data": "invalid"}`,
			wantErr:        true,
			errMsg:         "decode envelope data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify method
				if r.Method != http.MethodGet {
					t.Errorf("Expected GET request, got %s", r.Method)
				}

				// Verify path
				if r.URL.Path != "/api/agents/config" {
					t.Errorf("Expected path /api/agents/config, got %s", r.URL.Path)
				}

				// Verify headers
				auth := r.Header.Get("Authorization")
				if !strings.HasPrefix(auth, "Bearer ") {
					t.Errorf("Expected Authorization header to start with 'Bearer ', got %q", auth)
				}

				secret := r.Header.Get("X-Agent-Secret")
				if secret != "test-secret" {
					t.Errorf("Expected X-Agent-Secret header to be 'test-secret', got %q", secret)
				}

				// Send response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.responseStatus)

				if str, ok := tt.responseBody.(string); ok {
					w.Write([]byte(str))
				} else {
					json.NewEncoder(w).Encode(tt.responseBody)
				}
			}))
			defer server.Close()

			client, err := NewFastifyClient(Options{
				BaseURL: server.URL,
				Token:   "test-token",
				Secret:  "test-secret",
				Retry: RetryConfig{
					MaxRetries: 0,
				},
			})
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			ctx := context.Background()
			cfg, err := client.FetchConfig(ctx)

			if tt.wantErr {
				if err == nil {
					t.Errorf("FetchConfig() expected error, got nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("FetchConfig() error = %v, want error containing %q", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("FetchConfig() unexpected error = %v", err)
				return
			}

			if cfg == nil {
				t.Errorf("FetchConfig() returned nil config")
				return
			}

			if tt.validateConfig != nil {
				tt.validateConfig(t, cfg)
			}
		})
	}
}

func TestFetchConfigContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(envelope[Config]{Success: true})
	}))
	defer server.Close()

	client, err := NewFastifyClient(Options{
		BaseURL: server.URL,
		Token:   "test-token",
		Secret:  "test-secret",
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = client.FetchConfig(ctx)
	if err == nil {
		t.Errorf("FetchConfig() expected error for cancelled context, got nil")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("FetchConfig() error = %v, want error containing 'context canceled'", err)
	}
}

func TestReportStatus(t *testing.T) {
	tests := []struct {
		name           string
		status         Status
		responseStatus int
		responseBody   interface{}
		wantErr        bool
		errMsg         string
	}{
		{
			name: "success with status",
			status: Status{
				State:     "running",
				Version:   1,
				Message:   "Agent is running",
				UpdatedAt: time.Now().UTC(),
			},
			responseStatus: http.StatusOK,
			responseBody:   envelope[Status]{Success: true},
			wantErr:        false,
		},
		{
			name: "success with auto-timestamp",
			status: Status{
				State:   "running",
				Version: 1,
			},
			responseStatus: http.StatusOK,
			responseBody:   envelope[Status]{Success: true},
			wantErr:        false,
		},
		{
			name: "success with details",
			status: Status{
				State:     "running",
				Version:   1,
				UpdatedAt: time.Now().UTC(),
				Details: map[string]interface{}{
					"connections": 100,
					"uptime":      "1h30m",
				},
			},
			responseStatus: http.StatusOK,
			responseBody:   envelope[Status]{Success: true},
			wantErr:        false,
		},
		{
			name: "HTTP 400",
			status: Status{
				State: "running",
			},
			responseStatus: http.StatusBadRequest,
			responseBody:   "bad request",
			wantErr:        true,
			errMsg:         "request failed with status 400",
		},
		{
			name: "HTTP 500 (retryable)",
			status: Status{
				State: "running",
			},
			responseStatus: http.StatusInternalServerError,
			responseBody:   "internal server error",
			wantErr:        true,
			errMsg:         "retryable server status 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedStatus Status
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify method
				if r.Method != http.MethodPost {
					t.Errorf("Expected POST request, got %s", r.Method)
				}

				// Verify path
				if r.URL.Path != "/api/agents/status" {
					t.Errorf("Expected path /api/agents/status, got %s", r.URL.Path)
				}

				// Verify headers
				auth := r.Header.Get("Authorization")
				if !strings.HasPrefix(auth, "Bearer ") {
					t.Errorf("Expected Authorization header to start with 'Bearer ', got %q", auth)
				}

				secret := r.Header.Get("X-Agent-Secret")
				if secret != "test-secret" {
					t.Errorf("Expected X-Agent-Secret header to be 'test-secret', got %q", secret)
				}

				contentType := r.Header.Get("Content-Type")
				if contentType != "application/json" {
					t.Errorf("Expected Content-Type header to be 'application/json', got %q", contentType)
				}

				// Verify request body
				body, _ := io.ReadAll(r.Body)
				if err := json.Unmarshal(body, &receivedStatus); err != nil {
					t.Errorf("Failed to unmarshal request body: %v", err)
				}

				// Send response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.responseStatus)
				json.NewEncoder(w).Encode(tt.responseBody)
			}))
			defer server.Close()

			client, err := NewFastifyClient(Options{
				BaseURL: server.URL,
				Token:   "test-token",
				Secret:  "test-secret",
				Retry: RetryConfig{
					MaxRetries: 0,
				},
			})
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			ctx := context.Background()
			err = client.ReportStatus(ctx, tt.status)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ReportStatus() expected error, got nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ReportStatus() error = %v, want error containing %q", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("ReportStatus() unexpected error = %v", err)
				return
			}

			// Verify status was sent correctly
			if receivedStatus.State != tt.status.State {
				t.Errorf("Received status.State = %q, want %q", receivedStatus.State, tt.status.State)
			}

			// Verify timestamp was auto-set if needed
			if tt.status.UpdatedAt.IsZero() && receivedStatus.UpdatedAt.IsZero() {
				t.Errorf("Expected UpdatedAt to be auto-set, but it was zero")
			}
		})
	}
}

func TestReportStatusContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(envelope[Status]{Success: true})
	}))
	defer server.Close()

	client, err := NewFastifyClient(Options{
		BaseURL: server.URL,
		Token:   "test-token",
		Secret:  "test-secret",
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = client.ReportStatus(ctx, Status{State: "running"})
	if err == nil {
		t.Errorf("ReportStatus() expected error for cancelled context, got nil")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("ReportStatus() error = %v, want error containing 'context canceled'", err)
	}
}

func TestDoJSONWithNetworkError(t *testing.T) {
	// Use an invalid URL to trigger a network error
	client, err := NewFastifyClient(Options{
		BaseURL: "http://invalid-host-that-does-not-exist-12345.local",
		Token:   "test-token",
		Secret:  "test-secret",
		Retry: RetryConfig{
			MaxRetries: 1,
			BaseDelay:  10 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err = client.Heartbeat(ctx)
	if err == nil {
		t.Errorf("Heartbeat() expected error for network failure, got nil")
	}
}

func TestDoJSONWithRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			// Return 500 for first two attempts
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Success on third attempt
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(envelope[map[string]time.Time]{Success: true})
	}))
	defer server.Close()

	client, err := NewFastifyClient(Options{
		BaseURL: server.URL,
		Token:   "test-token",
		Secret:  "test-secret",
		Retry: RetryConfig{
			MaxRetries: 3,
			BaseDelay:  10 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx := context.Background()
	err = client.Heartbeat(ctx)
	if err != nil {
		t.Errorf("Heartbeat() unexpected error after retries = %v", err)
	}

	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestDoJSONRetryExhausted(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewFastifyClient(Options{
		BaseURL: server.URL,
		Token:   "test-token",
		Secret:  "test-secret",
		Retry: RetryConfig{
			MaxRetries: 2,
			BaseDelay:  10 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx := context.Background()
	err = client.Heartbeat(ctx)

	if err == nil {
		t.Errorf("Heartbeat() expected error after retry exhaustion, got nil")
	}

	if !strings.Contains(err.Error(), "retryable server status 500") {
		t.Errorf("Heartbeat() error = %v, want error containing 'retryable server status 500'", err)
	}

	if attempts != 3 { // initial + 2 retries
		t.Errorf("Expected 3 attempts (initial + 2 retries), got %d", attempts)
	}
}

func TestDoJSONWithNonRetryableError(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	client, err := NewFastifyClient(Options{
		BaseURL: server.URL,
		Token:   "test-token",
		Secret:  "test-secret",
		Retry: RetryConfig{
			MaxRetries: 3,
			BaseDelay:  10 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx := context.Background()
	err = client.Heartbeat(ctx)

	if err == nil {
		t.Errorf("Heartbeat() expected error for non-retryable status, got nil")
	}

	if !strings.Contains(err.Error(), "request failed with status 400") {
		t.Errorf("Heartbeat() error = %v, want error containing 'request failed with status 400'", err)
	}

	if attempts != 1 {
		t.Errorf("Expected 1 attempt for non-retryable error, got %d", attempts)
	}
}

func TestDoJSONWithUnmarshalablePayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(envelope[map[string]time.Time]{Success: true})
	}))
	defer server.Close()

	client, err := NewFastifyClient(Options{
		BaseURL: server.URL,
		Token:   "test-token",
		Secret:  "test-secret",
		Retry: RetryConfig{
			MaxRetries: 0,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create a type that cannot be marshaled to JSON
	type unmarshalableType struct {
		Func func()
	}

	ctx := context.Background()
	err = client.doJSON(ctx, http.MethodPost, "/test", unmarshalableType{Func: func() {}}, nil)

	if err == nil {
		t.Errorf("doJSON() expected error for unmarshalable payload, got nil")
	}

	if !strings.Contains(err.Error(), "marshal request payload") {
		t.Errorf("doJSON() error = %v, want error containing 'marshal request payload'", err)
	}
}

func TestEnvelopeParsing(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantErr  bool
		errMsg   string
		validate func(*testing.T, interface{})
	}{
		{
			name:     "valid envelope with success",
			response: `{"success": true, "data": {"version": 1}}`,
			wantErr:  false,
			validate: func(t *testing.T, out interface{}) {
				cfg, ok := out.(*Config)
				if !ok {
					t.Errorf("Expected *Config, got %T", out)
					return
				}
				if cfg.Version != 1 {
					t.Errorf("Config.Version = %d, want 1", cfg.Version)
				}
			},
		},
		{
			name:     "valid envelope with empty data",
			response: `{"success": true, "data": null}`,
			wantErr:  false,
		},
		{
			name:     "valid envelope with empty array data",
			response: `{"success": true, "data": []}`,
			wantErr:  true, // Cannot unmarshal array into struct
			errMsg:   "cannot unmarshal",
		},
		{
			name:     "envelope with error",
			response: `{"success": false, "error": {"code": "ERR_001", "message": "test error"}}`,
			wantErr:  true,
			errMsg:   "API error ERR_001: test error",
		},
		{
			name:     "envelope with success false but no error",
			response: `{"success": false}`,
			wantErr:  true,
			errMsg:   "API returned unsuccessful response",
		},
		{
			name:     "invalid JSON",
			response: `{invalid json}`,
			wantErr:  true,
			errMsg:   "decode envelope",
		},
		{
			name:     "missing success field",
			response: `{"data": {"version": 1}}`,
			wantErr:  true,
			errMsg:   "API returned unsuccessful response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tt.response))
			}))
			defer server.Close()

			client, err := NewFastifyClient(Options{
				BaseURL: server.URL,
				Token:   "test-token",
				Secret:  "test-secret",
				Retry: RetryConfig{
					MaxRetries: 0,
				},
			})
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			ctx := context.Background()
			var cfg Config
			err = client.doJSON(ctx, http.MethodGet, "/test", nil, &cfg)

			if tt.wantErr {
				if err == nil {
					t.Errorf("doJSON() expected error, got nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("doJSON() error = %v, want error containing %q", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("doJSON() unexpected error = %v", err)
				return
			}

			if tt.validate != nil {
				tt.validate(t, &cfg)
			}
		})
	}
}

func TestRetryConfigWithDefaults(t *testing.T) {
	tests := []struct {
		name string
		cfg  RetryConfig
		want RetryConfig
	}{
		{
			name: "zero values get defaults",
			cfg:  RetryConfig{},
			want: RetryConfig{
				MaxRetries: DefaultMaxRetries,
				BaseDelay:  DefaultBaseDelay,
				MaxDelay:   DefaultMaxDelay,
			},
		},
		{
			name: "negative values get defaults",
			cfg: RetryConfig{
				MaxRetries: -1,
				BaseDelay:  -1 * time.Second,
				MaxDelay:   -1 * time.Second,
			},
			want: RetryConfig{
				MaxRetries: DefaultMaxRetries,
				BaseDelay:  DefaultBaseDelay,
				MaxDelay:   DefaultMaxDelay,
			},
		},
		{
			name: "valid values preserved",
			cfg: RetryConfig{
				MaxRetries: 5,
				BaseDelay:  2 * time.Second,
				MaxDelay:   60 * time.Second,
			},
			want: RetryConfig{
				MaxRetries: 5,
				BaseDelay:  2 * time.Second,
				MaxDelay:   60 * time.Second,
			},
		},
		{
			name: "partial defaults",
			cfg: RetryConfig{
				MaxRetries: 5,
				BaseDelay:  0,
				MaxDelay:   0,
			},
			want: RetryConfig{
				MaxRetries: 5,
				BaseDelay:  DefaultBaseDelay,
				MaxDelay:   DefaultMaxDelay,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.withDefaults()
			if got.MaxRetries != tt.want.MaxRetries {
				t.Errorf("MaxRetries = %d, want %d", got.MaxRetries, tt.want.MaxRetries)
			}
			if got.BaseDelay != tt.want.BaseDelay {
				t.Errorf("BaseDelay = %v, want %v", got.BaseDelay, tt.want.BaseDelay)
			}
			if got.MaxDelay != tt.want.MaxDelay {
				t.Errorf("MaxDelay = %v, want %v", got.MaxDelay, tt.want.MaxDelay)
			}
		})
	}
}

func TestRetryConfigBackoff(t *testing.T) {
	tests := []struct {
		name    string
		cfg     RetryConfig
		attempt int
		wantMin time.Duration
		wantMax time.Duration
	}{
		{
			name:    "attempt 0",
			cfg:     RetryConfig{BaseDelay: time.Second, MaxDelay: 30 * time.Second},
			attempt: 0,
			wantMin: time.Second,
			wantMax: time.Second,
		},
		{
			name:    "attempt 1",
			cfg:     RetryConfig{BaseDelay: time.Second, MaxDelay: 30 * time.Second},
			attempt: 1,
			wantMin: 2 * time.Second,
			wantMax: 2 * time.Second,
		},
		{
			name:    "attempt 2",
			cfg:     RetryConfig{BaseDelay: time.Second, MaxDelay: 30 * time.Second},
			attempt: 2,
			wantMin: 4 * time.Second,
			wantMax: 4 * time.Second,
		},
		{
			name:    "attempt 3",
			cfg:     RetryConfig{BaseDelay: time.Second, MaxDelay: 30 * time.Second},
			attempt: 3,
			wantMin: 8 * time.Second,
			wantMax: 8 * time.Second,
		},
		{
			name:    "attempt 4",
			cfg:     RetryConfig{BaseDelay: time.Second, MaxDelay: 30 * time.Second},
			attempt: 4,
			wantMin: 16 * time.Second,
			wantMax: 16 * time.Second,
		},
		{
			name:    "attempt 5 (capped at max)",
			cfg:     RetryConfig{BaseDelay: time.Second, MaxDelay: 30 * time.Second},
			attempt: 5,
			wantMin: 30 * time.Second,
			wantMax: 30 * time.Second,
		},
		{
			name:    "attempt 10 (capped at max)",
			cfg:     RetryConfig{BaseDelay: time.Second, MaxDelay: 30 * time.Second},
			attempt: 10,
			wantMin: 30 * time.Second,
			wantMax: 30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.backoff(tt.attempt)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("backoff(%d) = %v, want between %v and %v", tt.attempt, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestRetryFunction(t *testing.T) {
	t.Run("success on first attempt", func(t *testing.T) {
		cfg := RetryConfig{MaxRetries: 3, BaseDelay: 10 * time.Millisecond}
		err := Retry(context.Background(), cfg, func(ctx context.Context) (bool, error) {
			return false, nil
		})
		require.NoError(t, err)
	})

	t.Run("non-retryable error", func(t *testing.T) {
		cfg := RetryConfig{MaxRetries: 3, BaseDelay: 10 * time.Millisecond}
		err := Retry(context.Background(), cfg, func(ctx context.Context) (bool, error) {
			return false, io.EOF
		})
		require.Error(t, err)
		assert.Equal(t, io.EOF.Error(), err.Error())
	})

	t.Run("retryable error exhausted", func(t *testing.T) {
		cfg := RetryConfig{MaxRetries: 2, BaseDelay: 10 * time.Millisecond}
		err := Retry(context.Background(), cfg, func(ctx context.Context) (bool, error) {
			return true, io.EOF // always retryable
		})
		require.Error(t, err)
		assert.Equal(t, io.EOF.Error(), err.Error())
	})

	t.Run("success after retry", func(t *testing.T) {
		cfg := RetryConfig{MaxRetries: 3, BaseDelay: 10 * time.Millisecond}
		callCount := 0
		err := Retry(context.Background(), cfg, func(ctx context.Context) (bool, error) {
			callCount++
			if callCount == 1 {
				return true, io.EOF // retryable on first call
			}
			return false, nil // success on second call
		})
		require.NoError(t, err)
		assert.Equal(t, 2, callCount)
	})

	t.Run("context cancellation", func(t *testing.T) {
		cfg := RetryConfig{MaxRetries: 3, BaseDelay: 100 * time.Millisecond}
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()
		err := Retry(ctx, cfg, func(ctx context.Context) (bool, error) {
			return true, io.EOF
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context")
	})
}

func TestIntegrationFullWorkflow(t *testing.T) {
	// Test a complete workflow: heartbeat -> fetch config -> report status
	var heartbeatReceived, configFetched, statusReported bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/agents/heartbeat":
			heartbeatReceived = true
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(envelope[map[string]time.Time]{Success: true})

		case "/api/agents/config":
			configFetched = true
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(envelope[Config]{
				Success: true,
				Data: Config{
					Version:   1,
					Timestamp: time.Now().UTC(),
				},
			})

		case "/api/agents/status":
			statusReported = true
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(envelope[Status]{Success: true})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := NewFastifyClient(Options{
		BaseURL: server.URL,
		Token:   "test-token",
		Secret:  "test-secret",
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx := context.Background()

	// Heartbeat
	if err := client.Heartbeat(ctx); err != nil {
		t.Errorf("Heartbeat() failed = %v", err)
	}

	// Fetch config
	cfg, err := client.FetchConfig(ctx)
	if err != nil {
		t.Errorf("FetchConfig() failed = %v", err)
	}
	if cfg == nil {
		t.Errorf("FetchConfig() returned nil config")
		return
	}

	// Report status
	status := Status{
		State:     "running",
		Version:   cfg.Version,
		UpdatedAt: time.Now().UTC(),
	}
	if err := client.ReportStatus(ctx, status); err != nil {
		t.Errorf("ReportStatus() failed = %v", err)
	}

	// Verify all endpoints were called
	if !heartbeatReceived {
		t.Error("Heartbeat endpoint was not called")
	}
	if !configFetched {
		t.Error("Config endpoint was not called")
	}
	if !statusReported {
		t.Error("Status endpoint was not called")
	}
}
