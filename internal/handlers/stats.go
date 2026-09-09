package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/the-lpn-foundation/sing-box-agent/internal/models"
)

const (
	CodeStatsUnavailable = "SERVICE_UNAVAILABLE"
)

// StatsProvider is the interface for retrieving traffic and online user stats.
type StatsProvider interface {
	GetTrafficStats(ctx context.Context, inbound string, start, end *time.Time) ([]models.TrafficInboundStat, error)
	GetUserTrafficStats(ctx context.Context) ([]models.TrafficUserStat, error)
	GetOnlineUsers(ctx context.Context) ([]models.OnlineUser, error)
}

type StatsHandler struct {
	provider StatsProvider
	logger   *slog.Logger
}

func NewStatsHandler(provider StatsProvider, logger *slog.Logger) *StatsHandler {
	return &StatsHandler{provider: provider, logger: logger}
}

func (h *StatsHandler) GetTrafficStats(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		h.sendError(w, http.StatusServiceUnavailable, CodeStatsUnavailable, "stats provider not configured")
		return
	}

	inbound := r.URL.Query().Get("inbound")
	start, end, err := parseTimeRange(r)
	if err != nil {
		h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
		return
	}

	stats, err := h.provider.GetTrafficStats(r.Context(), inbound, start, end)
	if err != nil {
		h.sendError(w, http.StatusServiceUnavailable, CodeStatsUnavailable, fmt.Sprintf("failed to fetch traffic stats: %v", err))
		return
	}

	data := map[string]interface{}{
		"inbounds": stats,
	}
	// Per-user counters are cumulative since sing-box start — they are only
	// meaningful without a time filter (consumers compute their own deltas).
	if start == nil {
		users, err := h.provider.GetUserTrafficStats(r.Context())
		if err == nil {
			data["users"] = users
		}
	}
	if start != nil {
		data["start"] = start.Format(time.RFC3339)
	}
	if end != nil {
		data["end"] = end.Format(time.RFC3339)
	}

	h.sendSuccess(w, http.StatusOK, data)
}

func (h *StatsHandler) GetOnlineUsers(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		h.sendError(w, http.StatusServiceUnavailable, CodeStatsUnavailable, "stats provider not configured")
		return
	}

	users, err := h.provider.GetOnlineUsers(r.Context())
	if err != nil {
		h.sendError(w, http.StatusServiceUnavailable, CodeStatsUnavailable, fmt.Sprintf("failed to fetch online users: %v", err))
		return
	}

	h.sendSuccess(w, http.StatusOK, map[string]interface{}{
		"count": len(users),
		"users": users,
	})
}

func parseTimeRange(r *http.Request) (*time.Time, *time.Time, error) {
	startValue := r.URL.Query().Get("start")
	endValue := r.URL.Query().Get("end")

	if startValue == "" && endValue == "" {
		return nil, nil, nil
	}
	if startValue == "" || endValue == "" {
		return nil, nil, fmt.Errorf("both start and end are required when filtering by time range")
	}

	start, err := time.Parse(time.RFC3339, startValue)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid start timestamp: use RFC3339")
	}

	end, err := time.Parse(time.RFC3339, endValue)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid end timestamp: use RFC3339")
	}

	if end.Before(start) {
		return nil, nil, fmt.Errorf("end must be after or equal to start")
	}

	return &start, &end, nil
}

func (h *StatsHandler) sendSuccess(w http.ResponseWriter, statusCode int, data interface{}) {
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

func (h *StatsHandler) sendError(w http.ResponseWriter, statusCode int, code, message string) {
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
