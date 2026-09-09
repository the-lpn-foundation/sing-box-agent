package handlers

import (
	"bytes"
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

	"github.com/oglenyaboss/sing-box-agent/internal/models"
)

type mockSingBoxClient struct {
	inbounds     []models.Inbound
	failOnGet    bool
	failOnCreate bool
	failOnUpdate bool
	failOnDelete bool
}

func (m *mockSingBoxClient) GetInbounds(ctx context.Context) ([]models.Inbound, error) {
	if m.failOnGet {
		return nil, errors.New("mock get error")
	}
	return m.inbounds, nil
}

func (m *mockSingBoxClient) GetUsers(ctx context.Context) ([]models.User, error) {
	return nil, nil
}

func (m *mockSingBoxClient) CreateInbound(ctx context.Context, inbound models.Inbound) error {
	if m.failOnCreate {
		return errors.New("mock create error")
	}
	m.inbounds = append(m.inbounds, inbound)
	return nil
}

func (m *mockSingBoxClient) UpdateInbound(ctx context.Context, inbound models.Inbound) error {
	if m.failOnUpdate {
		return errors.New("mock update error")
	}
	for i, existing := range m.inbounds {
		if existing.Tag == inbound.Tag {
			m.inbounds[i] = inbound
			return nil
		}
	}
	return errors.New("inbound not found")
}

func (m *mockSingBoxClient) DeleteInbound(ctx context.Context, tag string) error {
	if m.failOnDelete {
		return errors.New("mock delete error")
	}
	for i, existing := range m.inbounds {
		if existing.Tag == tag {
			m.inbounds = append(m.inbounds[:i], m.inbounds[i+1:]...)
			return nil
		}
	}
	return errors.New("inbound not found")
}

func (m *mockSingBoxClient) CreateUser(ctx context.Context, user models.User) error {
	return nil
}

func (m *mockSingBoxClient) UpdateUser(ctx context.Context, user models.User) error {
	return nil
}

func (m *mockSingBoxClient) DeleteUser(ctx context.Context, subID string) error {
	return nil
}

func newTestHandler() *InboundHandler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockClient := &mockSingBoxClient{
		inbounds: []models.Inbound{
			{
				Tag:     "vless-reality",
				Type:    "vless",
				Listen:  "127.0.0.1",
				Port:    443,
				Options: map[string]interface{}{},
			},
		},
	}
	return NewInboundHandler(mockClient, logger)
}

func TestListInbounds(t *testing.T) {
	handler := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/inbounds", nil)
	w := httptest.NewRecorder()
	handler.ListInbounds(w, req)
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response Response
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.True(t, response.Success)

	inbounds, ok := response.Data.([]interface{})
	require.True(t, ok)
	assert.Len(t, inbounds, 1)
}

func TestListInbounds_Error(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, nil))
	mockClient := &mockSingBoxClient{failOnGet: true}
	handler := NewInboundHandler(mockClient, logger)

	req := httptest.NewRequest(http.MethodGet, "/inbounds", nil)
	w := httptest.NewRecorder()
	handler.ListInbounds(w, req)
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var response Response
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.NotNil(t, response.Error)
	assert.Equal(t, CodeInternalError, response.Error.Code)
}

func TestGetInbound_Found(t *testing.T) {
	handler := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/inbounds/vless-reality", nil)
	w := httptest.NewRecorder()
	handler.GetInbound(w, req)
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response Response
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.True(t, response.Success)
}

func TestGetInbound_NotFound(t *testing.T) {
	handler := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/inbounds/nonexistent", nil)
	w := httptest.NewRecorder()
	handler.GetInbound(w, req)
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var response Response
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.NotNil(t, response.Error)
	assert.Equal(t, CodeInboundNotFound, response.Error.Code)
}

func TestGetInbound_MissingTag(t *testing.T) {
	handler := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/inbounds/", nil)
	w := httptest.NewRecorder()
	handler.GetInbound(w, req)
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var response Response
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.NotNil(t, response.Error)
	assert.Equal(t, CodeInvalidRequest, response.Error.Code)
}

func TestCreateInbound_InvalidType(t *testing.T) {
	handler := newTestHandler()
	inbound := models.Inbound{
		Tag:    "invalid-inbound",
		Type:   "invalid-type",
		Listen: "127.0.0.1",
		Port:   8443,
	}
	body, _ := json.Marshal(inbound)
	req := httptest.NewRequest(http.MethodPost, "/inbounds", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	handler.CreateInbound(w, req)
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var response Response
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.NotNil(t, response.Error)
}

func TestCreateInbound_DuplicateTag(t *testing.T) {
	handler := newTestHandler()
	inbound := models.Inbound{
		Tag:    "vless-reality",
		Type:   "vless",
		Listen: "127.0.0.1",
		Port:   444,
	}
	body, _ := json.Marshal(inbound)
	req := httptest.NewRequest(http.MethodPost, "/inbounds", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	handler.CreateInbound(w, req)
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusConflict, resp.StatusCode)

	var response Response
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.NotNil(t, response.Error)
	assert.Equal(t, CodeInboundExists, response.Error.Code)
}

func TestCreateInbound_MissingRequiredFields(t *testing.T) {
	handler := newTestHandler()

	tests := []struct {
		name    string
		inbound models.Inbound
	}{
		{
			name:    "missing tag",
			inbound: models.Inbound{Type: "vless", Listen: "127.0.0.1"},
		},
		{
			name:    "missing type",
			inbound: models.Inbound{Tag: "test", Listen: "127.0.0.1"},
		},
		{
			name:    "missing listen",
			inbound: models.Inbound{Tag: "test", Type: "vless"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.inbound)
			req := httptest.NewRequest(http.MethodPost, "/inbounds", bytes.NewBuffer(body))
			w := httptest.NewRecorder()
			handler.CreateInbound(w, req)
			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

			var response Response
			err := json.NewDecoder(resp.Body).Decode(&response)
			require.NoError(t, err)
			assert.False(t, response.Success)
			assert.NotNil(t, response.Error)
			assert.Equal(t, CodeInvalidRequest, response.Error.Code)
		})
	}
}

func TestUpdateInbound_NotFound(t *testing.T) {
	handler := newTestHandler()
	inbound := models.Inbound{
		Tag:    "nonexistent",
		Type:   "vless",
		Listen: "127.0.0.1",
		Port:   443,
	}
	body, _ := json.Marshal(inbound)
	req := httptest.NewRequest(http.MethodPut, "/inbounds/nonexistent", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	handler.UpdateInbound(w, req)
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var response Response
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.NotNil(t, response.Error)
	assert.Equal(t, CodeInboundNotFound, response.Error.Code)
}

func TestUpdateInbound_TagMismatch(t *testing.T) {
	handler := newTestHandler()
	inbound := models.Inbound{
		Tag:    "different-tag",
		Type:   "vless",
		Listen: "127.0.0.1",
		Port:   443,
	}
	body, _ := json.Marshal(inbound)
	req := httptest.NewRequest(http.MethodPut, "/inbounds/vless-reality", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	handler.UpdateInbound(w, req)
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var response Response
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.NotNil(t, response.Error)
	assert.Equal(t, CodeInvalidRequest, response.Error.Code)
}

func TestDeleteInbound_Found(t *testing.T) {
	handler := newTestHandler()
	req := httptest.NewRequest(http.MethodDelete, "/inbounds/vless-reality", nil)
	w := httptest.NewRecorder()
	handler.DeleteInbound(w, req)
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestDeleteInbound_NotFound(t *testing.T) {
	handler := newTestHandler()
	req := httptest.NewRequest(http.MethodDelete, "/inbounds/nonexistent", nil)
	w := httptest.NewRecorder()
	handler.DeleteInbound(w, req)
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var response Response
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.NotNil(t, response.Error)
	assert.Equal(t, CodeInboundNotFound, response.Error.Code)
}

func TestDeleteInbound_MissingTag(t *testing.T) {
	handler := newTestHandler()
	req := httptest.NewRequest(http.MethodDelete, "/inbounds/", nil)
	w := httptest.NewRecorder()
	handler.DeleteInbound(w, req)
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var response Response
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.NotNil(t, response.Error)
	assert.Equal(t, CodeInvalidRequest, response.Error.Code)
}

func TestValidateInbound_Valid(t *testing.T) {
	handler := newTestHandler()

	tests := []struct {
		name    string
		inbound models.Inbound
		valid   bool
	}{
		{
			name: "valid vless",
			inbound: models.Inbound{
				Tag:    "vless-test",
				Type:   "vless",
				Listen: "127.0.0.1",
				Port:   443,
			},
			valid: true,
		},
		{
			name: "valid trojan",
			inbound: models.Inbound{
				Tag:    "trojan-test",
				Type:   "trojan",
				Listen: "0.0.0.0",
				Port:   8443,
			},
			valid: true,
		},
		{
			name: "valid vmess",
			inbound: models.Inbound{
				Tag:    "vmess-test",
				Type:   "vmess",
				Listen: "::",
				Port:   8080,
			},
			valid: true,
		},
		{
			name: "valid shadowsocks",
			inbound: models.Inbound{
				Tag:    "ss-test",
				Type:   "shadowsocks",
				Listen: "127.0.0.1",
				Port:   8388,
			},
			valid: true,
		},
		{
			name: "valid hysteria2",
			inbound: models.Inbound{
				Tag:    "h2-test",
				Type:   "hysteria2",
				Listen: "0.0.0.0",
				Port:   443,
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateInbound(&tt.inbound)
			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestValidateInbound_Invalid(t *testing.T) {
	handler := newTestHandler()

	tests := []struct {
		name    string
		inbound models.Inbound
	}{
		{
			name: "missing tag",
			inbound: models.Inbound{
				Type:   "vless",
				Listen: "127.0.0.1",
			},
		},
		{
			name: "missing type",
			inbound: models.Inbound{
				Tag:    "test",
				Listen: "127.0.0.1",
			},
		},
		{
			name: "missing listen",
			inbound: models.Inbound{
				Tag:  "test",
				Type: "vless",
			},
		},
		{
			name: "invalid type",
			inbound: models.Inbound{
				Tag:    "test",
				Type:   "invalid",
				Listen: "127.0.0.1",
			},
		},
		{
			name: "invalid port too low",
			inbound: models.Inbound{
				Tag:    "test",
				Type:   "vless",
				Listen: "127.0.0.1",
				Port:   -1,
			},
		},
		{
			name: "invalid port too high",
			inbound: models.Inbound{
				Tag:    "test",
				Type:   "vless",
				Listen: "127.0.0.1",
				Port:   70000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateInbound(&tt.inbound)
			assert.Error(t, err)
		})
	}
}

func TestValidInboundTypes(t *testing.T) {
	expectedTypes := []string{
		"trojan",
		"vless",
		"vmess",
		"shadowsocks",
		"shadowtls",
		"hysteria2",
		"tuic",
	}
	assert.Equal(t, expectedTypes, ValidInboundTypes)
}

func TestErrorCodes(t *testing.T) {
	assert.Equal(t, "INVALID_REQUEST", CodeInvalidRequest)
	assert.Equal(t, "INBOUND_NOT_FOUND", CodeInboundNotFound)
	assert.Equal(t, "INBOUND_EXISTS", CodeInboundExists)
	assert.Equal(t, "INTERNAL_ERROR", CodeInternalError)
}

func TestGetInbound_GetInboundsError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, nil))
	mockClient := &mockSingBoxClient{failOnGet: true}
	handler := NewInboundHandler(mockClient, logger)

	req := httptest.NewRequest(http.MethodGet, "/inbounds/vless-reality", nil)
	w := httptest.NewRecorder()
	handler.GetInbound(w, req)
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var response Response
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.NotNil(t, response.Error)
	assert.Equal(t, CodeInternalError, response.Error.Code)
}

func TestCreateInbound_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockClient := &mockSingBoxClient{
		inbounds: []models.Inbound{
			{Tag: "vless-reality", Type: "vless", Listen: "127.0.0.1", Port: 443},
		},
	}
	handler := NewInboundHandler(mockClient, logger)

	inbound := models.Inbound{
		Tag:    "new-inbound",
		Type:   "vless",
		Listen: "127.0.0.1",
		Port:   8443,
	}
	body, _ := json.Marshal(inbound)
	req := httptest.NewRequest(http.MethodPost, "/inbounds", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	handler.CreateInbound(w, req)
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var response Response
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.True(t, response.Success)
}

func TestCreateInbound_GetInboundsError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, nil))
	mockClient := &mockSingBoxClient{failOnGet: true}
	handler := NewInboundHandler(mockClient, logger)

	inbound := models.Inbound{
		Tag:    "new-inbound",
		Type:   "vless",
		Listen: "127.0.0.1",
		Port:   8443,
	}
	body, _ := json.Marshal(inbound)
	req := httptest.NewRequest(http.MethodPost, "/inbounds", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	handler.CreateInbound(w, req)
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var response Response
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.Equal(t, CodeInternalError, response.Error.Code)
}

func TestCreateInbound_CreateError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, nil))
	mockClient := &mockSingBoxClient{failOnCreate: true}
	handler := NewInboundHandler(mockClient, logger)

	inbound := models.Inbound{
		Tag:    "new-inbound",
		Type:   "vless",
		Listen: "127.0.0.1",
		Port:   8443,
	}
	body, _ := json.Marshal(inbound)
	req := httptest.NewRequest(http.MethodPost, "/inbounds", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	handler.CreateInbound(w, req)
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var response Response
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.Equal(t, CodeInternalError, response.Error.Code)
}

func TestUpdateInbound_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockClient := &mockSingBoxClient{
		inbounds: []models.Inbound{
			{Tag: "vless-reality", Type: "vless", Listen: "127.0.0.1", Port: 443},
		},
	}
	handler := NewInboundHandler(mockClient, logger)

	inbound := models.Inbound{
		Tag:    "vless-reality",
		Type:   "vless",
		Listen: "0.0.0.0",
		Port:   443,
	}
	body, _ := json.Marshal(inbound)
	req := httptest.NewRequest(http.MethodPut, "/inbounds/vless-reality", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	handler.UpdateInbound(w, req)
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response Response
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.True(t, response.Success)
}

func TestUpdateInbound_GetInboundsError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, nil))
	mockClient := &mockSingBoxClient{failOnGet: true}
	handler := NewInboundHandler(mockClient, logger)

	inbound := models.Inbound{
		Tag:    "vless-reality",
		Type:   "vless",
		Listen: "127.0.0.1",
		Port:   443,
	}
	body, _ := json.Marshal(inbound)
	req := httptest.NewRequest(http.MethodPut, "/inbounds/vless-reality", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	handler.UpdateInbound(w, req)
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var response Response
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.Equal(t, CodeInternalError, response.Error.Code)
}

func TestUpdateInbound_UpdateError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, nil))
	mockClient := &mockSingBoxClient{
		inbounds: []models.Inbound{
			{Tag: "vless-reality", Type: "vless", Listen: "127.0.0.1", Port: 443},
		},
		failOnUpdate: true,
	}
	handler := NewInboundHandler(mockClient, logger)

	inbound := models.Inbound{
		Tag:    "vless-reality",
		Type:   "vless",
		Listen: "0.0.0.0",
		Port:   443,
	}
	body, _ := json.Marshal(inbound)
	req := httptest.NewRequest(http.MethodPut, "/inbounds/vless-reality", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	handler.UpdateInbound(w, req)
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var response Response
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.Equal(t, CodeInternalError, response.Error.Code)
}

func TestDeleteInbound_GetInboundsError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, nil))
	mockClient := &mockSingBoxClient{failOnGet: true}
	handler := NewInboundHandler(mockClient, logger)

	req := httptest.NewRequest(http.MethodDelete, "/inbounds/vless-reality", nil)
	w := httptest.NewRecorder()
	handler.DeleteInbound(w, req)
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var response Response
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.Equal(t, CodeInternalError, response.Error.Code)
}

func TestDeleteInbound_DeleteError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, nil))
	mockClient := &mockSingBoxClient{
		inbounds: []models.Inbound{
			{Tag: "vless-reality", Type: "vless", Listen: "127.0.0.1", Port: 443},
		},
		failOnDelete: true,
	}
	handler := NewInboundHandler(mockClient, logger)

	req := httptest.NewRequest(http.MethodDelete, "/inbounds/vless-reality", nil)
	w := httptest.NewRecorder()
	handler.DeleteInbound(w, req)
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var response Response
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.Equal(t, CodeInternalError, response.Error.Code)
}

func TestInboundHandlerSendSuccess_EncodeError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockClient := &mockSingBoxClient{}
	handler := NewInboundHandler(mockClient, logger)

	// Create a custom ResponseWriter that will fail on Write
	w := &failingResponseWriter{}

	handler.sendSuccess(w, http.StatusOK, map[string]string{"key": "value"})
	// Should not panic, just log error
}

func TestInboundHandlerSendError_EncodeError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockClient := &mockSingBoxClient{}
	handler := NewInboundHandler(mockClient, logger)

	// Create a custom ResponseWriter that will fail on Write
	w := &failingResponseWriter{}

	handler.sendError(w, http.StatusBadRequest, CodeInvalidRequest, "test error")
	// Should not panic, just log error
}

// failingResponseWriter is a mock ResponseWriter that always fails on Write.
type failingResponseWriter struct {
	header http.Header
}

func (f *failingResponseWriter) Header() http.Header {
	if f.header == nil {
		f.header = make(http.Header)
	}
	return f.header
}

func (f *failingResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("write error")
}

func (f *failingResponseWriter) WriteHeader(statusCode int) {
}

func TestUpdateInbound_InvalidRequestBody(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockClient := &mockSingBoxClient{}
	handler := NewInboundHandler(mockClient, logger)

	req := httptest.NewRequest(http.MethodPut, "/inbounds/vless-reality", bytes.NewBufferString("invalid json"))
	w := httptest.NewRecorder()
	handler.UpdateInbound(w, req)
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var response Response
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.Equal(t, CodeInvalidRequest, response.Error.Code)
}

func TestUpdateInbound_ValidationError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockClient := &mockSingBoxClient{
		inbounds: []models.Inbound{
			{Tag: "vless-reality", Type: "vless", Listen: "127.0.0.1", Port: 443},
		},
	}
	handler := NewInboundHandler(mockClient, logger)

	inbound := models.Inbound{
		Tag:    "vless-reality",
		Type:   "invalid-type",
		Listen: "127.0.0.1",
		Port:   443,
	}
	body, _ := json.Marshal(inbound)
	req := httptest.NewRequest(http.MethodPut, "/inbounds/vless-reality", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	handler.UpdateInbound(w, req)
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var response Response
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.Equal(t, CodeInvalidRequest, response.Error.Code)
}

func TestCreateInbound_InvalidRequestBody(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockClient := &mockSingBoxClient{}
	handler := NewInboundHandler(mockClient, logger)

	req := httptest.NewRequest(http.MethodPost, "/inbounds", bytes.NewBufferString("invalid json"))
	w := httptest.NewRecorder()
	handler.CreateInbound(w, req)
	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var response Response
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.False(t, response.Success)
	assert.Equal(t, CodeInvalidRequest, response.Error.Code)
}
