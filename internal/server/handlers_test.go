package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHealthHandler(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		expectedStatus int
		expectedBody   string
		expectedCT     string
	}{
		{
			name:           "GET request returns OK",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			expectedBody:   "OK",
			expectedCT:     "text/plain",
		},
		{
			name:           "POST request returns OK",
			method:         http.MethodPost,
			expectedStatus: http.StatusOK,
			expectedBody:   "OK",
			expectedCT:     "text/plain",
		},
		{
			name:           "PUT request returns OK",
			method:         http.MethodPut,
			expectedStatus: http.StatusOK,
			expectedBody:   "OK",
			expectedCT:     "text/plain",
		},
		{
			name:           "DELETE request returns OK",
			method:         http.MethodDelete,
			expectedStatus: http.StatusOK,
			expectedBody:   "OK",
			expectedCT:     "text/plain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/healthz", nil)
			w := httptest.NewRecorder()

			HealthHandler(w, req)

			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
			}

			contentType := resp.Header.Get("Content-Type")
			if contentType != tt.expectedCT {
				t.Errorf("expected Content-Type '%s', got '%s'", tt.expectedCT, contentType)
			}

			body, _ := io.ReadAll(resp.Body)
			if string(body) != tt.expectedBody {
				t.Errorf("expected body '%s', got '%s'", tt.expectedBody, string(body))
			}
		})
	}
}

func TestReadyzHandler(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		expectedStatus int
		expectedBody   string
		expectedCT     string
	}{
		{
			name:           "GET request returns 503 with nil dependencies",
			method:         http.MethodGet,
			expectedStatus: http.StatusServiceUnavailable,
			expectedBody:   "",
			expectedCT:     "application/json",
		},
		{
			name:           "POST request returns 503 with nil dependencies",
			method:         http.MethodPost,
			expectedStatus: http.StatusServiceUnavailable,
			expectedBody:   "",
			expectedCT:     "application/json",
		},
		{
			name:           "HEAD request returns 503 with nil dependencies",
			method:         http.MethodHead,
			expectedStatus: http.StatusServiceUnavailable,
			expectedBody:   "",
			expectedCT:     "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/readyz", nil)
			w := httptest.NewRecorder()

			ReadyzHandler(nil, nil)(w, req)

			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
			}

			contentType := resp.Header.Get("Content-Type")
			if contentType != tt.expectedCT {
				t.Errorf("expected Content-Type '%s', got '%s'", tt.expectedCT, contentType)
			}

			if tt.expectedBody != "" {
				body, _ := io.ReadAll(resp.Body)
				if string(body) != tt.expectedBody {
					t.Errorf("expected body '%s', got '%s'", tt.expectedBody, string(body))
				}
			}
		})
	}
}

func TestStatusHandler(t *testing.T) {
	tests := []struct {
		name              string
		startTime         time.Time
		expectedStatus    int
		expectedVersion   string
		expectedStatusStr string
		minUptime         float64
	}{
		{
			name:              "server just started",
			startTime:         time.Now(),
			expectedStatus:    http.StatusOK,
			expectedVersion:   "dev",
			expectedStatusStr: "running",
			minUptime:         0,
		},
		{
			name:              "server running for 10 seconds",
			startTime:         time.Now().Add(-10 * time.Second),
			expectedStatus:    http.StatusOK,
			expectedVersion:   "dev",
			expectedStatusStr: "running",
			minUptime:         9.9,
		},
		{
			name:              "server running for 1 minute",
			startTime:         time.Now().Add(-1 * time.Minute),
			expectedStatus:    http.StatusOK,
			expectedVersion:   "dev",
			expectedStatusStr: "running",
			minUptime:         59.9,
		},
		{
			name:              "server running for 1 hour",
			startTime:         time.Now().Add(-1 * time.Hour),
			expectedStatus:    http.StatusOK,
			expectedVersion:   "dev",
			expectedStatusStr: "running",
			minUptime:         3599.9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := StatusHandler(tt.startTime, "dev", nil)

			req := httptest.NewRequest(http.MethodGet, "/status", nil)
			w := httptest.NewRecorder()

			handler(w, req)

			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
			}

			contentType := resp.Header.Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("expected Content-Type 'application/json', got '%s'", contentType)
			}

			var statusResp StatusResponse
			if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
				t.Fatalf("failed to decode JSON: %v", err)
			}

			if statusResp.Version != tt.expectedVersion {
				t.Errorf("expected version '%s', got '%s'", tt.expectedVersion, statusResp.Version)
			}

			if statusResp.Status != tt.expectedStatusStr {
				t.Errorf("expected status '%s', got '%s'", tt.expectedStatusStr, statusResp.Status)
			}

			if statusResp.Uptime < tt.minUptime {
				t.Errorf("expected uptime >= %f, got %f", tt.minUptime, statusResp.Uptime)
			}

			if statusResp.Timestamp == 0 {
				t.Error("expected non-zero timestamp")
			}

			// Verify timestamp is recent (within 1 second)
			now := time.Now().Unix()
			if now-statusResp.Timestamp > 1 {
				t.Errorf("timestamp too old: %d vs now %d", statusResp.Timestamp, now)
			}
		})
	}
}

func TestStatusHandler_Methods(t *testing.T) {
	startTime := time.Now()
	handler := StatusHandler(startTime, "dev", nil)

	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodHead,
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/status", nil)
			w := httptest.NewRecorder()

			handler(w, req)

			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()

			// All methods should return 200 OK
			if resp.StatusCode != http.StatusOK {
				t.Errorf("expected status 200 for %s, got %d", method, resp.StatusCode)
			}

			contentType := resp.Header.Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("expected Content-Type 'application/json', got '%s'", contentType)
			}
		})
	}
}

func TestStatusHandler_JSONStructure(t *testing.T) {
	startTime := time.Now()
	handler := StatusHandler(startTime, "dev", nil)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	// Verify it's valid JSON
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Check required fields exist
	requiredFields := []string{"version", "uptime_seconds", "status", "timestamp"}
	for _, field := range requiredFields {
		if _, ok := data[field]; !ok {
			t.Errorf("missing required field: %s", field)
		}
	}

	// Verify field types
	if _, ok := data["version"].(string); !ok {
		t.Error("version should be string")
	}
	if _, ok := data["uptime_seconds"].(float64); !ok {
		t.Error("uptime_seconds should be float64")
	}
	if _, ok := data["status"].(string); !ok {
		t.Error("status should be string")
	}
	if _, ok := data["timestamp"].(float64); !ok {
		t.Error("timestamp should be float64")
	}
}

func TestStatusHandler_Concurrent(t *testing.T) {
	startTime := time.Now()
	handler := StatusHandler(startTime, "dev", nil)

	// Test concurrent requests
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			req := httptest.NewRequest(http.MethodGet, "/status", nil)
			w := httptest.NewRecorder()
			handler(w, req)

			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("expected status 200, got %d", resp.StatusCode)
			}

			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestStatusHandler_ResponseFields(t *testing.T) {
	startTime := time.Now().Add(-5 * time.Second)
	handler := StatusHandler(startTime, "dev", nil)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	var statusResp StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	// Test each field
	t.Run("Version field", func(t *testing.T) {
		if statusResp.Version == "" {
			t.Error("version should not be empty")
		}
	})

	t.Run("Uptime field", func(t *testing.T) {
		if statusResp.Uptime <= 0 {
			t.Error("uptime should be positive")
		}
		expectedMin := 4.9 // 5 seconds minus small margin
		if statusResp.Uptime < expectedMin {
			t.Errorf("uptime should be at least %f, got %f", expectedMin, statusResp.Uptime)
		}
	})

	t.Run("Status field", func(t *testing.T) {
		if statusResp.Status != "running" {
			t.Errorf("expected status 'running', got '%s'", statusResp.Status)
		}
	})

	t.Run("Timestamp field", func(t *testing.T) {
		if statusResp.Timestamp == 0 {
			t.Error("timestamp should not be zero")
		}
	})
}

func TestStatusHandler_JSONEncoding(t *testing.T) {
	startTime := time.Now()
	handler := StatusHandler(startTime, "dev", nil)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	// Verify JSON is properly formatted
	var statusResp StatusResponse
	if err := json.Unmarshal(body, &statusResp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	// Re-marshal to verify it's valid
	reencoded, err := json.Marshal(statusResp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Verify the re-encoded JSON is also valid
	var redecoded StatusResponse
	if err := json.Unmarshal(reencoded, &redecoded); err != nil {
		t.Fatalf("failed to unmarshal re-encoded JSON: %v", err)
	}

	if redecoded.Version != statusResp.Version {
		t.Error("version mismatch after re-encoding")
	}
}

func TestStatusHandler_DifferentPaths(t *testing.T) {
	startTime := time.Now()
	handler := StatusHandler(startTime, "dev", nil)

	paths := []string{
		"/status",
		"/status/",
		"/status?foo=bar",
		"/status?debug=true",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()

			handler(w, req)

			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("expected status 200 for path %s, got %d", path, resp.StatusCode)
			}

			var statusResp StatusResponse
			if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
				t.Errorf("failed to decode JSON for path %s: %v", path, err)
			}
		})
	}
}

func TestStatusHandler_WithHeaders(t *testing.T) {
	startTime := time.Now()
	handler := StatusHandler(startTime, "dev", nil)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	req.Header.Set("User-Agent", "test-agent")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Custom-Header", "custom-value")

	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", contentType)
	}

	// Verify response body is valid JSON
	var statusResp StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		t.Errorf("failed to decode JSON: %v", err)
	}
}

func TestStatusHandler_BodySize(t *testing.T) {
	startTime := time.Now()
	handler := StatusHandler(startTime, "dev", nil)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	// Response should be reasonably sized (not empty, not huge)
	if len(body) == 0 {
		t.Error("response body should not be empty")
	}

	if len(body) > 1024 {
		t.Errorf("response body too large: %d bytes", len(body))
	}

	// Should contain expected JSON structure
	bodyStr := string(body)
	expectedFields := []string{"version", "uptime_seconds", "status", "timestamp"}
	for _, field := range expectedFields {
		if !strings.Contains(bodyStr, field) {
			t.Errorf("response body should contain field '%s'", field)
		}
	}
}
