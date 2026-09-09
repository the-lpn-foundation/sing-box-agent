package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/the-lpn-foundation/sing-box-agent/internal/auth"
	"github.com/the-lpn-foundation/sing-box-agent/internal/handlers"
	"github.com/the-lpn-foundation/sing-box-agent/internal/middleware"
	"github.com/the-lpn-foundation/sing-box-agent/internal/models"
)

type MockSingBoxConfigE2E struct {
	config map[string]interface{}
	mu     sync.RWMutex
}

func (m *MockSingBoxConfigE2E) GetInbounds(ctx context.Context) ([]models.Inbound, error) {
	return []models.Inbound{}, nil
}

func (m *MockSingBoxConfigE2E) GetUsers(ctx context.Context) ([]models.User, error) {
	return []models.User{}, nil
}

func (m *MockSingBoxConfigE2E) CreateInbound(ctx context.Context, inbound models.Inbound) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.config == nil {
		m.config = make(map[string]interface{})
	}

	inbounds, ok := m.config["inbounds"]
	if !ok {
		m.config["inbounds"] = []models.Inbound{inbound}
		return nil
	}

	inboundList, ok := inbounds.([]models.Inbound)
	if !ok {
		m.config["inbounds"] = []models.Inbound{inbound}
		return nil
	}

	for _, existing := range inboundList {
		if existing.Tag == inbound.Tag {
			return errors.New("INBOUND_ALREADY_EXISTS: Inbound already exists")
		}
	}

	m.config["inbounds"] = append(inboundList, inbound)
	return nil
}

func (m *MockSingBoxConfigE2E) UpdateInbound(ctx context.Context, inbound models.Inbound) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	inbounds, ok := m.config["inbounds"]
	if !ok {
		return errors.New("INBOUND_NOT_FOUND: Inbound not found")
	}

	inboundList, ok := inbounds.([]models.Inbound)
	if !ok {
		return errors.New("INBOUND_NOT_FOUND: Inbound not found")
	}

	for i, existing := range inboundList {
		if existing.Tag == inbound.Tag {
			inboundList[i] = inbound
			m.config["inbounds"] = inboundList
			return nil
		}
	}

	return errors.New("INBOUND_NOT_FOUND: Inbound not found")
}

func (m *MockSingBoxConfigE2E) DeleteInbound(ctx context.Context, tag string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	inbounds, ok := m.config["inbounds"]
	if !ok {
		return errors.New("INBOUND_NOT_FOUND: Inbound not found")
	}

	inboundList, ok := inbounds.([]models.Inbound)
	if !ok {
		return errors.New("INBOUND_NOT_FOUND: Inbound not found")
	}

	for i, existing := range inboundList {
		if existing.Tag == tag {
			m.config["inbounds"] = append(inboundList[:i], inboundList[i+1:]...)
			return nil
		}
	}

	return errors.New("INBOUND_NOT_FOUND: Inbound not found")
}

func (m *MockSingBoxConfigE2E) CreateUser(ctx context.Context, user models.User) error {
	return nil
}

func (m *MockSingBoxConfigE2E) UpdateUser(ctx context.Context, user models.User) error {
	return nil
}

func (m *MockSingBoxConfigE2E) DeleteUser(ctx context.Context, subID string) error {
	return nil
}

type MockCoreServiceE2E struct {
	config       map[string]interface{}
	reloadCount  int
	restartCount int
	mu           sync.RWMutex
}

func (m *MockCoreServiceE2E) Reload(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reloadCount++
	return nil
}

func (m *MockCoreServiceE2E) Restart(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restartCount++
	return nil
}

func (m *MockCoreServiceE2E) GetConfig(ctx context.Context) (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.config == nil {
		return make(map[string]interface{}), nil
	}

	configCopy := make(map[string]interface{})
	for k, v := range m.config {
		configCopy[k] = v
	}
	return configCopy, nil
}

func setupConfigE2EServer(t *testing.T) *httptest.Server {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	mockSingBox := &MockSingBoxConfigE2E{
		config: map[string]interface{}{
			"log": map[string]string{"level": "info"},
			"dns": map[string]interface{}{
				"servers": []map[string]string{
					{"tag": "google", "address": "tls://8.8.8.8"},
				},
			},
		},
	}

	inboundHandler := handlers.NewInboundHandler(mockSingBox, logger)

	mockCore := &MockCoreServiceE2E{
		config: make(map[string]interface{}),
	}
	coreHandler := handlers.NewCoreHandler(mockCore, logger)

	nonceCache := auth.NewNonceCache()
	authConfig := middleware.AuthConfig{
		Token:      "config-e2e-token",
		Secret:     "config-e2e-secret-key-12345",
		NonceCache: nonceCache,
		Logger:     logger,
	}

	mux := http.NewServeMux()

	// Register paths without HTTP method in the pattern. Method is validated by the handler in tests.
	// Use single handler for /inbounds and /inbounds/ paths and dispatch by method.
	mux.Handle("/inbounds", middleware.AuthMiddleware(authConfig)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			inboundHandler.ListInbounds(w, r)
		case http.MethodPost:
			inboundHandler.CreateInbound(w, r)
		default:
			http.NotFound(w, r)
		}
	})))

	mux.Handle("/inbounds/", middleware.AuthMiddleware(authConfig)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			inboundHandler.UpdateInbound(w, r)
		case http.MethodDelete:
			inboundHandler.DeleteInbound(w, r)
		default:
			http.NotFound(w, r)
		}
	})))

	mux.Handle("/core/reload", middleware.AuthMiddleware(authConfig)(http.HandlerFunc(coreHandler.ReloadConfig)))
	mux.Handle("/core/restart", middleware.AuthMiddleware(authConfig)(http.HandlerFunc(coreHandler.RestartCore)))
	mux.Handle("/core/config", middleware.AuthMiddleware(authConfig)(http.HandlerFunc(coreHandler.GetConfig)))

	return httptest.NewServer(mux)
}

func makeConfigAuthenticatedRequest(t *testing.T, method, url string, body interface{}) *http.Request {
	t.Helper()

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		require.NoError(t, err)
	} else {
		bodyBytes = []byte("")
	}

	req, _ := http.NewRequest(method, url, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer config-e2e-token")

	timestamp := time.Now().Unix()
	nonce := "config-nonce-" + time.Now().Format("20060102150405")
	bodyHash := auth.ComputeBodyHash(bodyBytes)

	canonicalString := auth.BuildCanonicalString(
		nonce,
		strconv.FormatInt(timestamp, 10),
		method,
		url,
		bodyHash,
	)

	signature := auth.SignRequest(canonicalString, "config-e2e-secret-key-12345")

	req.Header.Set("X-Signature", signature)
	req.Header.Set("X-Timestamp", strconv.FormatInt(timestamp, 10))
	req.Header.Set("X-Nonce", nonce)

	return req
}

func TestConfigWorkflow_CreateAndUpdateInbound(t *testing.T) {
	t.Parallel()

	if checkAgentAvailable(t) {
		t.Skip("Agent running; skipping test server-based E2E test")
	}

	server := setupConfigE2EServer(t)
	defer server.Close()

	createInbound := models.Inbound{
		Tag:     "vless-test",
		Type:    "vless",
		Listen:  "0.0.0.0",
		Port:    8443,
		Options: map[string]interface{}{},
	}

	createReq := makeConfigAuthenticatedRequest(t, "POST", server.URL+"/inbounds", createInbound)
	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer func() { _ = createResp.Body.Close() }()

	assert.Equal(t, http.StatusCreated, createResp.StatusCode)

	var createResult handlers.Response
	err = json.NewDecoder(createResp.Body).Decode(&createResult)
	require.NoError(t, err)
	assert.True(t, createResult.Success)

	updatedInbound := models.Inbound{
		Tag:     "vless-test",
		Type:    "vless",
		Listen:  "0.0.0.0",
		Port:    9443,
		Options: map[string]interface{}{},
	}

	updateReq := makeConfigAuthenticatedRequest(t, "PUT", server.URL+"/inbounds/vless-test", updatedInbound)
	updateResp, err := http.DefaultClient.Do(updateReq)
	require.NoError(t, err)
	defer func() { _ = updateResp.Body.Close() }()

	assert.Equal(t, http.StatusOK, updateResp.StatusCode)

	var updateResult handlers.Response
	err = json.NewDecoder(updateResp.Body).Decode(&updateResult)
	require.NoError(t, err)
	assert.True(t, updateResult.Success)
}

func TestConfigWorkflow_ReloadAndRestart(t *testing.T) {
	t.Parallel()

	if checkAgentAvailable(t) {
		t.Skip("Agent running; skipping test server-based E2E test")
	}

	server := setupConfigE2EServer(t)
	defer server.Close()

	reloadReq := makeConfigAuthenticatedRequest(t, "POST", server.URL+"/core/reload", nil)
	reloadResp, err := http.DefaultClient.Do(reloadReq)
	require.NoError(t, err)
	defer func() { _ = reloadResp.Body.Close() }()

	assert.Equal(t, http.StatusOK, reloadResp.StatusCode)

	var reloadResult handlers.Response
	err = json.NewDecoder(reloadResp.Body).Decode(&reloadResult)
	require.NoError(t, err)
	assert.True(t, reloadResult.Success)

	restartReq := makeConfigAuthenticatedRequest(t, "POST", server.URL+"/core/restart", nil)
	restartResp, err := http.DefaultClient.Do(restartReq)
	require.NoError(t, err)
	defer func() { _ = restartResp.Body.Close() }()

	assert.Equal(t, http.StatusOK, restartResp.StatusCode)

	var restartResult handlers.Response
	err = json.NewDecoder(restartResp.Body).Decode(&restartResult)
	require.NoError(t, err)
	assert.True(t, restartResult.Success)
}

func TestConfigWorkflow_GetConfig(t *testing.T) {
	t.Parallel()

	if checkAgentAvailable(t) {
		t.Skip("Agent running; skipping test server-based E2E test")
	}

	server := setupConfigE2EServer(t)
	defer server.Close()

	getConfigReq := makeConfigAuthenticatedRequest(t, "GET", server.URL+"/core/config", nil)
	getConfigResp, err := http.DefaultClient.Do(getConfigReq)
	require.NoError(t, err)
	defer func() { _ = getConfigResp.Body.Close() }()

	assert.Equal(t, http.StatusOK, getConfigResp.StatusCode)

	var getConfigResult handlers.Response
	err = json.NewDecoder(getConfigResp.Body).Decode(&getConfigResult)
	require.NoError(t, err)
	assert.True(t, getConfigResult.Success)
	assert.NotNil(t, getConfigResult.Data)
}

func TestConfigWorkflow_FullDeployment(t *testing.T) {
	t.Parallel()

	if checkAgentAvailable(t) {
		t.Skip("Agent running; skipping test server-based E2E test")
	}

	server := setupConfigE2EServer(t)
	defer server.Close()

	inbounds := []models.Inbound{
		{
			Tag:     "vless-reality",
			Type:    "vless",
			Listen:  "0.0.0.0",
			Port:    443,
			Options: map[string]interface{}{},
		},
		{
			Tag:     "hysteria2",
			Type:    "hysteria2",
			Listen:  "0.0.0.0",
			Port:    8443,
			Options: map[string]interface{}{},
		},
	}

	for _, inbound := range inbounds {
		createReq := makeConfigAuthenticatedRequest(t, "POST", server.URL+"/inbounds", inbound)
		createResp, err := http.DefaultClient.Do(createReq)
		require.NoError(t, err)
		_ = createResp.Body.Close()

		assert.Equal(t, http.StatusCreated, createResp.StatusCode)
	}

	reloadReq := makeConfigAuthenticatedRequest(t, "POST", server.URL+"/core/reload", nil)
	reloadResp, err := http.DefaultClient.Do(reloadReq)
	require.NoError(t, err)
	defer func() { _ = reloadResp.Body.Close() }()

	assert.Equal(t, http.StatusOK, reloadResp.StatusCode)

	listReq := makeConfigAuthenticatedRequest(t, "GET", server.URL+"/inbounds", nil)
	listResp, err := http.DefaultClient.Do(listReq)
	require.NoError(t, err)
	defer func() { _ = listResp.Body.Close() }()

	assert.Equal(t, http.StatusOK, listResp.StatusCode)

	var listResult handlers.Response
	err = json.NewDecoder(listResp.Body).Decode(&listResult)
	require.NoError(t, err)
	assert.True(t, listResult.Success)

	var inboundsList []models.Inbound
	inboundsBytes, err := json.Marshal(listResult.Data)
	require.NoError(t, err)
	err = json.Unmarshal(inboundsBytes, &inboundsList)
	require.NoError(t, err)
	assert.Equal(t, 2, len(inboundsList))
}

func TestConfigWorkflow_DeleteAndRecreate(t *testing.T) {
	t.Parallel()

	if checkAgentAvailable(t) {
		t.Skip("Agent running; skipping test server-based E2E test")
	}

	server := setupConfigE2EServer(t)
	defer server.Close()

	createInbound := models.Inbound{
		Tag:     "temp-inbound",
		Type:    "vmess",
		Listen:  "0.0.0.0",
		Port:    10000,
		Options: map[string]interface{}{},
	}

	createReq := makeConfigAuthenticatedRequest(t, "POST", server.URL+"/inbounds", createInbound)
	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	_ = createResp.Body.Close()

	assert.Equal(t, http.StatusCreated, createResp.StatusCode)

	deleteReq := makeConfigAuthenticatedRequest(t, "DELETE", server.URL+"/inbounds/temp-inbound", nil)
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	require.NoError(t, err)
	defer func() { _ = deleteResp.Body.Close() }()

	assert.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	recreateReq := makeConfigAuthenticatedRequest(t, "POST", server.URL+"/inbounds", createInbound)
	recreateResp, err := http.DefaultClient.Do(recreateReq)
	require.NoError(t, err)
	defer func() { _ = recreateResp.Body.Close() }()

	assert.Equal(t, http.StatusCreated, recreateResp.StatusCode)

	reloadReq := makeConfigAuthenticatedRequest(t, "POST", server.URL+"/core/reload", nil)
	reloadResp, err := http.DefaultClient.Do(reloadReq)
	require.NoError(t, err)
	defer func() { _ = reloadResp.Body.Close() }()

	assert.Equal(t, http.StatusOK, reloadResp.StatusCode)
}

func TestConfigWorkflow_MultipleReloads(t *testing.T) {
	t.Parallel()

	if checkAgentAvailable(t) {
		t.Skip("Agent running; skipping test server-based E2E test")
	}

	server := setupConfigE2EServer(t)
	defer server.Close()

	for i := 0; i < 5; i++ {
		reloadReq := makeConfigAuthenticatedRequest(t, "POST", server.URL+"/core/reload", nil)
		reloadResp, err := http.DefaultClient.Do(reloadReq)
		require.NoError(t, err)
		_ = reloadResp.Body.Close()

		assert.Equal(t, http.StatusOK, reloadResp.StatusCode)

		var result handlers.Response
		err = json.NewDecoder(reloadResp.Body).Decode(&result)
		require.NoError(t, err)
		assert.True(t, result.Success)
	}

	restartReq := makeConfigAuthenticatedRequest(t, "POST", server.URL+"/core/restart", nil)
	restartResp, err := http.DefaultClient.Do(restartReq)
	require.NoError(t, err)
	defer func() { _ = restartResp.Body.Close() }()

	assert.Equal(t, http.StatusOK, restartResp.StatusCode)
}

func TestConfigWorkflow_LoadFromFixture(t *testing.T) {
	t.Parallel()

	if _, err := os.Stat("../../fixtures/singbox_config.json"); os.IsNotExist(err) {
		t.Skip("Fixture file not found, skipping test")
	}

	if checkAgentAvailable(t) {
		t.Skip("Agent running; skipping test server-based E2E test")
	}

	server := setupConfigE2EServer(t)
	defer server.Close()

	fixtureData, err := os.ReadFile("../../fixtures/singbox_config.json")
	require.NoError(t, err)

	var config map[string]interface{}
	err = json.Unmarshal(fixtureData, &config)
	require.NoError(t, err)

	createReq := makeConfigAuthenticatedRequest(t, "POST", server.URL+"/inbounds", config)
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Logf("Config upload may fail due to type mismatch: %v", err)
	} else {
		defer func() { _ = createResp.Body.Close() }()
		assert.True(t, createResp.StatusCode >= 200 && createResp.StatusCode < 300)
	}
}
