package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

const redactedValue = "***REDACTED***"

type CoreService interface {
	Reload(ctx context.Context) error
	Restart(ctx context.Context) error
	GetConfig(ctx context.Context) (map[string]interface{}, error)
}

type CoreHandler struct {
	service CoreService
	logger  *slog.Logger
}

func NewCoreHandler(service CoreService, logger *slog.Logger) *CoreHandler {
	return &CoreHandler{service: service, logger: logger}
}

func (h *CoreHandler) ReloadConfig(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		h.sendError(w, http.StatusServiceUnavailable, CodeStatsUnavailable, "core service not configured")
		return
	}

	if err := h.service.Reload(r.Context()); err != nil {
		h.sendError(w, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("failed to reload core: %v", err))
		return
	}

	h.sendSuccess(w, http.StatusOK, map[string]string{"status": "reloaded"})
}

func (h *CoreHandler) RestartCore(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		h.sendError(w, http.StatusServiceUnavailable, CodeStatsUnavailable, "core service not configured")
		return
	}

	if err := h.service.Restart(r.Context()); err != nil {
		h.sendError(w, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("failed to restart core: %v", err))
		return
	}

	h.sendSuccess(w, http.StatusOK, map[string]string{"status": "restarted"})
}

func (h *CoreHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		h.sendError(w, http.StatusServiceUnavailable, CodeStatsUnavailable, "core service not configured")
		return
	}

	config, err := h.service.GetConfig(r.Context())
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("failed to get core config: %v", err))
		return
	}

	h.sendSuccess(w, http.StatusOK, redactConfig(config))
}

func redactConfig(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return nil
	}
	return redactMap(input)
}

func redactMap(input map[string]interface{}) map[string]interface{} {
	redacted := make(map[string]interface{}, len(input))
	for key, value := range input {
		switch key {
		case "private_key", "password", "short_id":
			redacted[key] = redactedValue
		default:
			redacted[key] = redactValue(value)
		}
	}
	return redacted
}

func redactSlice(input []interface{}) []interface{} {
	redacted := make([]interface{}, len(input))
	for i := range input {
		redacted[i] = redactValue(input[i])
	}
	return redacted
}

func redactValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return redactMap(typed)
	case []interface{}:
		return redactSlice(typed)
	default:
		return typed
	}
}

func (h *CoreHandler) sendSuccess(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := Response{Success: true, Data: data}
	if err := json.NewEncoder(w).Encode(response); err != nil && h.logger != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

func (h *CoreHandler) sendError(w http.ResponseWriter, statusCode int, code, message string) {
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
