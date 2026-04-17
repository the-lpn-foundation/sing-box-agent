package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/lenya/sing-box-agent/internal/models"
	syncpkg "github.com/lenya/sing-box-agent/internal/sync"
)

const (
	CodeVersionConflict = "VERSION_CONFLICT"
	CodeSyncFailed      = "SYNC_FAILED"
)

// SyncHandler handles desired-state sync operations.
type SyncHandler struct {
	syncEngine *syncpkg.Engine
	singbox    syncpkg.SingBoxClient
	logger     *slog.Logger
}

// NewSyncHandler creates a new sync handler.
func NewSyncHandler(syncEngine *syncpkg.Engine, singbox syncpkg.SingBoxClient, logger *slog.Logger) *SyncHandler {
	return &SyncHandler{
		syncEngine: syncEngine,
		singbox:    singbox,
		logger:     logger,
	}
}

// ApplyDesiredState handles POST /sync/desired-state.
// Receives a desired state, reconciles with current state, and applies changes.
func (h *SyncHandler) ApplyDesiredState(w http.ResponseWriter, r *http.Request) {
	var desired models.DesiredState
	if err := json.NewDecoder(r.Body).Decode(&desired); err != nil {
		h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	if desired.Version <= 0 {
		h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, "version must be a positive integer")
		return
	}

	// Check version conflict
	status := h.syncEngine.Status()
	if status != nil && desired.Version <= status.CurrentVersion {
		h.sendError(w, http.StatusConflict, CodeVersionConflict,
			fmt.Sprintf("version %d is not newer than current version %d", desired.Version, status.CurrentVersion))
		return
	}

	// Reconcile: diff current vs desired
	plan, err := h.syncEngine.Reconcile(r.Context(), &desired)
	if err != nil {
		h.logger.Error("reconcile failed", slog.Any("error", err))
		h.sendError(w, http.StatusInternalServerError, CodeSyncFailed, fmt.Sprintf("Reconcile failed: %v", err))
		return
	}

	// Apply the plan
	if err := h.syncEngine.Apply(r.Context(), plan, desired.Version); err != nil {
		h.logger.Error("apply failed", slog.Any("error", err))
		h.sendError(w, http.StatusInternalServerError, CodeSyncFailed, fmt.Sprintf("Apply failed: %v", err))
		return
	}

	result := models.SyncResult{
		Success:        true,
		ChangesApplied: plan.StepsCount(),
		Version:        desired.Version,
	}

	h.logger.Info("desired state applied",
		slog.Int("version", desired.Version),
		slog.Int("changes", plan.StepsCount()),
	)

	h.sendSuccess(w, http.StatusOK, result)
}

// GetSyncStatus handles GET /sync/status.
// Returns the current sync engine status.
func (h *SyncHandler) GetSyncStatus(w http.ResponseWriter, r *http.Request) {
	status := h.syncEngine.Status()
	if status == nil {
		h.sendSuccess(w, http.StatusOK, map[string]interface{}{
			"state":   "unknown",
			"version": -1,
			"in_sync": false,
		})
		return
	}

	h.sendSuccess(w, http.StatusOK, status.Get())
}

func (h *SyncHandler) sendSuccess(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := Response{
		Success: true,
		Data:    data,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil && h.logger != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

func (h *SyncHandler) sendError(w http.ResponseWriter, statusCode int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := Response{
		Success: false,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
		},
	}

	if err := json.NewEncoder(w).Encode(response); err != nil && h.logger != nil {
		h.logger.Error("failed to encode error response", "error", err)
	}
}
