package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/the-lpn-foundation/sing-box-agent/internal/singbox"
	syncpkg "github.com/the-lpn-foundation/sing-box-agent/internal/sync"
)

// HealthHandler returns 200 OK with "OK" body.
// This endpoint indicates the server is running.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// ReadyzHandler returns 200 if the server is ready to serve requests.
// Returns 503 if not ready (sing-box not running or sync not completed).
func ReadyzHandler(syncEngine *syncpkg.Engine, coreService *singbox.CoreServiceAdapter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		ready := true
		checks := make(map[string]string)

		// Check sing-box systemd service status
		switch {
		case coreService == nil:
			checks["singbox"] = "not_configured"
			ready = false
		case coreService.IsActive():
			checks["singbox"] = "running"
		default:
			checks["singbox"] = "not_running"
			ready = false
		}
		// Check sync engine status
		if syncEngine != nil {
			syncStatus := syncEngine.Status()
			if syncStatus != nil {
				statusMap := syncStatus.Get()
				if state, ok := statusMap["state"].(string); ok {
					checks["sync"] = state
					if state == "error" {
						ready = false
					}
				}
			}
		} else {
			checks["sync"] = "not_configured"
		}

		response := map[string]interface{}{
			"ready":  ready,
			"checks": checks,
		}

		if ready {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		_ = json.NewEncoder(w).Encode(response)
	}
}

// StatusResponse represents the server status response.
type StatusResponse struct {
	Version    string                 `json:"version"`
	Uptime     float64                `json:"uptime_seconds"`
	Status     string                 `json:"status"`
	Timestamp  int64                  `json:"timestamp"`
	SyncState  string                 `json:"sync_state,omitempty"`
	SyncStatus map[string]interface{} `json:"sync_status,omitempty"`
}

// StatusHandler returns server status as JSON.
// Includes version, uptime, sync state, and current status.
func StatusHandler(startTime time.Time, version string, syncEngine *syncpkg.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := StatusResponse{
			Version:   version,
			Uptime:    time.Since(startTime).Seconds(),
			Status:    "running",
			Timestamp: time.Now().Unix(),
		}

		if syncEngine != nil {
			syncStatus := syncEngine.Status()
			if syncStatus != nil {
				statusMap := syncStatus.Get()
				if state, ok := statusMap["state"].(string); ok {
					status.SyncState = state
				}
				status.SyncStatus = statusMap
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(status)
	}
}
