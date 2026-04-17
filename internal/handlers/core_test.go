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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockCoreService struct {
	reloadCalled  bool
	restartCalled bool
	reloadErr     error
	restartErr    error
	configErr     error
	config        map[string]interface{}
}

func (m *mockCoreService) Reload(_ context.Context) error {
	m.reloadCalled = true
	return m.reloadErr
}

func (m *mockCoreService) Restart(_ context.Context) error {
	m.restartCalled = true
	return m.restartErr
}

func (m *mockCoreService) GetConfig(_ context.Context) (map[string]interface{}, error) {
	if m.configErr != nil {
		return nil, m.configErr
	}
	return m.config, nil
}

func newCoreHandlerForTest(svc CoreService) *CoreHandler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewCoreHandler(svc, logger)
}

func TestNewCoreHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := &mockCoreService{}

	h := NewCoreHandler(svc, logger)

	assert.NotNil(t, h)
	assert.Equal(t, svc, h.service)
	assert.Equal(t, logger, h.logger)
}

func TestReloadConfig_NilService(t *testing.T) {
	h := newCoreHandlerForTest(nil)

	req := httptest.NewRequest(http.MethodPost, "/core/reload", nil)
	w := httptest.NewRecorder()

	h.ReloadConfig(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.False(t, envelope.Success)
	assert.Equal(t, CodeStatsUnavailable, envelope.Error.Code)
	assert.Contains(t, envelope.Error.Message, "core service not configured")
}

func TestReloadConfig_Success(t *testing.T) {
	mockSvc := &mockCoreService{}
	h := newCoreHandlerForTest(mockSvc)

	req := httptest.NewRequest(http.MethodPost, "/core/reload", nil)
	w := httptest.NewRecorder()
	h.ReloadConfig(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t, mockSvc.reloadCalled)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.True(t, envelope.Success)

	data, ok := envelope.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "reloaded", data["status"])
}

func TestReloadConfig_Error(t *testing.T) {
	h := newCoreHandlerForTest(&mockCoreService{reloadErr: errors.New("reload failed")})

	req := httptest.NewRequest(http.MethodPost, "/core/reload", nil)
	w := httptest.NewRecorder()
	h.ReloadConfig(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.False(t, envelope.Success)
	assert.Equal(t, CodeInternalError, envelope.Error.Code)
	assert.Contains(t, envelope.Error.Message, "failed to reload core")
}

func TestRestartCore_NilService(t *testing.T) {
	h := newCoreHandlerForTest(nil)

	req := httptest.NewRequest(http.MethodPost, "/core/restart", nil)
	w := httptest.NewRecorder()

	h.RestartCore(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.False(t, envelope.Success)
	assert.Equal(t, CodeStatsUnavailable, envelope.Error.Code)
	assert.Contains(t, envelope.Error.Message, "core service not configured")
}

func TestRestartCore_Success(t *testing.T) {
	mockSvc := &mockCoreService{}
	h := newCoreHandlerForTest(mockSvc)

	req := httptest.NewRequest(http.MethodPost, "/core/restart", nil)
	w := httptest.NewRecorder()
	h.RestartCore(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t, mockSvc.restartCalled)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.True(t, envelope.Success)

	data, ok := envelope.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "restarted", data["status"])
}

func TestRestartCore_Error(t *testing.T) {
	h := newCoreHandlerForTest(&mockCoreService{restartErr: errors.New("restart failed")})

	req := httptest.NewRequest(http.MethodPost, "/core/restart", nil)
	w := httptest.NewRecorder()
	h.RestartCore(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.False(t, envelope.Success)
	assert.Equal(t, CodeInternalError, envelope.Error.Code)
	assert.Contains(t, envelope.Error.Message, "failed to restart core")
}

func TestGetConfig_NilService(t *testing.T) {
	h := newCoreHandlerForTest(nil)

	req := httptest.NewRequest(http.MethodGet, "/core/config", nil)
	w := httptest.NewRecorder()

	h.GetConfig(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.False(t, envelope.Success)
	assert.Equal(t, CodeStatsUnavailable, envelope.Error.Code)
	assert.Contains(t, envelope.Error.Message, "core service not configured")
}

func TestGetConfig_RedactsSensitiveFields(t *testing.T) {
	h := newCoreHandlerForTest(&mockCoreService{config: map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{
				"type":        "vless",
				"private_key": "secret-key",
				"short_id":    "abcd",
				"users": []interface{}{
					map[string]interface{}{"name": "user-a", "password": "user-pass"},
				},
			},
		},
		"password": "top-secret",
	}})

	req := httptest.NewRequest(http.MethodGet, "/core/config", nil)
	w := httptest.NewRecorder()
	h.GetConfig(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.True(t, envelope.Success)

	data, ok := envelope.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, redactedValue, data["password"])

	inbounds, ok := data["inbounds"].([]interface{})
	require.True(t, ok)
	first, ok := inbounds[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, redactedValue, first["private_key"])
	assert.Equal(t, redactedValue, first["short_id"])

	users, ok := first["users"].([]interface{})
	require.True(t, ok)
	firstUser, ok := users[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, redactedValue, firstUser["password"])
}

func TestGetConfig_EmptyConfig(t *testing.T) {
	h := newCoreHandlerForTest(&mockCoreService{config: map[string]interface{}{}})

	req := httptest.NewRequest(http.MethodGet, "/core/config", nil)
	w := httptest.NewRecorder()
	h.GetConfig(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.True(t, envelope.Success)

	data, ok := envelope.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Empty(t, data)
}

func TestGetConfig_NestedRedaction(t *testing.T) {
	h := newCoreHandlerForTest(&mockCoreService{config: map[string]interface{}{
		"level1": map[string]interface{}{
			"level2": map[string]interface{}{
				"password": "deep-secret",
			},
		},
	}})

	req := httptest.NewRequest(http.MethodGet, "/core/config", nil)
	w := httptest.NewRecorder()
	h.GetConfig(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.True(t, envelope.Success)

	data, ok := envelope.Data.(map[string]interface{})
	require.True(t, ok)
	level1, ok := data["level1"].(map[string]interface{})
	require.True(t, ok)
	level2, ok := level1["level2"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, redactedValue, level2["password"])
}

func TestGetConfig_ArrayRedaction(t *testing.T) {
	h := newCoreHandlerForTest(&mockCoreService{config: map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"name": "item1", "password": "pass1"},
			map[string]interface{}{"name": "item2", "password": "pass2"},
		},
	}})

	req := httptest.NewRequest(http.MethodGet, "/core/config", nil)
	w := httptest.NewRecorder()
	h.GetConfig(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.True(t, envelope.Success)

	data, ok := envelope.Data.(map[string]interface{})
	require.True(t, ok)
	items, ok := data["items"].([]interface{})
	require.True(t, ok)
	assert.Len(t, items, 2)

	item1, ok := items[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, redactedValue, item1["password"])

	item2, ok := items[1].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, redactedValue, item2["password"])
}

func TestGetConfig_Error(t *testing.T) {
	h := newCoreHandlerForTest(&mockCoreService{configErr: errors.New("unavailable")})

	req := httptest.NewRequest(http.MethodGet, "/core/config", nil)
	w := httptest.NewRecorder()
	h.GetConfig(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.False(t, envelope.Success)
	assert.Equal(t, CodeInternalError, envelope.Error.Code)
	assert.Contains(t, envelope.Error.Message, "failed to get core config")
}

func TestRedactConfig_Nil(t *testing.T) {
	result := redactConfig(nil)

	assert.Nil(t, result)
}

func TestRedactConfig_Empty(t *testing.T) {
	result := redactConfig(map[string]interface{}{})

	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestRedactMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "redacts private_key",
			input: map[string]interface{}{
				"private_key": "secret",
				"other":       "value",
			},
			expected: map[string]interface{}{
				"private_key": redactedValue,
				"other":       "value",
			},
		},
		{
			name: "redacts password",
			input: map[string]interface{}{
				"password": "secret",
				"other":    "value",
			},
			expected: map[string]interface{}{
				"password": redactedValue,
				"other":    "value",
			},
		},
		{
			name: "redacts short_id",
			input: map[string]interface{}{
				"short_id": "abcd",
				"other":    "value",
			},
			expected: map[string]interface{}{
				"short_id": redactedValue,
				"other":    "value",
			},
		},
		{
			name: "redacts all sensitive fields",
			input: map[string]interface{}{
				"private_key": "pk",
				"password":    "pw",
				"short_id":    "si",
				"normal":      "val",
			},
			expected: map[string]interface{}{
				"private_key": redactedValue,
				"password":    redactedValue,
				"short_id":    redactedValue,
				"normal":      "val",
			},
		},
		{
			name:     "empty map",
			input:    map[string]interface{}{},
			expected: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := redactMap(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRedactSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    []interface{}
		expected []interface{}
	}{
		{
			name: "redacts nested maps",
			input: []interface{}{
				map[string]interface{}{"password": "secret"},
				map[string]interface{}{"name": "value"},
			},
			expected: []interface{}{
				map[string]interface{}{"password": redactedValue},
				map[string]interface{}{"name": "value"},
			},
		},
		{
			name: "preserves non-map values",
			input: []interface{}{
				"string",
				123,
				true,
			},
			expected: []interface{}{
				"string",
				123,
				true,
			},
		},
		{
			name:     "empty slice",
			input:    []interface{}{},
			expected: []interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := redactSlice(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRedactValue(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{
			name:     "map",
			input:    map[string]interface{}{"password": "secret"},
			expected: map[string]interface{}{"password": redactedValue},
		},
		{
			name: "slice",
			input: []interface{}{
				map[string]interface{}{"password": "secret"},
			},
			expected: []interface{}{
				map[string]interface{}{"password": redactedValue},
			},
		},
		{
			name:     "string",
			input:    "value",
			expected: "value",
		},
		{
			name:     "int",
			input:    123,
			expected: 123,
		},
		{
			name:     "bool",
			input:    true,
			expected: true,
		},
		{
			name:     "nil",
			input:    nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := redactValue(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCoreHandlerSendSuccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := &CoreHandler{logger: logger}

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

func TestCoreHandlerSendError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := &CoreHandler{logger: logger}

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

func TestCoreHandlerSendSuccess_WithNilLogger(t *testing.T) {
	h := &CoreHandler{logger: nil}

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

func TestCoreHandlerSendError_WithNilLogger(t *testing.T) {
	h := &CoreHandler{logger: nil}

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
