package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/lenya/sing-box-agent/internal/models"
	syncpkg "github.com/lenya/sing-box-agent/internal/sync"
)

// Error codes for inbound operations.
const (
	CodeInvalidRequest  = "INVALID_REQUEST"
	CodeInboundNotFound = "INBOUND_NOT_FOUND"
	CodeInboundExists   = "INBOUND_EXISTS"
	CodeInternalError   = "INTERNAL_ERROR"
)

// ValidInboundTypes are the allowed inbound protocol types.
var ValidInboundTypes = []string{
	"trojan",
	"vless",
	"vmess",
	"shadowsocks",
	"hysteria",
	"hysteria2",
	"tuic",
}

// Response is the standard API response envelope.
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorInfo  `json:"error,omitempty"`
}

// ErrorInfo contains error details in the response.
type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// InboundHandler handles inbound CRUD operations.
type InboundHandler struct {
	syncEngine *syncpkg.Engine
	singbox    syncpkg.SingBoxClient
	logger     *slog.Logger
}

// NewInboundHandler creates a new inbound handler.
func NewInboundHandler(syncEngine *syncpkg.Engine, singbox syncpkg.SingBoxClient, logger *slog.Logger) *InboundHandler {
	return &InboundHandler{
		syncEngine: syncEngine,
		singbox:    singbox,
		logger:     logger,
	}
}

// ListInbounds returns all inbounds.
// GET /inbounds
func (h *InboundHandler) ListInbounds(w http.ResponseWriter, r *http.Request) {
	inbounds, err := h.singbox.GetInbounds(r.Context())
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("Failed to get inbounds: %v", err))
		return
	}

	h.sendSuccess(w, http.StatusOK, inbounds)
}

// GetInbound returns a single inbound by tag.
// GET /inbounds/{tag}
func (h *InboundHandler) GetInbound(w http.ResponseWriter, r *http.Request) {
	tag := strings.TrimPrefix(r.URL.Path, "/inbounds/")
	if tag == "" || tag == r.URL.Path {
		h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, "Missing inbound tag")
		return
	}

	inbounds, err := h.singbox.GetInbounds(r.Context())
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("Failed to get inbounds: %v", err))
		return
	}

	for _, inbound := range inbounds {
		if inbound.Tag == tag {
			h.sendSuccess(w, http.StatusOK, inbound)
			return
		}
	}

	h.sendError(w, http.StatusNotFound, CodeInboundNotFound, fmt.Sprintf("Inbound not found: %s", tag))
}

// CreateInbound creates a new inbound.
// POST /inbounds
func (h *InboundHandler) CreateInbound(w http.ResponseWriter, r *http.Request) {
	var inbound models.Inbound
	if err := json.NewDecoder(r.Body).Decode(&inbound); err != nil {
		h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	// Validate inbound
	if err := h.validateInbound(&inbound); err != nil {
		h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
		return
	}

	// Check if inbound already exists
	inbounds, err := h.singbox.GetInbounds(r.Context())
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("Failed to get inbounds: %v", err))
		return
	}

	for _, existing := range inbounds {
		if existing.Tag == inbound.Tag {
			h.sendError(w, http.StatusConflict, CodeInboundExists, fmt.Sprintf("Inbound already exists: %s", inbound.Tag))
			return
		}
	}

	// Create inbound via sync engine
	if err := h.singbox.CreateInbound(r.Context(), inbound); err != nil {
		h.sendError(w, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("Failed to create inbound: %v", err))
		return
	}

	// Trigger sync to apply changes
	h.triggerSync(r.Context())

	h.sendSuccess(w, http.StatusCreated, inbound)
}

// UpdateInbound updates an existing inbound.
// PUT /inbounds/{tag}
func (h *InboundHandler) UpdateInbound(w http.ResponseWriter, r *http.Request) {
	tag := strings.TrimPrefix(r.URL.Path, "/inbounds/")
	if tag == "" || tag == r.URL.Path {
		h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, "Missing inbound tag")
		return
	}

	var inbound models.Inbound
	if err := json.NewDecoder(r.Body).Decode(&inbound); err != nil {
		h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	// Ensure tag matches URL parameter
	if inbound.Tag != tag {
		h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, "Tag in request body must match URL parameter")
		return
	}

	// Validate inbound
	if err := h.validateInbound(&inbound); err != nil {
		h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
		return
	}

	// Check if inbound exists
	inbounds, err := h.singbox.GetInbounds(r.Context())
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("Failed to get inbounds: %v", err))
		return
	}

	found := false
	for _, existing := range inbounds {
		if existing.Tag == tag {
			found = true
			break
		}
	}

	if !found {
		h.sendError(w, http.StatusNotFound, CodeInboundNotFound, fmt.Sprintf("Inbound not found: %s", tag))
		return
	}

	// Update inbound via sync engine
	if err := h.singbox.UpdateInbound(r.Context(), inbound); err != nil {
		h.sendError(w, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("Failed to update inbound: %v", err))
		return
	}

	// Trigger sync to apply changes
	h.triggerSync(r.Context())

	h.sendSuccess(w, http.StatusOK, inbound)
}

// DeleteInbound deletes an inbound by tag.
// DELETE /inbounds/{tag}
func (h *InboundHandler) DeleteInbound(w http.ResponseWriter, r *http.Request) {
	tag := strings.TrimPrefix(r.URL.Path, "/inbounds/")
	if tag == "" || tag == r.URL.Path {
		h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, "Missing inbound tag")
		return
	}

	// Check if inbound exists
	inbounds, err := h.singbox.GetInbounds(r.Context())
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("Failed to get inbounds: %v", err))
		return
	}

	found := false
	for _, existing := range inbounds {
		if existing.Tag == tag {
			found = true
			break
		}
	}

	if !found {
		h.sendError(w, http.StatusNotFound, CodeInboundNotFound, fmt.Sprintf("Inbound not found: %s", tag))
		return
	}

	// Delete inbound via sync engine
	if err := h.singbox.DeleteInbound(r.Context(), tag); err != nil {
		h.sendError(w, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("Failed to delete inbound: %v", err))
		return
	}

	// Trigger sync to apply changes
	h.triggerSync(r.Context())

	w.WriteHeader(http.StatusNoContent)
}

// validateInbound validates an inbound configuration.
func (h *InboundHandler) validateInbound(inbound *models.Inbound) error {
	// Required fields
	if inbound.Tag == "" {
		return errors.New("tag is required")
	}

	if inbound.Type == "" {
		return errors.New("type is required")
	}

	if inbound.Listen == "" {
		return errors.New("listen is required")
	}

	// Validate type
	validType := false
	for _, t := range ValidInboundTypes {
		if inbound.Type == t {
			validType = true
			break
		}
	}

	if !validType {
		return fmt.Errorf("invalid inbound type: %s (must be one of: %s)", inbound.Type, strings.Join(ValidInboundTypes, ", "))
	}

	// Validate port (optional but if set, must be valid)
	if inbound.Port != 0 {
		if inbound.Port < 1 || inbound.Port > 65535 {
			return fmt.Errorf("invalid port: %d (must be between 1 and 65535)", inbound.Port)
		}
	}

	return nil
}

// triggerSync triggers a sync after inbound changes.
// It reads the current state from sing-box and reconciles it.
func (h *InboundHandler) triggerSync(ctx context.Context) {
	if h.syncEngine == nil {
		h.logger.Warn("sync engine not configured, skipping sync trigger")
		return
	}

	inbounds, err := h.singbox.GetInbounds(ctx)
	if err != nil {
		h.logger.Error("failed to get inbounds for sync", slog.Any("error", err))
		return
	}

	// Build desired state from current config state
	desired := &models.DesiredState{
		Inbounds: inbounds,
	}

	plan, err := h.syncEngine.Reconcile(ctx, desired)
	if err != nil {
		h.logger.Error("reconcile failed after inbound change", slog.Any("error", err))
		return
	}

	if plan.StepsCount() > 0 {
		if err := h.syncEngine.Apply(ctx, plan, 0); err != nil {
			h.logger.Error("apply failed after inbound change", slog.Any("error", err))
			return
		}
		h.logger.Info("sync completed after inbound change", slog.Int("changes", plan.StepsCount()))
	} else {
		h.logger.Debug("no sync changes needed after inbound update")
	}
}

// sendSuccess sends a successful response.
func (h *InboundHandler) sendSuccess(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := Response{
		Success: true,
		Data:    data,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

// sendError sends an error response.
func (h *InboundHandler) sendError(w http.ResponseWriter, statusCode int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := Response{
		Success: false,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
		},
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("failed to encode error response", "error", err)
	}
}
