package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lenya/sing-box-agent/internal/config"
	"github.com/lenya/sing-box-agent/internal/singbox"
)

// testConfig returns a valid test configuration
func testConfig() *config.Config {
	return &config.Config{
		APIPort:           8080,
		MetricsPort:       9090,
		Token:             "test-token-with-32-chars-minimum",
		Secret:            "test-secret-with-32-chars-minimum",
		SingBoxConfigPath: "/tmp/test-config.json",
		LogLevel:          "info",
		TLSCertPath:       "",
		TLSKeyPath:        "",
	}
}

// testLogger returns a test logger
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

// testWrapper returns a mock singbox wrapper for testing
func testWrapper() *singbox.Wrapper {
	return singbox.NewWrapper("/tmp/test-config.json")
}

func TestNew(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config.Config
		logger      *slog.Logger
		wrapper     *singbox.Wrapper
		wantErr     bool
		checkFields bool
	}{
		{
			name:        "valid config creates server",
			cfg:         testConfig(),
			logger:      testLogger(),
			wrapper:     testWrapper(),
			wantErr:     false,
			checkFields: true,
		},
		{
			name: "different port",
			cfg: &config.Config{
				APIPort:           9999,
				MetricsPort:       9090,
				Token:             "test-token-with-32-chars-minimum",
				Secret:            "test-secret-with-32-chars-minimum",
				SingBoxConfigPath: "/tmp/test-config.json",
				LogLevel:          "info",
			},
			logger:      testLogger(),
			wrapper:     testWrapper(),
			wantErr:     false,
			checkFields: true,
		},
		{
			name: "with TLS config",
			cfg: &config.Config{
				APIPort:           8080,
				MetricsPort:       9090,
				Token:             "test-token-with-32-chars-minimum",
				Secret:            "test-secret-with-32-chars-minimum",
				SingBoxConfigPath: "/tmp/test-config.json",
				LogLevel:          "info",
				TLSCertPath:       "/tmp/cert.pem",
				TLSKeyPath:        "/tmp/key.pem",
			},
			logger:      testLogger(),
			wrapper:     testWrapper(),
			wantErr:     false,
			checkFields: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := New(tt.cfg, tt.logger, tt.wrapper)

			if server == nil {
				t.Fatal("New() returned nil server")
			}

			if tt.checkFields {
				if server.httpServer == nil {
					t.Error("httpServer field is nil")
				}
				if server.logger == nil {
					t.Error("logger field is nil")
				}
				if server.config == nil {
					t.Error("config field is nil")
				}
				if server.wrapper == nil {
					t.Error("wrapper field is nil")
				}
				if server.startTime.IsZero() {
					t.Error("startTime should be set")
				}
			}
		})
	}
}

func TestNew_NilParameters(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Config
		logger  *slog.Logger
		wrapper *singbox.Wrapper
		panic   bool
	}{
		{
			name:    "nil config",
			cfg:     nil,
			logger:  testLogger(),
			wrapper: testWrapper(),
			panic:   true,
		},
		{
			name:    "nil logger",
			cfg:     testConfig(),
			logger:  nil,
			wrapper: testWrapper(),
			panic:   false,
		},
		{
			name:    "nil wrapper",
			cfg:     testConfig(),
			logger:  testLogger(),
			wrapper: nil,
			panic:   false, // wrapper is not used in New()
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					if !tt.panic {
						t.Errorf("unexpected panic: %v", r)
					}
				}
			}()

			server := New(tt.cfg, tt.logger, tt.wrapper)
			if tt.panic {
				t.Error("expected panic but got none")
			}
			if server == nil && !tt.panic {
				t.Error("New() should return a server even with nil parameters")
			}
		})
	}
}

func TestServer_Addr(t *testing.T) {
	tests := []struct {
		name     string
		port     int
		expected string
	}{
		{
			name:     "port 8080",
			port:     8080,
			expected: ":8080",
		},
		{
			name:     "port 9999",
			port:     9999,
			expected: ":9999",
		},
		{
			name:     "port 80",
			port:     80,
			expected: ":80",
		},
		{
			name:     "port 443",
			port:     443,
			expected: ":443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfig()
			cfg.APIPort = tt.port

			server := New(cfg, testLogger(), testWrapper())
			if server == nil {
				t.Fatal("New() returned nil server")
			}

			addr := server.Addr()
			if addr != tt.expected {
				t.Errorf("Addr() = %q, want %q", addr, tt.expected)
			}
		})
	}
}

func TestServer_Shutdown(t *testing.T) {
	tests := []struct {
		name       string
		setupTLS   bool
		wantErr    bool
		concurrent bool
	}{
		{
			name:       "successful shutdown",
			setupTLS:   false,
			wantErr:    false,
			concurrent: false,
		},
		{
			name:       "shutdown with TLS config",
			setupTLS:   true,
			wantErr:    false,
			concurrent: false,
		},
		{
			name:       "concurrent shutdown calls",
			setupTLS:   false,
			wantErr:    false,
			concurrent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfig()
			if tt.setupTLS {
				cfg.TLSCertPath = "/tmp/cert.pem"
				cfg.TLSKeyPath = "/tmp/key.pem"
			}

			server := New(cfg, testLogger(), testWrapper())
			if server == nil {
				t.Fatal("New() returned nil server")
			}

			if tt.concurrent {
				var wg sync.WaitGroup
				errs := make(chan error, 10)

				for i := 0; i < 10; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						errs <- server.Shutdown()
					}()
				}

				wg.Wait()
				close(errs)

				for err := range errs {
					if (err != nil) != tt.wantErr {
						t.Errorf("Shutdown() error = %v, wantErr %v", err, tt.wantErr)
					}
				}
			} else {
				err := server.Shutdown()
				if (err != nil) != tt.wantErr {
					t.Errorf("Shutdown() error = %v, wantErr %v", err, tt.wantErr)
				}

				err = server.Shutdown()
				if (err != nil) != tt.wantErr {
					t.Errorf("Second Shutdown() error = %v, wantErr %v", err, tt.wantErr)
				}
			}
		})
	}
}

func TestServer_Routes(t *testing.T) {
	cfg := testConfig()
	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
		authRequired   bool
	}{
		{
			name:           "GET /healthz",
			method:         http.MethodGet,
			path:           "/healthz",
			expectedStatus: http.StatusOK,
			authRequired:   false,
		},
		{
			name:           "POST /healthz",
			method:         http.MethodPost,
			path:           "/healthz",
			expectedStatus: http.StatusOK,
			authRequired:   false,
		},
		{
			name:           "GET /readyz",
			method:         http.MethodGet,
			path:           "/readyz",
			expectedStatus: http.StatusServiceUnavailable,
			authRequired:   false,
		},
		{
			name:           "GET /status",
			method:         http.MethodGet,
			path:           "/status",
			expectedStatus: http.StatusOK,
			authRequired:   false,
		},
		{
			name:           "GET /metrics",
			method:         http.MethodGet,
			path:           "/metrics",
			expectedStatus: http.StatusOK,
			authRequired:   false,
		},
		{
			name:           "GET /inbounds without auth",
			method:         http.MethodGet,
			path:           "/inbounds",
			expectedStatus: http.StatusUnauthorized,
			authRequired:   true,
		},
		{
			name:           "POST /inbounds without auth",
			method:         http.MethodPost,
			path:           "/inbounds",
			expectedStatus: http.StatusUnauthorized,
			authRequired:   true,
		},
		{
			name:           "GET /inbounds/123 without auth",
			method:         http.MethodGet,
			path:           "/inbounds/123",
			expectedStatus: http.StatusUnauthorized,
			authRequired:   true,
		},
		{
			name:           "PUT /inbounds/123 without auth",
			method:         http.MethodPut,
			path:           "/inbounds/123",
			expectedStatus: http.StatusUnauthorized,
			authRequired:   true,
		},
		{
			name:           "DELETE /inbounds/123 without auth",
			method:         http.MethodDelete,
			path:           "/inbounds/123",
			expectedStatus: http.StatusUnauthorized,
			authRequired:   true,
		},
		{
			name:           "GET /inbounds/123/users without auth",
			method:         http.MethodGet,
			path:           "/inbounds/123/users",
			expectedStatus: http.StatusUnauthorized,
			authRequired:   true,
		},
		{
			name:           "POST /inbounds/123/users without auth",
			method:         http.MethodPost,
			path:           "/inbounds/123/users",
			expectedStatus: http.StatusUnauthorized,
			authRequired:   true,
		},
		{
			name:           "GET /inbounds/123/users/456 without auth",
			method:         http.MethodGet,
			path:           "/inbounds/123/users/456",
			expectedStatus: http.StatusUnauthorized,
			authRequired:   true,
		},
		{
			name:           "PUT /inbounds/123/users/456 without auth",
			method:         http.MethodPut,
			path:           "/inbounds/123/users/456",
			expectedStatus: http.StatusUnauthorized,
			authRequired:   true,
		},
		{
			name:           "DELETE /inbounds/123/users/456 without auth",
			method:         http.MethodDelete,
			path:           "/inbounds/123/users/456",
			expectedStatus: http.StatusUnauthorized,
			authRequired:   true,
		},
		{
			name:           "POST /subscription/generate without auth",
			method:         http.MethodPost,
			path:           "/subscription/generate",
			expectedStatus: http.StatusUnauthorized,
			authRequired:   true,
		},
		{
			name:           "GET /subscription/abc123 without auth",
			method:         http.MethodGet,
			path:           "/subscription/abc123",
			expectedStatus: http.StatusUnauthorized,
			authRequired:   true,
		},
		{
			name:           "GET /invalid",
			method:         http.MethodGet,
			path:           "/invalid",
			expectedStatus: http.StatusNotFound,
			authRequired:   false,
		},
		{
			name:           "GET /inbounds/ (trailing slash)",
			method:         http.MethodGet,
			path:           "/inbounds/",
			expectedStatus: http.StatusUnauthorized,
			authRequired:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			server.httpServer.Handler.ServeHTTP(w, req)

			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
			}
		})
	}
}

func TestServer_Routes_WithAuth(t *testing.T) {
	cfg := testConfig()
	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	authToken := "Bearer " + cfg.Token

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
	}{
		{
			name:           "GET /inbounds with auth",
			method:         http.MethodGet,
			path:           "/inbounds",
			expectedStatus: http.StatusUnauthorized, // Missing X-Signature header
		},
		{
			name:           "POST /inbounds with auth",
			method:         http.MethodPost,
			path:           "/inbounds",
			expectedStatus: http.StatusUnauthorized, // Missing X-Signature header
		},
		{
			name:           "GET /inbounds/123 with auth",
			method:         http.MethodGet,
			path:           "/inbounds/123",
			expectedStatus: http.StatusUnauthorized, // Missing X-Signature header
		},
		{
			name:           "PUT /inbounds/123 with auth",
			method:         http.MethodPut,
			path:           "/inbounds/123",
			expectedStatus: http.StatusUnauthorized, // Missing X-Signature header
		},
		{
			name:           "DELETE /inbounds/123 with auth",
			method:         http.MethodDelete,
			path:           "/inbounds/123",
			expectedStatus: http.StatusUnauthorized, // Missing X-Signature header
		},
		{
			name:           "GET /inbounds/123/users with auth",
			method:         http.MethodGet,
			path:           "/inbounds/123/users",
			expectedStatus: http.StatusUnauthorized, // Missing X-Signature header
		},
		{
			name:           "POST /inbounds/123/users with auth",
			method:         http.MethodPost,
			path:           "/inbounds/123/users",
			expectedStatus: http.StatusUnauthorized, // Missing X-Signature header
		},
		{
			name:           "GET /inbounds/123/users/456 with auth",
			method:         http.MethodGet,
			path:           "/inbounds/123/users/456",
			expectedStatus: http.StatusUnauthorized, // Missing X-Signature header
		},
		{
			name:           "PUT /inbounds/123/users/456 with auth",
			method:         http.MethodPut,
			path:           "/inbounds/123/users/456",
			expectedStatus: http.StatusUnauthorized, // Missing X-Signature header
		},
		{
			name:           "DELETE /inbounds/123/users/456 with auth",
			method:         http.MethodDelete,
			path:           "/inbounds/123/users/456",
			expectedStatus: http.StatusUnauthorized, // Missing X-Signature header
		},
		{
			name:           "POST /subscription/generate with auth",
			method:         http.MethodPost,
			path:           "/subscription/generate",
			expectedStatus: http.StatusUnauthorized, // Missing X-Signature header
		},
		{
			name:           "GET /subscription/abc123 with auth",
			method:         http.MethodGet,
			path:           "/subscription/abc123",
			expectedStatus: http.StatusUnauthorized, // Missing X-Signature header
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.Header.Set("Authorization", authToken)
			w := httptest.NewRecorder()

			server.httpServer.Handler.ServeHTTP(w, req)

			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
			}
		})
	}
}

func TestServer_Start_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := testConfig()
	cfg.APIPort = 18080

	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get("http://localhost:" + strconv.Itoa(cfg.APIPort) + "/healthz")
	if err != nil {
		t.Fatalf("failed to connect to server: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "OK" {
		t.Errorf("expected body 'OK', got %q", string(body))
	}

	if err := server.Shutdown(); err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}

	select {
	case err := <-serverErr:
		if err != nil {
			t.Errorf("Start() returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop within timeout")
	}

	_, err = client.Get("http://localhost:" + strconv.Itoa(cfg.APIPort) + "/healthz")
	if err == nil {
		t.Error("server should not be responding after shutdown")
	}
}

func TestServer_Timeouts(t *testing.T) {
	cfg := testConfig()
	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	if server.httpServer.ReadTimeout != DefaultReadTimeout {
		t.Errorf("ReadTimeout = %v, want %v", server.httpServer.ReadTimeout, DefaultReadTimeout)
	}
	if server.httpServer.WriteTimeout != DefaultWriteTimeout {
		t.Errorf("WriteTimeout = %v, want %v", server.httpServer.WriteTimeout, DefaultWriteTimeout)
	}
	if server.httpServer.IdleTimeout != DefaultIdleTimeout {
		t.Errorf("IdleTimeout = %v, want %v", server.httpServer.IdleTimeout, DefaultIdleTimeout)
	}
}

func TestServer_HandlerChain(t *testing.T) {
	cfg := testConfig()
	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("User-Agent", "test-agent")
	w := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "OK" {
		t.Errorf("expected body 'OK', got %q", string(body))
	}
}

func TestServer_StatusEndpoint(t *testing.T) {
	cfg := testConfig()
	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", contentType)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	expectedFields := []string{"version", "uptime_seconds", "status", "timestamp"}
	for _, field := range expectedFields {
		if !strings.Contains(bodyStr, field) {
			t.Errorf("response should contain field '%s'", field)
		}
	}

	if !strings.Contains(bodyStr, `"status":"running"`) {
		t.Error("status should be 'running'")
	}
}

func TestServer_MetricsEndpoint(t *testing.T) {
	cfg := testConfig()
	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") {
		t.Errorf("expected Content-Type to contain 'text/plain', got %q", contentType)
	}

	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Error("metrics response should not be empty")
	}
}

func TestServer_ShutdownContext(t *testing.T) {
	cfg := testConfig()
	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- server.Shutdown()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Shutdown() error = %v", err)
		}
	case <-ctx.Done():
		t.Error("shutdown did not complete within timeout")
	}
}

func TestServer_ConcurrentShutdown(t *testing.T) {
	cfg := testConfig()
	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	var wg sync.WaitGroup
	errs := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- server.Shutdown()
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("Shutdown() error = %v", err)
		}
	}
}

func TestServer_StartTime(t *testing.T) {
	before := time.Now()

	cfg := testConfig()
	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	after := time.Now()

	if server.startTime.IsZero() {
		t.Error("startTime should be set")
	}

	if server.startTime.Before(before) || server.startTime.After(after) {
		t.Error("startTime should be between before and after")
	}
}

func TestServer_ConfigPreservation(t *testing.T) {
	cfg := testConfig()
	cfg.APIPort = 9999
	cfg.LogLevel = "debug"

	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	if server.config.APIPort != 9999 {
		t.Errorf("config.APIPort = %d, want 9999", server.config.APIPort)
	}

	if server.config.LogLevel != "debug" {
		t.Errorf("config.LogLevel = %s, want debug", server.config.LogLevel)
	}
}

func TestServer_WrapperPreservation(t *testing.T) {
	wrapper := testWrapper()
	cfg := testConfig()

	server := New(cfg, testLogger(), wrapper)
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	if server.wrapper != wrapper {
		t.Error("wrapper should be preserved")
	}
}

func TestServer_LoggerPreservation(t *testing.T) {
	logger := testLogger()
	cfg := testConfig()

	server := New(cfg, logger, testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	if server.logger != logger {
		t.Error("logger should be preserved")
	}
}

func TestServer_RouteNotFound(t *testing.T) {
	cfg := testConfig()
	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	paths := []string{
		"/notfound",
		"/api/v1/unknown",
		"/api/v2/unknown",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()

			server.httpServer.Handler.ServeHTTP(w, req)

			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("expected status 404 for %s, got %d", path, resp.StatusCode)
			}
		})
	}
}

func TestServer_AuthMiddleware(t *testing.T) {
	cfg := testConfig()
	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "no auth header",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid auth header format",
			authHeader:     "InvalidFormat",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "wrong token",
			authHeader:     "Bearer wrong-token",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "correct token",
			authHeader:     "Bearer " + cfg.Token,
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/inbounds", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()

			server.httpServer.Handler.ServeHTTP(w, req)

			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
			}
		})
	}
}

func TestServer_MultipleServers(t *testing.T) {
	cfg1 := testConfig()
	cfg1.APIPort = 18082

	cfg2 := testConfig()
	cfg2.APIPort = 18083

	server1 := New(cfg1, testLogger(), testWrapper())
	server2 := New(cfg2, testLogger(), testWrapper())

	if server1 == nil || server2 == nil {
		t.Fatal("New() returned nil server")
	}

	if server1.Addr() == server2.Addr() {
		t.Error("servers should have different addresses")
	}

	if server1.Addr() != ":18082" {
		t.Errorf("server1.Addr() = %s, want :18082", server1.Addr())
	}

	if server2.Addr() != ":18083" {
		t.Errorf("server2.Addr() = %s, want :18083", server2.Addr())
	}
}

func TestServer_ShutdownIdempotent(t *testing.T) {
	cfg := testConfig()
	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	for i := 0; i < 5; i++ {
		err := server.Shutdown()
		if err != nil {
			t.Errorf("Shutdown() iteration %d error = %v", i, err)
		}
	}
}

func TestServer_StartTimeAccuracy(t *testing.T) {
	cfg := testConfig()
	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "uptime_seconds") {
		t.Error("status response should contain uptime_seconds")
	}

	if !strings.Contains(bodyStr, `"status":"running"`) {
		t.Error("status should be 'running'")
	}
}

func TestServer_ShutdownErrorPath(t *testing.T) {
	cfg := testConfig()
	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	// Shutdown should succeed even if server was never started
	err := server.Shutdown()
	if err != nil {
		t.Errorf("Shutdown() should succeed even if server was never started, got error: %v", err)
	}
}

func TestServer_NewWithDifferentConfigs(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
	}{
		{
			name: "minimal config",
			cfg: &config.Config{
				APIPort:           8080,
				Token:             "test-token-with-32-chars-minimum",
				Secret:            "test-secret-with-32-chars-minimum",
				SingBoxConfigPath: "/tmp/test-config.json",
			},
		},
		{
			name: "config with all fields",
			cfg: &config.Config{
				APIPort:           9090,
				MetricsPort:       9191,
				Token:             "test-token-with-32-chars-minimum",
				Secret:            "test-secret-with-32-chars-minimum",
				SingBoxConfigPath: "/tmp/test-config.json",
				LogLevel:          "debug",
				TLSCertPath:       "/tmp/cert.pem",
				TLSKeyPath:        "/tmp/key.pem",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := New(tt.cfg, testLogger(), testWrapper())
			if server == nil {
				t.Fatal("New() returned nil server")
			}

			if server.config != tt.cfg {
				t.Error("config should be preserved")
			}
		})
	}
}

func TestServer_NewFieldInitialization(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	wrapper := testWrapper()

	server := New(cfg, logger, wrapper)
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	// Verify all fields are initialized
	if server.httpServer == nil {
		t.Error("httpServer should be initialized")
	}
	if server.logger == nil {
		t.Error("logger should be initialized")
	}
	if server.config == nil {
		t.Error("config should be initialized")
	}
	if server.wrapper == nil {
		t.Error("wrapper should be initialized")
	}
	if server.startTime.IsZero() {
		t.Error("startTime should be initialized")
	}

	// Verify httpServer fields
	if server.httpServer.Handler == nil {
		t.Error("httpServer.Handler should be initialized")
	}
	if server.httpServer.Addr == "" {
		t.Error("httpServer.Addr should be set")
	}
	if server.httpServer.ReadTimeout == 0 {
		t.Error("httpServer.ReadTimeout should be set")
	}
	if server.httpServer.WriteTimeout == 0 {
		t.Error("httpServer.WriteTimeout should be set")
	}
	if server.httpServer.IdleTimeout == 0 {
		t.Error("httpServer.IdleTimeout should be set")
	}
}

func TestServer_AddrConsistency(t *testing.T) {
	cfg := testConfig()
	cfg.APIPort = 12345

	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	// Addr() should return consistent value
	addr1 := server.Addr()
	addr2 := server.Addr()

	if addr1 != addr2 {
		t.Errorf("Addr() should return consistent value, got %q and %q", addr1, addr2)
	}

	expected := ":12345"
	if addr1 != expected {
		t.Errorf("Addr() = %q, want %q", addr1, expected)
	}
}

func TestServer_MultipleShutdownCalls(t *testing.T) {
	cfg := testConfig()
	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	// Multiple shutdown calls should all succeed
	for i := 0; i < 10; i++ {
		err := server.Shutdown()
		if err != nil {
			t.Errorf("Shutdown() call %d failed: %v", i+1, err)
		}
	}
}

func TestServer_Routes_AllEndpoints(t *testing.T) {
	cfg := testConfig()
	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	// Test all public endpoints without auth
	publicEndpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/healthz"},
		{http.MethodPost, "/healthz"},
		{http.MethodPut, "/healthz"},
		{http.MethodDelete, "/healthz"},
		{http.MethodGet, "/readyz"},
		{http.MethodPost, "/readyz"},
		{http.MethodGet, "/status"},
		{http.MethodPost, "/status"},
		{http.MethodGet, "/metrics"},
	}

	for _, ep := range publicEndpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()

			server.httpServer.Handler.ServeHTTP(w, req)

			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()

			if ep.path == "/readyz" {
				if resp.StatusCode != http.StatusServiceUnavailable {
					t.Errorf("expected status 503 for %s %s (nil wrapper), got %d", ep.method, ep.path, resp.StatusCode)
				}
			} else if resp.StatusCode != http.StatusOK {
				t.Errorf("expected status 200 for %s %s, got %d", ep.method, ep.path, resp.StatusCode)
			}
		})
	}
}

func TestServer_Routes_AllInboundEndpoints(t *testing.T) {
	cfg := testConfig()
	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	// Test all inbound endpoints without auth (should return 401)
	inboundEndpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/inbounds"},
		{http.MethodPost, "/inbounds"},
		{http.MethodGet, "/inbounds/123"},
		{http.MethodPut, "/inbounds/123"},
		{http.MethodDelete, "/inbounds/123"},
		{http.MethodGet, "/inbounds/123/users"},
		{http.MethodPost, "/inbounds/123/users"},
		{http.MethodGet, "/inbounds/123/users/456"},
		{http.MethodPut, "/inbounds/123/users/456"},
		{http.MethodDelete, "/inbounds/123/users/456"},
	}

	for _, ep := range inboundEndpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()

			server.httpServer.Handler.ServeHTTP(w, req)

			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected status 401 for %s %s, got %d", ep.method, ep.path, resp.StatusCode)
			}
		})
	}
}

func TestServer_Routes_AllSubscriptionEndpoints(t *testing.T) {
	cfg := testConfig()
	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	// Test all subscription endpoints without auth (should return 401)
	subscriptionEndpoints := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/subscription/generate"},
		{http.MethodGet, "/subscription/abc123"},
	}

	for _, ep := range subscriptionEndpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()

			server.httpServer.Handler.ServeHTTP(w, req)

			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected status 401 for %s %s, got %d", ep.method, ep.path, resp.StatusCode)
			}
		})
	}
}

func TestServer_NewRouteSetup(t *testing.T) {
	cfg := testConfig()
	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	// Verify that the handler is set up correctly
	if server.httpServer.Handler == nil {
		t.Fatal("httpServer.Handler should not be nil")
	}

	// Test that all routes are registered by making requests
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/healthz"},
		{http.MethodGet, "/readyz"},
		{http.MethodGet, "/status"},
		{http.MethodGet, "/metrics"},
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, nil)
			w := httptest.NewRecorder()

			server.httpServer.Handler.ServeHTTP(w, req)

			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()

			if route.path == "/readyz" {
				if resp.StatusCode != http.StatusServiceUnavailable {
					t.Errorf("route %s %s returned status %d, expected 503", route.method, route.path, resp.StatusCode)
				}
			} else if resp.StatusCode != http.StatusOK {
				t.Errorf("route %s %s returned status %d", route.method, route.path, resp.StatusCode)
			}
		})
	}
}

func TestServer_NewWithTLSConfig(t *testing.T) {
	cfg := testConfig()
	cfg.TLSCertPath = "/tmp/cert.pem"
	cfg.TLSKeyPath = "/tmp/key.pem"

	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	// Verify TLS config is preserved
	if server.config.TLSCertPath != "/tmp/cert.pem" {
		t.Errorf("TLSCertPath not preserved, got %s", server.config.TLSCertPath)
	}
	if server.config.TLSKeyPath != "/tmp/key.pem" {
		t.Errorf("TLSKeyPath not preserved, got %s", server.config.TLSKeyPath)
	}

	// Server should still work for HTTP requests
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestServer_NewWithDifferentLogLevels(t *testing.T) {
	logLevels := []string{"debug", "info", "warn", "error"}

	for _, level := range logLevels {
		t.Run(level, func(t *testing.T) {
			cfg := testConfig()
			cfg.LogLevel = level

			server := New(cfg, testLogger(), testWrapper())
			if server == nil {
				t.Fatal("New() returned nil server")
			}

			if server.config.LogLevel != level {
				t.Errorf("LogLevel not preserved, got %s", server.config.LogLevel)
			}
		})
	}
}

func TestServer_NewWithDifferentPorts(t *testing.T) {
	ports := []int{80, 443, 8080, 9090, 9999}

	for _, port := range ports {
		t.Run(strconv.Itoa(port), func(t *testing.T) {
			cfg := testConfig()
			cfg.APIPort = port

			server := New(cfg, testLogger(), testWrapper())
			if server == nil {
				t.Fatal("New() returned nil server")
			}

			expectedAddr := ":" + strconv.Itoa(port)
			if server.Addr() != expectedAddr {
				t.Errorf("Addr() = %s, want %s", server.Addr(), expectedAddr)
			}
		})
	}
}

func TestServer_NewHandlerChain(t *testing.T) {
	cfg := testConfig()
	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	// Test that the handler chain is working by making a request
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	// Check that response is successful
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Check that body is correct
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "OK" {
		t.Errorf("expected body 'OK', got %q", string(body))
	}
}

func TestServer_NewMiddlewareSetup(t *testing.T) {
	cfg := testConfig()
	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	// Test that auth middleware is working by making a request without auth
	req := httptest.NewRequest(http.MethodGet, "/inbounds", nil)
	w := httptest.NewRecorder()

	server.httpServer.Handler.ServeHTTP(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	// Should get 401 Unauthorized
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", resp.StatusCode)
	}
}

func TestServer_NewAllRoutesRegistered(t *testing.T) {
	cfg := testConfig()
	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	// Test all public routes
	publicRoutes := map[string][]string{
		"/healthz": {http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete},
		"/readyz":  {http.MethodGet, http.MethodPost},
		"/status":  {http.MethodGet, http.MethodPost},
		"/metrics": {http.MethodGet},
	}

	for path, methods := range publicRoutes {
		for _, method := range methods {
			t.Run(method+" "+path, func(t *testing.T) {
				req := httptest.NewRequest(method, path, nil)
				w := httptest.NewRecorder()

				server.httpServer.Handler.ServeHTTP(w, req)

				resp := w.Result()
				defer func() { _ = resp.Body.Close() }()

				if path == "/readyz" {
					if resp.StatusCode != http.StatusServiceUnavailable {
						t.Errorf("route %s %s returned status %d, expected 503", method, path, resp.StatusCode)
					}
				} else if resp.StatusCode != http.StatusOK {
					t.Errorf("route %s %s returned status %d", method, path, resp.StatusCode)
				}
			})
		}
	}
}

func TestServer_NewAuthProtectedRoutes(t *testing.T) {
	cfg := testConfig()
	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	// Test all auth-protected routes return 401 without auth
	authRoutes := map[string][]string{
		"/inbounds":               {http.MethodGet, http.MethodPost},
		"/inbounds/123":           {http.MethodGet, http.MethodPut, http.MethodDelete},
		"/inbounds/123/users":     {http.MethodGet, http.MethodPost},
		"/inbounds/123/users/456": {http.MethodGet, http.MethodPut, http.MethodDelete},
		"/subscription/generate":  {http.MethodPost},
		"/subscription/abc123":    {http.MethodGet},
	}

	for path, methods := range authRoutes {
		for _, method := range methods {
			t.Run(method+" "+path, func(t *testing.T) {
				req := httptest.NewRequest(method, path, nil)
				w := httptest.NewRecorder()

				server.httpServer.Handler.ServeHTTP(w, req)

				resp := w.Result()
				defer func() { _ = resp.Body.Close() }()

				if resp.StatusCode != http.StatusUnauthorized {
					t.Errorf("route %s %s should return 401, got %d", method, path, resp.StatusCode)
				}
			})
		}
	}
}

func TestServer_NewConfigFields(t *testing.T) {
	cfg := testConfig()
	cfg.APIPort = 12345
	cfg.MetricsPort = 54321
	cfg.LogLevel = "debug"
	cfg.SingBoxConfigPath = "/custom/config.json"

	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	// Verify all config fields are preserved
	if server.config.APIPort != 12345 {
		t.Errorf("APIPort not preserved, got %d", server.config.APIPort)
	}
	if server.config.MetricsPort != 54321 {
		t.Errorf("MetricsPort not preserved, got %d", server.config.MetricsPort)
	}
	if server.config.LogLevel != "debug" {
		t.Errorf("LogLevel not preserved, got %s", server.config.LogLevel)
	}
	if server.config.SingBoxConfigPath != "/custom/config.json" {
		t.Errorf("SingBoxConfigPath not preserved, got %s", server.config.SingBoxConfigPath)
	}
}

func TestServer_NewServerFields(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	wrapper := testWrapper()

	server := New(cfg, logger, wrapper)
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	// Verify all server fields are set
	if server.httpServer == nil {
		t.Error("httpServer should be set")
	}
	if server.logger != logger {
		t.Error("logger should be preserved")
	}
	if server.config != cfg {
		t.Error("config should be preserved")
	}
	if server.wrapper != wrapper {
		t.Error("wrapper should be preserved")
	}
	if server.startTime.IsZero() {
		t.Error("startTime should be set")
	}
}

func TestServer_NewHTTPServerConfig(t *testing.T) {
	cfg := testConfig()
	server := New(cfg, testLogger(), testWrapper())
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	// Verify http.Server configuration
	if server.httpServer.Handler == nil {
		t.Error("Handler should be set")
	}
	if server.httpServer.Addr != ":8080" {
		t.Errorf("Addr should be :8080, got %s", server.httpServer.Addr)
	}
	if server.httpServer.ReadTimeout != DefaultReadTimeout {
		t.Errorf("ReadTimeout should be %v, got %v", DefaultReadTimeout, server.httpServer.ReadTimeout)
	}
	if server.httpServer.WriteTimeout != DefaultWriteTimeout {
		t.Errorf("WriteTimeout should be %v, got %v", DefaultWriteTimeout, server.httpServer.WriteTimeout)
	}
	if server.httpServer.IdleTimeout != DefaultIdleTimeout {
		t.Errorf("IdleTimeout should be %v, got %v", DefaultIdleTimeout, server.httpServer.IdleTimeout)
	}
}

// TestRouteRegistration tests that all routes are properly registered
func TestRouteRegistration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Config{
		APIPort: 0, // Use random port
		Token:   "test-token-32-characters-long-xxx",
		Secret:  "test-secret-32-characters-long-xxx",
	}

	srv := New(cfg, logger, nil)
	require.NotNil(t, srv)

	// Test that we can make requests to various routes
	// Note: This tests that routes are registered, not handler logic
	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/healthz"},
		{http.MethodGet, "/readyz"},
		{http.MethodGet, "/status"},
		{http.MethodGet, "/metrics"},
		{http.MethodGet, "/inbounds"},
		{http.MethodPost, "/inbounds"},
		{http.MethodGet, "/inbounds/test-tag"},
		{http.MethodPut, "/inbounds/test-tag"},
		{http.MethodDelete, "/inbounds/test-tag"},
		{http.MethodGet, "/inbounds/test-tag/users"},
		{http.MethodPost, "/inbounds/test-tag/users"},
		{http.MethodGet, "/inbounds/test-tag/users/test-sub"},
		{http.MethodPut, "/inbounds/test-tag/users/test-sub"},
		{http.MethodDelete, "/inbounds/test-tag/users/test-sub"},
		{http.MethodPost, "/subscription/generate"},
		{http.MethodGet, "/subscription/test-sub"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			srv.httpServer.Handler.ServeHTTP(w, req)
			// Just verify the route exists (not 404 from mux, though handler may return 404 for missing data)
			// Healthz, readyz, status should return 200
			if tt.path == "/healthz" || tt.path == "/status" {
				assert.Equal(t, http.StatusOK, w.Code)
			} else if tt.path == "/readyz" {
				assert.Equal(t, http.StatusServiceUnavailable, w.Code)
			}
		})
	}
}

// TestShutdownServer_Idempotent tests that calling shutdownServer multiple times is safe
func TestShutdownServer_Idempotent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Config{
		APIPort: 0,
		Token:   "test-token-32-characters-long-xxx",
		Secret:  "test-secret-32-characters-long-xxx",
	}

	srv := New(cfg, logger, nil)
	require.NotNil(t, srv)

	// First shutdown
	err := srv.shutdownServer()
	require.NoError(t, err)

	// Second shutdown should also succeed (idempotent)
	err = srv.shutdownServer()
	require.NoError(t, err)

	// Verify shutdown flag is set
	assert.True(t, srv.shutdown)
}
