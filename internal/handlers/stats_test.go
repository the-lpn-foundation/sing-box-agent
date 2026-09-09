package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/the-lpn-foundation/sing-box-agent/internal/models"
)

type mockStatsProvider struct {
	trafficStats []models.TrafficInboundStat
	userStats    []models.TrafficUserStat
	onlineUsers  []models.OnlineUser
	trafficErr   error
	userErr      error
	onlineErr    error

	lastInbound string
	lastStart   *time.Time
	lastEnd     *time.Time
}

func (m *mockStatsProvider) GetUserTrafficStats(_ context.Context) ([]models.TrafficUserStat, error) {
	if m.userErr != nil {
		return nil, m.userErr
	}
	return m.userStats, nil
}

func (m *mockStatsProvider) GetTrafficStats(_ context.Context, inbound string, start, end *time.Time) ([]models.TrafficInboundStat, error) {
	m.lastInbound = inbound
	m.lastStart = start
	m.lastEnd = end
	if m.trafficErr != nil {
		return nil, m.trafficErr
	}
	return m.trafficStats, nil
}

func (m *mockStatsProvider) GetOnlineUsers(_ context.Context) ([]models.OnlineUser, error) {
	if m.onlineErr != nil {
		return nil, m.onlineErr
	}
	return m.onlineUsers, nil
}

func newStatsHandlerForTest(provider StatsProvider) *StatsHandler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewStatsHandler(provider, logger)
}

func TestNewStatsHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	provider := &mockStatsProvider{}

	h := NewStatsHandler(provider, logger)

	assert.NotNil(t, h)
	assert.Equal(t, provider, h.provider)
	assert.Equal(t, logger, h.logger)
}

func TestGetTrafficStats_NilProvider(t *testing.T) {
	h := newStatsHandlerForTest(nil)

	req := httptest.NewRequest(http.MethodGet, "/stats/traffic", nil)
	w := httptest.NewRecorder()

	h.GetTrafficStats(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.False(t, envelope.Success)
	assert.Equal(t, CodeStatsUnavailable, envelope.Error.Code)
	assert.Contains(t, envelope.Error.Message, "stats provider not configured")
}

func TestGetTrafficStats_Success(t *testing.T) {
	start := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	provider := &mockStatsProvider{
		trafficStats: []models.TrafficInboundStat{{Tag: "vless-reality", Type: "vless", UpBytes: 10, DownBytes: 20}},
	}
	h := newStatsHandlerForTest(provider)

	req := httptest.NewRequest(http.MethodGet, "/stats/traffic?inbound=vless-reality&start="+start.Format(time.RFC3339)+"&end="+end.Format(time.RFC3339), nil)
	w := httptest.NewRecorder()

	h.GetTrafficStats(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.True(t, envelope.Success)

	data, ok := envelope.Data.(map[string]interface{})
	require.True(t, ok)
	inbounds, ok := data["inbounds"].([]interface{})
	require.True(t, ok)
	assert.Len(t, inbounds, 1)
	assert.Equal(t, "vless-reality", provider.lastInbound)
	require.NotNil(t, provider.lastStart)
	require.NotNil(t, provider.lastEnd)
	assert.True(t, provider.lastStart.Equal(start))
	assert.True(t, provider.lastEnd.Equal(end))
}

func TestGetTrafficStats_NoTimeRange(t *testing.T) {
	provider := &mockStatsProvider{
		trafficStats: []models.TrafficInboundStat{{Tag: "all", UpBytes: 100, DownBytes: 200}},
		userStats:    []models.TrafficUserStat{{SubID: "sub-1", UpBytes: 10, DownBytes: 20}},
	}
	h := newStatsHandlerForTest(provider)

	req := httptest.NewRequest(http.MethodGet, "/stats/traffic", nil)
	w := httptest.NewRecorder()

	h.GetTrafficStats(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.True(t, envelope.Success)

	data, ok := envelope.Data.(map[string]interface{})
	require.True(t, ok)
	assert.NotContains(t, data, "start")
	assert.NotContains(t, data, "end")
	assert.Nil(t, provider.lastStart)
	assert.Nil(t, provider.lastEnd)

	users, ok := data["users"].([]interface{})
	require.True(t, ok)
	require.Len(t, users, 1)
	u, ok := users[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "sub-1", u["subId"])
	assert.Equal(t, float64(10), u["up"])
	assert.Equal(t, float64(20), u["down"])
}

func TestGetTrafficStats_WithTimeRangeOmitsUsers(t *testing.T) {
	start := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	provider := &mockStatsProvider{
		trafficStats: []models.TrafficInboundStat{{Tag: "all", UpBytes: 100, DownBytes: 200}},
		userStats:    []models.TrafficUserStat{{SubID: "sub-1", UpBytes: 10, DownBytes: 20}},
	}
	h := newStatsHandlerForTest(provider)

	req := httptest.NewRequest(http.MethodGet, "/stats/traffic?start="+start.Format(time.RFC3339)+"&end="+end.Format(time.RFC3339), nil)
	w := httptest.NewRecorder()

	h.GetTrafficStats(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)

	data, ok := envelope.Data.(map[string]interface{})
	require.True(t, ok)
	assert.NotContains(t, data, "users")
}

func TestGetTrafficStats_InvalidTimeRange(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "invalid start format",
			url:        "/stats/traffic?start=bad-time&end=2026-02-20T12:00:00Z",
			wantStatus: http.StatusBadRequest,
			wantCode:   CodeInvalidRequest,
		},
		{
			name:       "invalid end format",
			url:        "/stats/traffic?start=2026-02-20T10:00:00Z&end=bad-time",
			wantStatus: http.StatusBadRequest,
			wantCode:   CodeInvalidRequest,
		},
		{
			name:       "only start provided",
			url:        "/stats/traffic?start=2026-02-20T10:00:00Z",
			wantStatus: http.StatusBadRequest,
			wantCode:   CodeInvalidRequest,
		},
		{
			name:       "only end provided",
			url:        "/stats/traffic?end=2026-02-20T12:00:00Z",
			wantStatus: http.StatusBadRequest,
			wantCode:   CodeInvalidRequest,
		},
		{
			name:       "end before start",
			url:        "/stats/traffic?start=2026-02-20T12:00:00Z&end=2026-02-20T10:00:00Z",
			wantStatus: http.StatusBadRequest,
			wantCode:   CodeInvalidRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newStatsHandlerForTest(&mockStatsProvider{})
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			w := httptest.NewRecorder()

			h.GetTrafficStats(w, req)

			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()
			assert.Equal(t, tt.wantStatus, resp.StatusCode)

			var envelope Response
			err := json.NewDecoder(resp.Body).Decode(&envelope)
			require.NoError(t, err)
			assert.False(t, envelope.Success)
			assert.Equal(t, tt.wantCode, envelope.Error.Code)
		})
	}
}

func TestGetTrafficStats_ProviderError(t *testing.T) {
	h := newStatsHandlerForTest(&mockStatsProvider{trafficErr: errors.New("backend down")})
	req := httptest.NewRequest(http.MethodGet, "/stats/traffic", nil)
	w := httptest.NewRecorder()

	h.GetTrafficStats(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.False(t, envelope.Success)
	assert.Equal(t, CodeStatsUnavailable, envelope.Error.Code)
	assert.Contains(t, envelope.Error.Message, "failed to fetch traffic stats")
}

func TestGetOnlineUsers_NilProvider(t *testing.T) {
	h := newStatsHandlerForTest(nil)

	req := httptest.NewRequest(http.MethodGet, "/stats/online", nil)
	w := httptest.NewRecorder()

	h.GetOnlineUsers(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.False(t, envelope.Success)
	assert.Equal(t, CodeStatsUnavailable, envelope.Error.Code)
	assert.Contains(t, envelope.Error.Message, "stats provider not configured")
}

func TestGetOnlineUsers_Success(t *testing.T) {
	connectedAt := time.Date(2026, 2, 20, 11, 0, 0, 0, time.UTC)
	provider := &mockStatsProvider{
		onlineUsers: []models.OnlineUser{{SubID: "sub-1", Inbound: "vless-reality", ConnectedAt: connectedAt}},
	}
	h := newStatsHandlerForTest(provider)

	req := httptest.NewRequest(http.MethodGet, "/stats/online", nil)
	w := httptest.NewRecorder()

	h.GetOnlineUsers(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.True(t, envelope.Success)

	data, ok := envelope.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(1), data["count"])
	users, ok := data["users"].([]interface{})
	require.True(t, ok)
	assert.Len(t, users, 1)
}

func TestGetOnlineUsers_Empty(t *testing.T) {
	provider := &mockStatsProvider{onlineUsers: []models.OnlineUser{}}
	h := newStatsHandlerForTest(provider)

	req := httptest.NewRequest(http.MethodGet, "/stats/online", nil)
	w := httptest.NewRecorder()

	h.GetOnlineUsers(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.True(t, envelope.Success)

	data, ok := envelope.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(0), data["count"])
	users, ok := data["users"].([]interface{})
	require.True(t, ok)
	assert.Len(t, users, 0)
}

func TestGetOnlineUsers_ProviderError(t *testing.T) {
	h := newStatsHandlerForTest(&mockStatsProvider{onlineErr: errors.New("backend down")})
	req := httptest.NewRequest(http.MethodGet, "/stats/online", nil)
	w := httptest.NewRecorder()

	h.GetOnlineUsers(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.False(t, envelope.Success)
	assert.Equal(t, CodeStatsUnavailable, envelope.Error.Code)
	assert.Contains(t, envelope.Error.Message, "failed to fetch online users")
}

func TestParseTimeRange_NoParams(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/stats/traffic", nil)

	start, end, err := parseTimeRange(req)

	assert.NoError(t, err)
	assert.Nil(t, start)
	assert.Nil(t, end)
}

func TestParseTimeRange_ValidRange(t *testing.T) {
	start := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)
	req := httptest.NewRequest(http.MethodGet, "/stats/traffic?start="+start.Format(time.RFC3339)+"&end="+end.Format(time.RFC3339), nil)

	parsedStart, parsedEnd, err := parseTimeRange(req)

	assert.NoError(t, err)
	require.NotNil(t, parsedStart)
	require.NotNil(t, parsedEnd)
	assert.True(t, parsedStart.Equal(start))
	assert.True(t, parsedEnd.Equal(end))
}

func TestParseTimeRange_EqualTimes(t *testing.T) {
	tm := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	req := httptest.NewRequest(http.MethodGet, "/stats/traffic?start="+tm.Format(time.RFC3339)+"&end="+tm.Format(time.RFC3339), nil)

	start, end, err := parseTimeRange(req)

	assert.NoError(t, err)
	require.NotNil(t, start)
	require.NotNil(t, end)
	assert.True(t, start.Equal(tm))
	assert.True(t, end.Equal(tm))
}

func TestParseTimeRange_OnlyStart(t *testing.T) {
	startTime := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	req := httptest.NewRequest(http.MethodGet, "/stats/traffic?start="+startTime.Format(time.RFC3339), nil)

	start, end, err := parseTimeRange(req)

	assert.Error(t, err)
	assert.Nil(t, start)
	assert.Nil(t, end)
	assert.Contains(t, err.Error(), "both start and end are required")
}

func TestParseTimeRange_OnlyEnd(t *testing.T) {
	endTime := time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)
	req := httptest.NewRequest(http.MethodGet, "/stats/traffic?end="+endTime.Format(time.RFC3339), nil)

	start, end, err := parseTimeRange(req)

	assert.Error(t, err)
	assert.Nil(t, start)
	assert.Nil(t, end)
	assert.Contains(t, err.Error(), "both start and end are required")
}

func TestParseTimeRange_InvalidStart(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/stats/traffic?start=invalid&end=2026-02-20T12:00:00Z", nil)

	start, end, err := parseTimeRange(req)

	assert.Error(t, err)
	assert.Nil(t, start)
	assert.Nil(t, end)
	assert.Contains(t, err.Error(), "invalid start timestamp")
}

func TestParseTimeRange_InvalidEnd(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/stats/traffic?start=2026-02-20T10:00:00Z&end=invalid", nil)

	start, end, err := parseTimeRange(req)

	assert.Error(t, err)
	assert.Nil(t, start)
	assert.Nil(t, end)
	assert.Contains(t, err.Error(), "invalid end timestamp")
}

func TestParseTimeRange_EndBeforeStart(t *testing.T) {
	start := time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)
	end := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	req := httptest.NewRequest(http.MethodGet, "/stats/traffic?start="+start.Format(time.RFC3339)+"&end="+end.Format(time.RFC3339), nil)

	parsedStart, parsedEnd, err := parseTimeRange(req)

	assert.Error(t, err)
	assert.Nil(t, parsedStart)
	assert.Nil(t, parsedEnd)
	assert.Contains(t, err.Error(), "end must be after or equal to start")
}

func TestStatsHandlerSendSuccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := &StatsHandler{logger: logger}

	w := httptest.NewRecorder()
	data := map[string]interface{}{"key": "value"}

	h.sendSuccess(w, http.StatusOK, data)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.True(t, envelope.Success)
	assert.Equal(t, data, envelope.Data)
}

func TestStatsHandlerSendError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := &StatsHandler{logger: logger}

	w := httptest.NewRecorder()

	h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, "test error")

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.False(t, envelope.Success)
	require.NotNil(t, envelope.Error)
	assert.Equal(t, CodeInvalidRequest, envelope.Error.Code)
	assert.Equal(t, "test error", envelope.Error.Message)
}

func TestStatsHandlerSendSuccess_WithNilLogger(t *testing.T) {
	h := &StatsHandler{logger: nil}

	w := httptest.NewRecorder()
	data := map[string]interface{}{"key": "value"}

	h.sendSuccess(w, http.StatusOK, data)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.True(t, envelope.Success)
	assert.Equal(t, data, envelope.Data)
}

func TestStatsHandlerSendError_WithNilLogger(t *testing.T) {
	h := &StatsHandler{logger: nil}

	w := httptest.NewRecorder()

	h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, "test error")

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.False(t, envelope.Success)
	require.NotNil(t, envelope.Error)
	assert.Equal(t, CodeInvalidRequest, envelope.Error.Code)
	assert.Equal(t, "test error", envelope.Error.Message)
}
