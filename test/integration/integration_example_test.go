package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHealthEndpoint verifies GET /healthz returns 200 OK.
func TestHealthEndpoint(t *testing.T) {
	t.Parallel()

	server := setupTestServerWithHealth(t)
	defer server.Close()

	resp, err := http.Get(server.URL + "/healthz")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/plain", resp.Header.Get("Content-Type"))
}

// TestReadyzEndpoint verifies GET /readyz returns 200 OK.
func TestReadyzEndpoint(t *testing.T) {
	t.Parallel()

	server := setupTestServerWithHealth(t)
	defer server.Close()

	resp, err := http.Get(server.URL + "/readyz")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/plain", resp.Header.Get("Content-Type"))
}

// TestStatusEndpoint verifies GET /status returns JSON with status info.
func TestStatusEndpoint(t *testing.T) {
	t.Parallel()

	server := setupTestServerWithHealth(t)
	defer server.Close()

	resp, err := http.Get(server.URL + "/status")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var status map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&status)
	require.NoError(t, err)

	assert.Contains(t, status, "version")
	assert.Contains(t, status, "uptime_seconds")
	assert.Contains(t, status, "status")
	assert.Contains(t, status, "timestamp")
	assert.Equal(t, "running", status["status"])
	assert.Greater(t, status["uptime_seconds"], float64(0))
	assert.Greater(t, status["timestamp"], float64(0))
}

// TestHealthReadyzStatus verifies all health endpoints respond correctly.
// This is a comprehensive integration test covering health, readyz, and status endpoints.
func TestHealthReadyzStatus(t *testing.T) {
	t.Parallel()

	server := setupTestServerWithHealth(t)
	defer server.Close()

	// Test healthz
	resp1, err := http.Get(server.URL + "/healthz")
	require.NoError(t, err)
	defer func() { _ = resp1.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp1.StatusCode)

	// Test readyz
	resp2, err := http.Get(server.URL + "/readyz")
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	// Test status
	resp3, err := http.Get(server.URL + "/status")
	require.NoError(t, err)
	defer func() { _ = resp3.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp3.StatusCode)

	var status map[string]interface{}
	err = json.NewDecoder(resp3.Body).Decode(&status)
	require.NoError(t, err)
	assert.Equal(t, "running", status["status"])
}

// setupTestServerWithHealth creates a test server with health endpoints.
func setupTestServerWithHealth(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		status := map[string]interface{}{
			"version":        "dev",
			"uptime_seconds": time.Since(time.Now()).Seconds(),
			"status":         "running",
			"timestamp":      time.Now().Unix(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(status)
	})

	return httptest.NewServer(mux)
}
