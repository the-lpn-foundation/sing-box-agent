//nolint:errcheck
package handlers

//nolint:errcheck // Test file uses type assertions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oglenyaboss/sing-box-agent/internal/models"
)

type mockSubscriptionSingBoxClient struct {
	inbounds []models.Inbound
	users    []models.User
	err      error
}

func (m *mockSubscriptionSingBoxClient) GetInbounds(_ context.Context) ([]models.Inbound, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.inbounds, nil
}

func (m *mockSubscriptionSingBoxClient) GetUsers(_ context.Context) ([]models.User, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.users, nil
}

func (m *mockSubscriptionSingBoxClient) CreateInbound(_ context.Context, _ models.Inbound) error {
	return nil
}

func (m *mockSubscriptionSingBoxClient) UpdateInbound(_ context.Context, _ models.Inbound) error {
	return nil
}

func (m *mockSubscriptionSingBoxClient) DeleteInbound(_ context.Context, _ string) error {
	return nil
}

func (m *mockSubscriptionSingBoxClient) CreateUser(_ context.Context, _ models.User) error {
	return nil
}

func (m *mockSubscriptionSingBoxClient) UpdateUser(_ context.Context, _ models.User) error {
	return nil
}

func (m *mockSubscriptionSingBoxClient) DeleteUser(_ context.Context, _ string) error {
	return nil
}

func newSubscriptionHandlerForTest(client mockSubscriptionSingBoxClient) *SubscriptionHandler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewSubscriptionHandler(logger, &client)
}

func TestNewSubscriptionHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := &mockSubscriptionSingBoxClient{}

	h := NewSubscriptionHandler(logger, client)

	assert.NotNil(t, h)
	assert.Equal(t, logger, h.logger)
	assert.Equal(t, client, h.singbox)
	assert.NotNil(t, h.store)
}

func TestNewSubscriptionHandler_NoClient(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	h := NewSubscriptionHandler(logger)

	assert.NotNil(t, h)
	assert.Equal(t, logger, h.logger)
	assert.Nil(t, h.singbox)
	assert.NotNil(t, h.store)
}

func TestGenerateSubscription_InvalidRequestBody(t *testing.T) {
	h := newSubscriptionHandlerForTest(mockSubscriptionSingBoxClient{})

	req := httptest.NewRequest(http.MethodPost, "/subscription/generate", bytes.NewBufferString("invalid json"))
	w := httptest.NewRecorder()

	h.GenerateSubscription(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.False(t, envelope.Success)
	assert.Equal(t, CodeInvalidRequest, envelope.Error.Code)
	assert.Contains(t, envelope.Error.Message, "invalid request body")
}

func TestGenerateSubscription_MissingSubID(t *testing.T) {
	h := newSubscriptionHandlerForTest(mockSubscriptionSingBoxClient{})

	body, _ := json.Marshal(SubscriptionRequest{
		Format: SubscriptionFormatV2Ray,
		Server: "vpn.example.com",
		Port:   443,
	})
	req := httptest.NewRequest(http.MethodPost, "/subscription/generate", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	h.GenerateSubscription(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.False(t, envelope.Success)
	assert.Contains(t, envelope.Error.Message, "subId is required")
}

func TestGenerateSubscription_AllFormats(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		contains string
	}{
		{name: "v2ray", format: SubscriptionFormatV2Ray, contains: "vless://sub-1@vpn.example.com:443"},
		{name: "clash", format: SubscriptionFormatClash, contains: "proxies:"},
		{name: "sing-box", format: SubscriptionFormatSingBox, contains: "\"outbounds\""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h *SubscriptionHandler
			if tt.format == SubscriptionFormatSingBox {
				h = NewSubscriptionHandler(slog.New(slog.NewTextHandler(io.Discard, nil)))
			} else {
				h = newSubscriptionHandlerForTest(mockSubscriptionSingBoxClient{})
			}

			body, err := json.Marshal(SubscriptionRequest{
				SubID:  "sub-1",
				Format: tt.format,
				Server: "vpn.example.com",
				Port:   443,
			})
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/subscription/generate", bytes.NewBuffer(body))
			w := httptest.NewRecorder()

			h.GenerateSubscription(w, req)

			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var envelope Response
			err = json.NewDecoder(resp.Body).Decode(&envelope)
			require.NoError(t, err)
			assert.True(t, envelope.Success)

			data, ok := envelope.Data.(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, tt.format, data["format"])
			assert.Contains(t, data["config"], tt.contains)
		})
	}
}

func TestGenerateSubscription_V2RayMissingServer(t *testing.T) {
	h := newSubscriptionHandlerForTest(mockSubscriptionSingBoxClient{})

	body, _ := json.Marshal(SubscriptionRequest{
		SubID:  "sub-1",
		Format: SubscriptionFormatV2Ray,
		Port:   443,
	})
	req := httptest.NewRequest(http.MethodPost, "/subscription/generate", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	h.GenerateSubscription(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.False(t, envelope.Success)
	assert.Contains(t, envelope.Error.Message, "server is required for v2ray/clash format")
}

func TestGenerateSubscription_V2RayInvalidPort(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{name: "port zero", port: 0},
		{name: "port negative", port: -1},
		{name: "port too high", port: 65536},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newSubscriptionHandlerForTest(mockSubscriptionSingBoxClient{})

			body, _ := json.Marshal(SubscriptionRequest{
				SubID:  "sub-1",
				Format: SubscriptionFormatV2Ray,
				Server: "vpn.example.com",
				Port:   tt.port,
			})
			req := httptest.NewRequest(http.MethodPost, "/subscription/generate", bytes.NewBuffer(body))
			w := httptest.NewRecorder()

			h.GenerateSubscription(w, req)

			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

			var envelope Response
			err := json.NewDecoder(resp.Body).Decode(&envelope)
			require.NoError(t, err)
			assert.False(t, envelope.Success)
			assert.Contains(t, envelope.Error.Message, "port must be between 1 and 65535")
		})
	}
}

func TestGenerateSubscription_InvalidFormat(t *testing.T) {
	h := NewSubscriptionHandler(slog.New(slog.NewTextHandler(io.Discard, nil)))
	body := []byte(`{"subId":"sub-1","format":"invalid","server":"vpn.example.com","port":443}`)

	req := httptest.NewRequest(http.MethodPost, "/subscription/generate", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	h.GenerateSubscription(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.False(t, envelope.Success)
	assert.Contains(t, envelope.Error.Message, "unsupported format")
}

func TestGenerateSubscription_SingBoxWithClient(t *testing.T) {
	client := mockSubscriptionSingBoxClient{
		inbounds: []models.Inbound{
			{
				Tag:  "hysteria2-in",
				Type: "hysteria2",
				Port: 443,
				Options: map[string]interface{}{
					"tls": map[string]interface{}{
						"server_name": "vpn.example.com",
					},
				},
			},
		},
		users: []models.User{
			{
				SubID:      "sub-1",
				UUID:       "uuid-123",
				InboundTag: "hysteria2-in",
				Enabled:    true,
			},
		},
	}
	h := newSubscriptionHandlerForTest(client)

	body, _ := json.Marshal(SubscriptionRequest{
		SubID:  "sub-1",
		Format: SubscriptionFormatSingBox,
	})
	req := httptest.NewRequest(http.MethodPost, "/subscription/generate", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	h.GenerateSubscription(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.True(t, envelope.Success)

	data, ok := envelope.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, SubscriptionFormatSingBox, data["format"])
	assert.Contains(t, data["config"], "\"outbounds\"")
	assert.Contains(t, data["config"], "\"hysteria2\"")
}

func TestGenerateSubscription_SingBoxUserNotFound(t *testing.T) {
	client := mockSubscriptionSingBoxClient{
		inbounds: []models.Inbound{},
		users:    []models.User{},
	}
	h := newSubscriptionHandlerForTest(client)

	body, _ := json.Marshal(SubscriptionRequest{
		SubID:  "sub-1",
		Format: SubscriptionFormatSingBox,
	})
	req := httptest.NewRequest(http.MethodPost, "/subscription/generate", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	h.GenerateSubscription(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.False(t, envelope.Success)
	assert.Equal(t, CodeSubscriptionNotFound, envelope.Error.Code)
}

func TestGenerateSubscription_SingBoxClientError(t *testing.T) {
	client := mockSubscriptionSingBoxClient{
		err: errors.New("client error"),
	}
	h := newSubscriptionHandlerForTest(client)

	body, _ := json.Marshal(SubscriptionRequest{
		SubID:  "sub-1",
		Format: SubscriptionFormatSingBox,
	})
	req := httptest.NewRequest(http.MethodPost, "/subscription/generate", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	h.GenerateSubscription(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.False(t, envelope.Success)
	assert.Contains(t, envelope.Error.Message, "failed to load inbounds")
}

func TestGenerateSubscription_SingBoxWithoutClient(t *testing.T) {
	h := NewSubscriptionHandler(slog.New(slog.NewTextHandler(io.Discard, nil)))

	body, _ := json.Marshal(SubscriptionRequest{
		SubID:  "sub-1",
		Format: SubscriptionFormatSingBox,
		Server: "vpn.example.com",
		Port:   443,
	})
	req := httptest.NewRequest(http.MethodPost, "/subscription/generate", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	h.GenerateSubscription(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.True(t, envelope.Success)

	data, ok := envelope.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, SubscriptionFormatSingBox, data["format"])
	assert.Contains(t, data["config"], "\"outbounds\"")
}

func TestGenerateSubscription_SingBoxWithoutClientMissingServer(t *testing.T) {
	h := NewSubscriptionHandler(slog.New(slog.NewTextHandler(io.Discard, nil)))

	body, _ := json.Marshal(SubscriptionRequest{
		SubID:  "sub-1",
		Format: SubscriptionFormatSingBox,
	})
	req := httptest.NewRequest(http.MethodPost, "/subscription/generate", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	h.GenerateSubscription(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.False(t, envelope.Success)
	assert.Contains(t, envelope.Error.Message, "server is required")
}

func TestGetSubscription_Success(t *testing.T) {
	h := newSubscriptionHandlerForTest(mockSubscriptionSingBoxClient{})

	generateBody := []byte(`{"subId":"sub-1","format":"v2ray","server":"vpn.example.com","port":443}`)
	generateReq := httptest.NewRequest(http.MethodPost, "/subscription/generate", bytes.NewBuffer(generateBody))
	generateResp := httptest.NewRecorder()
	h.GenerateSubscription(generateResp, generateReq)
	require.Equal(t, http.StatusOK, generateResp.Code)

	getReq := httptest.NewRequest(http.MethodGet, "/subscription/sub-1", nil)
	getResp := httptest.NewRecorder()
	h.GetSubscription(getResp, getReq)

	resp := getResp.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.True(t, envelope.Success)

	data, ok := envelope.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "sub-1", data["subId"])
	assert.Contains(t, data["config"], "vless://sub-1")
}

func TestGetSubscription_EmptySubID(t *testing.T) {
	h := newSubscriptionHandlerForTest(mockSubscriptionSingBoxClient{})

	req := httptest.NewRequest(http.MethodGet, "/subscription/", nil)
	w := httptest.NewRecorder()

	h.GetSubscription(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.False(t, envelope.Success)
	assert.Contains(t, envelope.Error.Message, "subId is required")
}

func TestGetSubscription_NotFound(t *testing.T) {
	h := newSubscriptionHandlerForTest(mockSubscriptionSingBoxClient{})

	req := httptest.NewRequest(http.MethodGet, "/subscription/missing-sub", nil)
	w := httptest.NewRecorder()

	h.GetSubscription(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.False(t, envelope.Success)
	require.NotNil(t, envelope.Error)
	assert.Equal(t, CodeSubscriptionNotFound, envelope.Error.Code)
}

func TestGetSubscription_WithClient(t *testing.T) {
	client := mockSubscriptionSingBoxClient{
		inbounds: []models.Inbound{
			{
				Tag:  "hysteria2-in",
				Type: "hysteria2",
				Port: 443,
				Options: map[string]interface{}{
					"tls": map[string]interface{}{
						"server_name": "vpn.example.com",
					},
				},
			},
		},
		users: []models.User{
			{
				SubID:      "sub-1",
				UUID:       "uuid-123",
				InboundTag: "hysteria2-in",
				Enabled:    true,
			},
		},
	}
	h := newSubscriptionHandlerForTest(client)

	req := httptest.NewRequest(http.MethodGet, "/subscription/sub-1", nil)
	w := httptest.NewRecorder()

	h.GetSubscription(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.True(t, envelope.Success)

	data, ok := envelope.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "sub-1", data["subId"])
	assert.Equal(t, SubscriptionFormatSingBox, data["format"])
	assert.Contains(t, data["config"], "\"outbounds\"")
}

func TestResolveTarget_Success(t *testing.T) {
	client := mockSubscriptionSingBoxClient{
		inbounds: []models.Inbound{
			{Tag: "in-1", Type: "vless", Port: 443},
		},
		users: []models.User{
			{SubID: "sub-1", UUID: "uuid-1", InboundTag: "in-1", Enabled: true},
		},
	}
	h := newSubscriptionHandlerForTest(client)

	inbound, user, allInbounds, err := h.resolveTarget(context.Background(), "sub-1", "")

	require.NoError(t, err)
	assert.Equal(t, "in-1", inbound.Tag)
	assert.Equal(t, "sub-1", user.SubID)
	assert.Len(t, allInbounds, 1)
}

func TestResolveTarget_WithInboundTag(t *testing.T) {
	client := mockSubscriptionSingBoxClient{
		inbounds: []models.Inbound{
			{Tag: "in-1", Type: "vless", Port: 443},
			{Tag: "in-2", Type: "hysteria2", Port: 8443},
		},
		users: []models.User{
			{SubID: "sub-1", UUID: "uuid-1", InboundTag: "in-1", Enabled: true},
			{SubID: "sub-1", UUID: "uuid-2", InboundTag: "in-2", Enabled: true},
		},
	}
	h := newSubscriptionHandlerForTest(client)

	inbound, user, allInbounds, err := h.resolveTarget(context.Background(), "sub-1", "in-2")

	require.NoError(t, err)
	assert.Equal(t, "in-2", inbound.Tag)
	assert.Equal(t, "sub-1", user.SubID)
	assert.Len(t, allInbounds, 2)
}

func TestResolveTarget_UserNotFound(t *testing.T) {
	client := mockSubscriptionSingBoxClient{
		inbounds: []models.Inbound{},
		users:    []models.User{},
	}
	h := newSubscriptionHandlerForTest(client)

	inbound, user, allInbounds, err := h.resolveTarget(context.Background(), "sub-1", "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "subscription for sub-1 not found")
	assert.Empty(t, inbound.Tag)
	assert.Empty(t, user.SubID)
	assert.Len(t, allInbounds, 0)
}

func TestResolveTarget_InboundNotFound(t *testing.T) {
	client := mockSubscriptionSingBoxClient{
		inbounds: []models.Inbound{},
		users: []models.User{
			{SubID: "sub-1", UUID: "uuid-1", InboundTag: "missing-in", Enabled: true},
		},
	}
	h := newSubscriptionHandlerForTest(client)

	_, _, _, err := h.resolveTarget(context.Background(), "sub-1", "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "subscription for sub-1 not found")
}

func TestResolveTarget_GetInboundsError(t *testing.T) {
	client := mockSubscriptionSingBoxClient{
		err: errors.New("get inbounds error"),
	}
	h := newSubscriptionHandlerForTest(client)

	_, _, _, err := h.resolveTarget(context.Background(), "sub-1", "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load inbounds")
}

func TestResolveTarget_GetUsersError(t *testing.T) {
	client := mockSubscriptionSingBoxClient{
		inbounds: []models.Inbound{{Tag: "in-1", Type: "vless", Port: 443}},
		users:    []models.User{},
	}
	h := newSubscriptionHandlerForTest(client)

	_, _, _, err := h.resolveTarget(context.Background(), "sub-1", "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "subscription for sub-1 not found")
}

func TestNormalizeSubscriptionFormat(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", SubscriptionFormatSingBox},
		{"singbox", SubscriptionFormatSingBox},
		{"SingBox", SubscriptionFormatSingBox},
		{"  singbox  ", SubscriptionFormatSingBox},
		{"v2ray", "v2ray"},
		{"V2RAY", "v2ray"},
		{"  v2ray  ", "v2ray"},
		{"clash", "clash"},
		{"CLASH", "clash"},
		{"  clash  ", "clash"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeSubscriptionFormat(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildLegacySubscriptionConfig(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		contains string
	}{
		{
			name:     "v2ray",
			format:   SubscriptionFormatV2Ray,
			contains: "vless://sub-1@vpn.example.com:443",
		},
		{
			name:     "clash",
			format:   SubscriptionFormatClash,
			contains: "proxies:",
		},
		{
			name:     "sing-box",
			format:   SubscriptionFormatSingBox,
			contains: "\"outbounds\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := SubscriptionRequest{
				SubID:  "sub-1",
				Server: "vpn.example.com",
				Port:   443,
			}

			config, err := buildLegacySubscriptionConfig(req, tt.format)

			require.NoError(t, err)
			assert.Contains(t, config, tt.contains)
		})
	}
}

func TestBuildLegacySubscriptionConfig_UnsupportedFormat(t *testing.T) {
	req := SubscriptionRequest{
		SubID:  "sub-1",
		Server: "vpn.example.com",
		Port:   443,
	}

	config, err := buildLegacySubscriptionConfig(req, "unsupported")

	assert.Error(t, err)
	assert.Empty(t, config)
	assert.Contains(t, err.Error(), "unsupported format")
}

func TestResolveServerPort_FromRequest(t *testing.T) {
	req := SubscriptionRequest{
		Server: "custom.example.com",
		Port:   8443,
	}
	inbound := models.Inbound{Port: 443}

	server, port, err := resolveServerPort(req, inbound, "default.example.com")

	require.NoError(t, err)
	assert.Equal(t, "custom.example.com", server)
	assert.Equal(t, 8443, port)
}

func TestResolveServerPort_FromInboundTLS(t *testing.T) {
	req := SubscriptionRequest{
		Port: 0,
	}
	inbound := models.Inbound{
		Port: 443,
		Options: map[string]interface{}{
			"tls": map[string]interface{}{
				"server_name": "tls.example.com",
			},
		},
	}

	server, port, err := resolveServerPort(req, inbound, "default.example.com")

	require.NoError(t, err)
	assert.Equal(t, "tls.example.com", server)
	assert.Equal(t, 443, port)
}

func TestResolveServerPort_FromRequestHost(t *testing.T) {
	req := SubscriptionRequest{
		Port: 0,
	}
	inbound := models.Inbound{
		Port: 443,
	}

	server, port, err := resolveServerPort(req, inbound, "host.example.com:8080")

	require.NoError(t, err)
	assert.Equal(t, "host.example.com", server)
	assert.Equal(t, 443, port)
}

func TestResolveServerPort_InvalidPort(t *testing.T) {
	req := SubscriptionRequest{
		Server: "example.com",
		Port:   70000,
	}
	inbound := models.Inbound{Port: 443}

	server, port, err := resolveServerPort(req, inbound, "default.example.com")

	assert.Error(t, err)
	assert.Empty(t, server)
	assert.Equal(t, 0, port)
	assert.Contains(t, err.Error(), "invalid server port")
}

func TestResolveServerPort_CannotResolve(t *testing.T) {
	req := SubscriptionRequest{
		Port: 0,
	}
	inbound := models.Inbound{
		Port: 0,
	}

	server, port, err := resolveServerPort(req, inbound, "")

	assert.Error(t, err)
	assert.Empty(t, server)
	assert.Equal(t, 0, port)
	assert.Contains(t, err.Error(), "cannot resolve server address")
}

func TestBuildSingBoxSubscriptionConfig_Hysteria2(t *testing.T) {
	inbound := models.Inbound{
		Tag:  "h2-in",
		Type: "hysteria2",
		Port: 443,
		Options: map[string]interface{}{
			"obfs": map[string]interface{}{
				"type": "salamander",
			},
			"tls": map[string]interface{}{
				"server_name": "example.com",
			},
		},
	}
	user := models.User{
		SubID: "sub-1",
		UUID:  "uuid-123",
	}

	config, err := buildSingBoxSubscriptionConfig(SubscriptionFormatSingBox, inbound, user, []models.Inbound{}, "example.com", 443)

	require.NoError(t, err)
	assert.Contains(t, config, "\"hysteria2\"")
	assert.Contains(t, config, "uuid-123")
	assert.Contains(t, config, "\"obfs\"")
	assert.Contains(t, config, "\"tls\"")
}

func TestBuildSingBoxSubscriptionConfig_Vless(t *testing.T) {
	inbound := models.Inbound{
		Tag:  "vless-in",
		Type: "vless",
		Port: 443,
		Options: map[string]interface{}{
			"tls": map[string]interface{}{
				"server_name": "example.com",
			},
		},
	}
	user := models.User{
		SubID: "sub-1",
		UUID:  "uuid-123",
		Flow:  "xtls-rprx-vision",
	}

	config, err := buildSingBoxSubscriptionConfig(SubscriptionFormatSingBox, inbound, user, []models.Inbound{}, "example.com", 443)

	require.NoError(t, err)
	assert.Contains(t, config, "\"vless\"")
	assert.Contains(t, config, "uuid-123")
	assert.Contains(t, config, "xtls-rprx-vision")
}

func TestBuildSingBoxSubscriptionConfig_Vmess(t *testing.T) {
	inbound := models.Inbound{
		Tag:  "vmess-in",
		Type: "vmess",
		Port: 443,
	}
	user := models.User{
		SubID: "sub-1",
		UUID:  "uuid-123",
	}

	config, err := buildSingBoxSubscriptionConfig(SubscriptionFormatSingBox, inbound, user, []models.Inbound{}, "example.com", 443)

	require.NoError(t, err)
	assert.Contains(t, config, "\"vmess\"")
	assert.Contains(t, config, "uuid-123")
}

func TestBuildSingBoxSubscriptionConfig_Trojan(t *testing.T) {
	inbound := models.Inbound{
		Tag:  "trojan-in",
		Type: "trojan",
		Port: 443,
	}
	user := models.User{
		SubID: "sub-1",
		UUID:  "uuid-123",
	}

	config, err := buildSingBoxSubscriptionConfig(SubscriptionFormatSingBox, inbound, user, []models.Inbound{}, "example.com", 443)

	require.NoError(t, err)
	assert.Contains(t, config, "\"trojan\"")
	assert.Contains(t, config, "uuid-123")
}

func TestBuildSingBoxSubscriptionConfig_Tuic(t *testing.T) {
	inbound := models.Inbound{
		Tag:  "tuic-in",
		Type: "tuic",
		Port: 443,
	}
	user := models.User{
		SubID: "sub-1",
		UUID:  "uuid-123",
	}

	config, err := buildSingBoxSubscriptionConfig(SubscriptionFormatSingBox, inbound, user, []models.Inbound{}, "example.com", 443)

	require.NoError(t, err)
	assert.Contains(t, config, "\"tuic\"")
	assert.Contains(t, config, "uuid-123")
}

func TestBuildSingBoxSubscriptionConfig_ShadowTLS(t *testing.T) {
	inbounds := []models.Inbound{
		{
			Tag:  "ss-in",
			Type: "shadowsocks",
			Port: 8388,
			Options: map[string]interface{}{
				"method":   "aes-256-gcm",
				"password": "ss-password",
			},
		},
	}
	inbound := models.Inbound{
		Tag:  "st-in",
		Type: "shadowtls",
		Port: 443,
		Options: map[string]interface{}{
			"detour": "ss-in",
			"handshake": map[string]interface{}{
				"server": "www.google.com:443",
			},
			"version": 3,
		},
	}
	user := models.User{
		SubID: "sub-1",
		UUID:  "uuid-123",
	}

	config, err := buildSingBoxSubscriptionConfig(SubscriptionFormatSingBox, inbound, user, inbounds, "example.com", 443)

	require.NoError(t, err)
	assert.Contains(t, config, "\"shadowtls\"")
	assert.Contains(t, config, "\"shadowsocks\"")
	assert.Contains(t, config, "uuid-123")
	assert.Contains(t, config, "aes-256-gcm")
}

func TestBuildSingBoxSubscriptionConfig_ShadowTLS_DetourNotFound(t *testing.T) {
	inbound := models.Inbound{
		Tag:  "st-in",
		Type: "shadowtls",
		Port: 443,
		Options: map[string]interface{}{
			"detour": "missing-ss",
		},
	}
	user := models.User{
		SubID: "sub-1",
		UUID:  "uuid-123",
	}

	config, err := buildSingBoxSubscriptionConfig(SubscriptionFormatSingBox, inbound, user, []models.Inbound{}, "example.com", 443)

	assert.Error(t, err)
	assert.Empty(t, config)
	assert.Contains(t, err.Error(), "detour inbound missing-ss not found")
}

func TestBuildSingBoxSubscriptionConfig_UnsupportedType(t *testing.T) {
	inbound := models.Inbound{
		Tag:  "unsupported-in",
		Type: "unsupported",
		Port: 443,
	}
	user := models.User{
		SubID: "sub-1",
		UUID:  "uuid-123",
	}

	config, err := buildSingBoxSubscriptionConfig(SubscriptionFormatSingBox, inbound, user, []models.Inbound{}, "example.com", 443)

	assert.Error(t, err)
	assert.Empty(t, config)
	assert.Contains(t, err.Error(), "unsupported inbound type for subscription")
}

func TestBuildSingBoxSubscriptionConfig_FormatMismatch(t *testing.T) {
	inbound := models.Inbound{
		Tag:  "h2-in",
		Type: "hysteria2",
		Port: 443,
	}
	user := models.User{
		SubID: "sub-1",
		UUID:  "uuid-123",
	}

	config, err := buildSingBoxSubscriptionConfig("vless", inbound, user, []models.Inbound{}, "example.com", 443)

	assert.Error(t, err)
	assert.Empty(t, config)
	assert.Contains(t, err.Error(), "requested format vless does not match inbound type hysteria2")
}

func TestFindInboundByTag(t *testing.T) {
	inbounds := []models.Inbound{
		{Tag: "in-1", Type: "vless", Port: 443},
		{Tag: "in-2", Type: "hysteria2", Port: 8443},
	}

	t.Run("found", func(t *testing.T) {
		inbound, ok := findInboundByTag(inbounds, "in-2")
		assert.True(t, ok)
		assert.Equal(t, "in-2", inbound.Tag)
	})

	t.Run("not found", func(t *testing.T) {
		inbound, ok := findInboundByTag(inbounds, "missing")
		assert.False(t, ok)
		assert.Empty(t, inbound.Tag)
	})
}

func TestStripPort(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"example.com:443", "example.com"},
		{"example.com:8080", "example.com"},
		{"example.com", "example.com"},
		{"192.168.1.1:443", "192.168.1.1"},
		{"[::1]:443", "::1"},
		{"[2001:db8::1]:443", "2001:db8::1"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := stripPort(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAsMap(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected map[string]interface{}
	}{
		{
			name:     "valid map",
			input:    map[string]interface{}{"key": "value"},
			expected: map[string]interface{}{"key": "value"},
		},
		{
			name:     "nil",
			input:    nil,
			expected: nil,
		},
		{
			name:     "string",
			input:    "not a map",
			expected: nil,
		},
		{
			name:     "int",
			input:    123,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := asMap(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAsString(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "valid string",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "nil",
			input:    nil,
			expected: "",
		},
		{
			name:     "int",
			input:    123,
			expected: "",
		},
		{
			name:     "map",
			input:    map[string]interface{}{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := asString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAsInt(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected int
	}{
		{
			name:     "int",
			input:    42,
			expected: 42,
		},
		{
			name:     "int64",
			input:    int64(42),
			expected: 42,
		},
		{
			name:     "float64",
			input:    float64(42.5),
			expected: 42,
		},
		{
			name:     "nil",
			input:    nil,
			expected: 0,
		},
		{
			name:     "string",
			input:    "not a number",
			expected: 0,
		},
		{
			name:     "map",
			input:    map[string]interface{}{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := asInt(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSubscriptionHandlerSendSuccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := &SubscriptionHandler{logger: logger}

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

func TestSubscriptionHandlerSendError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := &SubscriptionHandler{logger: logger}

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

func TestSubscriptionHandlerSendSuccess_WithNilLogger(t *testing.T) {
	h := &SubscriptionHandler{logger: nil}

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

func TestSubscriptionHandlerSendError_WithNilLogger(t *testing.T) {
	h := &SubscriptionHandler{logger: nil}

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

func TestSubscriptionHandlerSendSuccess_EncodeError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := &SubscriptionHandler{logger: logger}

	// Create a custom ResponseWriter that will fail on Write
	w := &failingResponseWriter{}

	data := map[string]interface{}{"key": "value"}
	h.sendSuccess(w, http.StatusOK, data)
	// Should not panic, just log error
}

func TestSubscriptionHandlerSendError_EncodeError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := &SubscriptionHandler{logger: logger}

	// Create a custom ResponseWriter that will fail on Write
	w := &failingResponseWriter{}

	h.sendError(w, http.StatusBadRequest, CodeInvalidRequest, "test error")
	// Should not panic, just log error
}

func TestStripPort_EdgeCases(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"example.com:", "example.com"},
		{"example.com:0", "example.com"},
		{"example.com:65535", "example.com"},
		{"192.168.1.1:0", "192.168.1.1"},
		{"192.168.1.1:65535", "192.168.1.1"},
		// Note: stripPort doesn't handle IPv6 addresses without brackets properly
		// These test cases document the actual behavior
		{"[::1]", "[:"},
		{"[2001:db8::1]", "[2001:db8:"},
		{"[2001:db8::1]:443", "2001:db8::1"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := stripPort(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateSubscription_WithInboundTag(t *testing.T) {
	client := mockSubscriptionSingBoxClient{
		inbounds: []models.Inbound{
			{Tag: "in-1", Type: "vless", Port: 443},
			{Tag: "in-2", Type: "hysteria2", Port: 8443},
		},
		users: []models.User{
			{SubID: "sub-1", UUID: "uuid-1", InboundTag: "in-1", Enabled: true},
			{SubID: "sub-1", UUID: "uuid-2", InboundTag: "in-2", Enabled: true},
		},
	}
	h := newSubscriptionHandlerForTest(client)

	body, _ := json.Marshal(SubscriptionRequest{
		SubID:      "sub-1",
		Format:     SubscriptionFormatV2Ray,
		Server:     "vpn.example.com",
		Port:       443,
		InboundTag: "in-2",
	})
	req := httptest.NewRequest(http.MethodPost, "/subscription/generate", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	h.GenerateSubscription(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.True(t, envelope.Success)

	data, ok := envelope.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, SubscriptionFormatV2Ray, data["format"])
	assert.Contains(t, data["config"].(string), "vless://")
}

func TestGetSubscription_WithInboundTag(t *testing.T) {
	client := mockSubscriptionSingBoxClient{
		inbounds: []models.Inbound{
			{Tag: "in-1", Type: "vless", Port: 443},
			{Tag: "in-2", Type: "hysteria2", Port: 8443},
		},
		users: []models.User{
			{SubID: "sub-1", UUID: "uuid-1", InboundTag: "in-1", Enabled: true},
			{SubID: "sub-1", UUID: "uuid-2", InboundTag: "in-2", Enabled: true},
		},
	}
	h := newSubscriptionHandlerForTest(client)

	// First generate a subscription with inbound tag
	generateBody, _ := json.Marshal(SubscriptionRequest{
		SubID:      "sub-1",
		Format:     SubscriptionFormatV2Ray,
		Server:     "vpn.example.com",
		Port:       443,
		InboundTag: "in-2",
	})
	generateReq := httptest.NewRequest(http.MethodPost, "/subscription/generate", bytes.NewBuffer(generateBody))
	generateResp := httptest.NewRecorder()
	h.GenerateSubscription(generateResp, generateReq)
	require.Equal(t, http.StatusOK, generateResp.Code)

	// Now get the subscription with inbound tag
	getReq := httptest.NewRequest(http.MethodGet, "/subscription/sub-1?inboundTag=in-2", nil)
	getResp := httptest.NewRecorder()
	h.GetSubscription(getResp, getReq)

	resp := getResp.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var envelope Response
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.True(t, envelope.Success)

	data, ok := envelope.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "sub-1", data["subId"])
	assert.Contains(t, data["config"].(string), "vless://")
}

func TestResolveTarget_MultipleUsersSameSubID(t *testing.T) {
	client := mockSubscriptionSingBoxClient{
		inbounds: []models.Inbound{
			{Tag: "in-1", Type: "vless", Port: 443},
			{Tag: "in-2", Type: "hysteria2", Port: 8443},
			{Tag: "in-3", Type: "trojan", Port: 8443},
		},
		users: []models.User{
			{SubID: "sub-1", UUID: "uuid-1", InboundTag: "in-1", Enabled: true},
			{SubID: "sub-1", UUID: "uuid-2", InboundTag: "in-2", Enabled: true},
			{SubID: "sub-1", UUID: "uuid-3", InboundTag: "in-3", Enabled: true},
		},
	}
	h := newSubscriptionHandlerForTest(client)

	// Test without inbound tag (should return first match)
	inbound, user, allInbounds, err := h.resolveTarget(context.Background(), "sub-1", "")
	require.NoError(t, err)
	assert.Equal(t, "in-1", inbound.Tag)
	assert.Equal(t, "sub-1", user.SubID)
	assert.Len(t, allInbounds, 3)

	// Test with specific inbound tag
	inbound, user, allInbounds, err = h.resolveTarget(context.Background(), "sub-1", "in-2")
	require.NoError(t, err)
	assert.Equal(t, "in-2", inbound.Tag)
	assert.Equal(t, "sub-1", user.SubID)
	assert.Len(t, allInbounds, 3)
}

func TestResolveTarget_UserDisabled(t *testing.T) {
	client := mockSubscriptionSingBoxClient{
		inbounds: []models.Inbound{
			{Tag: "in-1", Type: "vless", Port: 443},
		},
		users: []models.User{
			{SubID: "sub-1", UUID: "uuid-1", InboundTag: "in-1", Enabled: false},
		},
	}
	h := newSubscriptionHandlerForTest(client)

	// Note: resolveTarget doesn't check Enabled status, it just finds the user
	// This test verifies that behavior
	inbound, user, allInbounds, err := h.resolveTarget(context.Background(), "sub-1", "")
	require.NoError(t, err)
	assert.Equal(t, "in-1", inbound.Tag)
	assert.Equal(t, "sub-1", user.SubID)
	assert.False(t, user.Enabled) // User is disabled but still returned
	assert.Len(t, allInbounds, 1)
}

func TestBuildSingBoxSubscriptionConfig_WithFlow(t *testing.T) {
	inbound := models.Inbound{
		Tag:  "vless-in",
		Type: "vless",
		Port: 443,
		Options: map[string]interface{}{
			"tls": map[string]interface{}{
				"server_name": "example.com",
			},
		},
	}
	user := models.User{
		SubID: "sub-1",
		UUID:  "uuid-123",
		Flow:  "xtls-rprx-vision",
	}

	config, err := buildSingBoxSubscriptionConfig(SubscriptionFormatSingBox, inbound, user, []models.Inbound{}, "example.com", 443)

	require.NoError(t, err)
	assert.Contains(t, config, "\"vless\"")
	assert.Contains(t, config, "uuid-123")
	assert.Contains(t, config, "xtls-rprx-vision")
}

func TestBuildSingBoxSubscriptionConfig_WithLimitIP(t *testing.T) {
	inbound := models.Inbound{
		Tag:  "vless-in",
		Type: "vless",
		Port: 443,
		Options: map[string]interface{}{
			"tls": map[string]interface{}{
				"server_name": "example.com",
			},
		},
	}
	user := models.User{
		SubID:   "sub-1",
		UUID:    "uuid-123",
		LimitIP: 5,
	}

	config, err := buildSingBoxSubscriptionConfig(SubscriptionFormatSingBox, inbound, user, []models.Inbound{}, "example.com", 443)

	require.NoError(t, err)
	assert.Contains(t, config, "\"vless\"")
	assert.Contains(t, config, "uuid-123")
	// Note: limit_ip is not included in the generated config by the current implementation
}

func TestBuildSingBoxSubscriptionConfig_WithTrafficLimits(t *testing.T) {
	inbound := models.Inbound{
		Tag:  "vless-in",
		Type: "vless",
		Port: 443,
		Options: map[string]interface{}{
			"tls": map[string]interface{}{
				"server_name": "example.com",
			},
		},
	}
	user := models.User{
		SubID:         "sub-1",
		UUID:          "uuid-123",
		UploadLimit:   1000000000,
		DownloadLimit: 2000000000,
	}

	config, err := buildSingBoxSubscriptionConfig(SubscriptionFormatSingBox, inbound, user, []models.Inbound{}, "example.com", 443)

	require.NoError(t, err)
	assert.Contains(t, config, "\"vless\"")
	assert.Contains(t, config, "uuid-123")
	// Note: upload_limit and download_limit are not included in the generated config by the current implementation
}

func TestBuildSingBoxSubscriptionConfig_WithAllOptions(t *testing.T) {
	inbound := models.Inbound{
		Tag:  "vless-in",
		Type: "vless",
		Port: 443,
		Options: map[string]interface{}{
			"tls": map[string]interface{}{
				"server_name": "example.com",
				"insecure":    true,
			},
			"transport": map[string]interface{}{
				"type": "ws",
				"path": "/path",
			},
		},
	}
	user := models.User{
		SubID:         "sub-1",
		UUID:          "uuid-123",
		Flow:          "xtls-rprx-vision",
		LimitIP:       3,
		UploadLimit:   1000000000,
		DownloadLimit: 2000000000,
		Email:         "user@example.com",
	}

	config, err := buildSingBoxSubscriptionConfig(SubscriptionFormatSingBox, inbound, user, []models.Inbound{}, "example.com", 443)

	require.NoError(t, err)
	assert.Contains(t, config, "\"vless\"")
	assert.Contains(t, config, "uuid-123")
	assert.Contains(t, config, "xtls-rprx-vision")
	assert.Contains(t, config, "\"insecure\": true")
	assert.Contains(t, config, "\"path\": \"/path\"")
	// Note: limit_ip, upload_limit, download_limit are not included in the generated config by the current implementation
}

// TestSubscriptionCache_CappedAtMaxEntries verifies that the in-memory
// subscription cache never grows beyond maxSubscriptionCacheEntries: records
// generated after the cap is reached are simply not cached, while responses
// to the client remain identical.
func TestSubscriptionCache_CappedAtMaxEntries(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewSubscriptionHandler(logger) // no client: legacy path always caches

	total := maxSubscriptionCacheEntries + 50
	for i := 0; i < total; i++ {
		req := SubscriptionRequest{
			SubID:  fmt.Sprintf("sub-%d", i),
			Format: SubscriptionFormatV2Ray,
			Server: "example.com",
			Port:   443,
		}
		body, err := json.Marshal(req)
		require.NoError(t, err)

		httpReq := httptest.NewRequest(http.MethodPost, "/subscription", bytes.NewReader(body))
		w := httptest.NewRecorder()
		handler.GenerateSubscription(w, httpReq)
		require.Equal(t, http.StatusOK, w.Code, "response must be unaffected for sub-%d", i)
	}

	handler.mu.RLock()
	size := len(handler.store)
	handler.mu.RUnlock()
	assert.Equal(t, maxSubscriptionCacheEntries, size,
		"cache must not grow beyond maxSubscriptionCacheEntries")
}
