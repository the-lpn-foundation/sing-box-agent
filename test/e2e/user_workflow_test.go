package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lenya/sing-box-agent/internal/auth"
	"github.com/lenya/sing-box-agent/internal/handlers"
	"github.com/lenya/sing-box-agent/internal/middleware"
	"github.com/lenya/sing-box-agent/internal/models"
	syncpkg "github.com/lenya/sing-box-agent/internal/sync"
)

type MockSingBoxE2E struct {
	inbounds []models.Inbound
	users    []models.User
}

func (m *MockSingBoxE2E) GetInbounds(ctx context.Context) ([]models.Inbound, error) {
	return m.inbounds, nil
}

func (m *MockSingBoxE2E) GetUsers(ctx context.Context) ([]models.User, error) {
	return m.users, nil
}

func (m *MockSingBoxE2E) CreateInbound(ctx context.Context, inbound models.Inbound) error {
	for _, existing := range m.inbounds {
		if existing.Tag == inbound.Tag {
			return errors.New("INBOUND_ALREADY_EXISTS: Inbound already exists")
		}
	}
	m.inbounds = append(m.inbounds, inbound)
	return nil
}

func (m *MockSingBoxE2E) UpdateInbound(ctx context.Context, inbound models.Inbound) error {
	for i, existing := range m.inbounds {
		if existing.Tag == inbound.Tag {
			m.inbounds[i] = inbound
			return nil
		}
	}
	return errors.New("INBOUND_NOT_FOUND: Inbound not found")
}

func (m *MockSingBoxE2E) DeleteInbound(ctx context.Context, tag string) error {
	for i, existing := range m.inbounds {
		if existing.Tag == tag {
			m.inbounds = append(m.inbounds[:i], m.inbounds[i+1:]...)
			return nil
		}
	}
	return errors.New("INBOUND_NOT_FOUND: Inbound not found")
}

func (m *MockSingBoxE2E) CreateUser(ctx context.Context, user models.User) error {
	for _, existing := range m.users {
		if existing.SubID == user.SubID {
			return errors.New("USER_ALREADY_EXISTS: User already exists")
		}
	}
	m.users = append(m.users, user)
	return nil
}

func (m *MockSingBoxE2E) UpdateUser(ctx context.Context, user models.User) error {
	for i, existing := range m.users {
		if existing.SubID == user.SubID {
			m.users[i] = user
			return nil
		}
	}
	return errors.New("USER_NOT_FOUND: User not found")
}

func (m *MockSingBoxE2E) DeleteUser(ctx context.Context, subID string) error {
	for i, existing := range m.users {
		if existing.SubID == subID {
			m.users = append(m.users[:i], m.users[i+1:]...)
			return nil
		}
	}
	return errors.New("USER_NOT_FOUND: User not found")
}

func setupE2EServer(t *testing.T) *httptest.Server {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	mockSingBox := &MockSingBoxE2E{
		inbounds: []models.Inbound{
			{Tag: "vless-reality", Type: "vless", Listen: "0.0.0.0", Port: 443},
		},
		users: []models.User{},
	}

	// instantiate sync engine (not used by tests directly)
	_ = syncpkg.NewEngine(mockSingBox)
	userHandler := handlers.NewUserHandler()

	nonceCache := auth.NewNonceCache()
	authConfig := middleware.AuthConfig{
		Token:      "e2e-test-token",
		Secret:     "e2e-test-secret-key-12345",
		NonceCache: nonceCache,
		Logger:     logger,
	}

	mux := http.NewServeMux()

	// Single entrypoint for /inbounds/ paths. Delegate to appropriate handler
	// based on HTTP method and URL path. Auth middleware wraps the delegator.
	mux.Handle("/inbounds/", middleware.AuthMiddleware(authConfig)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/users"):
			userHandler.ListUsers(w, r)
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/users"):
			userHandler.CreateUser(w, r)
		case r.Method == http.MethodPut && strings.Contains(path, "/users/"):
			userHandler.UpdateUser(w, r)
		case r.Method == http.MethodDelete && strings.Contains(path, "/users/"):
			userHandler.DeleteUser(w, r)
		default:
			http.NotFound(w, r)
		}
	})))

	return httptest.NewServer(mux)
}

func makeE2EAuthenticatedRequest(t *testing.T, method, url string, body interface{}) *http.Request {
	t.Helper()

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal body: %v", err)
		}
	} else {
		bodyBytes = []byte("")
	}

	req, _ := http.NewRequest(method, url, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer e2e-test-token")

	timestamp := time.Now().Unix()
	nonce := "e2e-nonce-" + time.Now().Format("20060102150405")
	bodyHash := auth.ComputeBodyHash(bodyBytes)

	canonicalString := auth.BuildCanonicalString(
		nonce,
		strconv.FormatInt(timestamp, 10),
		method,
		url,
		bodyHash,
	)

	signature := auth.SignRequest(canonicalString, "e2e-test-secret-key-12345")

	req.Header.Set("X-Signature", signature)
	req.Header.Set("X-Timestamp", strconv.FormatInt(timestamp, 10))
	req.Header.Set("X-Nonce", nonce)

	return req
}

func TestUserWorkflow_CreateUpdateDelete(t *testing.T) {
	t.Parallel()
	// If a real agent is running, skip these local httptest-based E2E tests
	if checkAgentAvailable(t) {
		t.Skip("Local agent is running; these tests are intended to run against the test server")
	}

	server := setupE2EServer(t)
	defer server.Close()

	listReq := makeE2EAuthenticatedRequest(t, "GET", server.URL+"/inbounds/vless-reality/users", nil)
	listResp, err := http.DefaultClient.Do(listReq)
	require.NoError(t, err)
	defer func() { _ = listResp.Body.Close() }()

	assert.Equal(t, http.StatusOK, listResp.StatusCode)

	var listResult handlers.Response
	err = json.NewDecoder(listResp.Body).Decode(&listResult)
	require.NoError(t, err)
	assert.True(t, listResult.Success)

	var users []models.User
	usersBytes, err := json.Marshal(listResult.Data)
	require.NoError(t, err)

	err = json.Unmarshal(usersBytes, &users)
	require.NoError(t, err)
	assert.Equal(t, 0, len(users), "Should start with no users")

	createUser := handlers.UserRequest{
		SubID:         "e2e-user-001",
		Email:         "e2e@example.com",
		Enabled:       func() *bool { b := true; return &b }(),
		UploadLimit:   1024 * 1024 * 1024, // 1GB
		DownloadLimit: 1024 * 1024 * 1024, // 1GB
	}

	createReq := makeE2EAuthenticatedRequest(t, "POST", server.URL+"/inbounds/vless-reality/users", createUser)
	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer func() { _ = createResp.Body.Close() }()

	assert.Equal(t, http.StatusCreated, createResp.StatusCode)

	var createResult handlers.Response
	err = json.NewDecoder(createResp.Body).Decode(&createResult)
	require.NoError(t, err)
	assert.True(t, createResult.Success)
	assert.NotNil(t, createResult.Data)

	listReq2 := makeE2EAuthenticatedRequest(t, "GET", server.URL+"/inbounds/vless-reality/users", nil)
	listResp2, err := http.DefaultClient.Do(listReq2)
	require.NoError(t, err)
	defer func() { _ = listResp2.Body.Close() }()

	var listResult2 handlers.Response
	err = json.NewDecoder(listResp2.Body).Decode(&listResult2)
	require.NoError(t, err)
	assert.True(t, listResult2.Success)

	var users2 []models.User
	usersBytes2, err := json.Marshal(listResult2.Data)
	require.NoError(t, err)

	err = json.Unmarshal(usersBytes2, &users2)
	require.NoError(t, err)
	assert.Equal(t, 1, len(users2), "Should have one user after creation")
	assert.Equal(t, "e2e-user-001", users2[0].SubID)

	updatedUser := handlers.UserRequest{
		SubID:   "e2e-user-001",
		Email:   "updated-e2e@example.com",
		Enabled: func() *bool { b := false; return &b }(),
	}

	updateReq := makeE2EAuthenticatedRequest(t, "PUT", server.URL+"/inbounds/vless-reality/users/e2e-user-001", updatedUser)
	updateResp, err := http.DefaultClient.Do(updateReq)
	require.NoError(t, err)
	defer func() { _ = updateResp.Body.Close() }()

	assert.Equal(t, http.StatusOK, updateResp.StatusCode)

	var updateResult handlers.Response
	err = json.NewDecoder(updateResp.Body).Decode(&updateResult)
	require.NoError(t, err)
	assert.True(t, updateResult.Success)

	getReq := makeE2EAuthenticatedRequest(t, "GET", server.URL+"/inbounds/vless-reality/users/e2e-user-001", nil)
	getResp, err := http.DefaultClient.Do(getReq)
	require.NoError(t, err)
	defer func() { _ = getResp.Body.Close() }()

	assert.Equal(t, http.StatusOK, getResp.StatusCode)

	var getResult handlers.Response
	err = json.NewDecoder(getResp.Body).Decode(&getResult)
	require.NoError(t, err)
	assert.True(t, getResult.Success)

	var updatedUserModel models.User
	userBytes, err := json.Marshal(getResult.Data)
	require.NoError(t, err)

	err = json.Unmarshal(userBytes, &updatedUserModel)
	require.NoError(t, err)
	assert.Equal(t, "updated-e2e@example.com", updatedUserModel.Email)
	assert.False(t, updatedUserModel.Enabled)

	deleteReq := makeE2EAuthenticatedRequest(t, "DELETE", server.URL+"/inbounds/vless-reality/users/e2e-user-001", nil)
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	require.NoError(t, err)
	defer func() { _ = deleteResp.Body.Close() }()

	assert.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	listReq3 := makeE2EAuthenticatedRequest(t, "GET", server.URL+"/inbounds/vless-reality/users", nil)
	listResp3, err := http.DefaultClient.Do(listReq3)
	require.NoError(t, err)
	defer func() { _ = listResp3.Body.Close() }()

	var listResult3 handlers.Response
	err = json.NewDecoder(listResp3.Body).Decode(&listResult3)
	require.NoError(t, err)
	assert.True(t, listResult3.Success)

	var users3 []models.User
	usersBytes3, err := json.Marshal(listResult3.Data)
	require.NoError(t, err)

	err = json.Unmarshal(usersBytes3, &users3)
	require.NoError(t, err)
	assert.Equal(t, 0, len(users3), "Should have no users after deletion")
}

func TestUserWorkflow_BatchOperations(t *testing.T) {
	t.Parallel()

	if checkAgentAvailable(t) {
		t.Skip("Agent running; skipping test server-based E2E test")
	}

	server := setupE2EServer(t)
	defer server.Close()

	numUsers := 5
	for i := 0; i < numUsers; i++ {
		createUser := handlers.UserRequest{
			SubID:         "batch-user-" + strconv.Itoa(i),
			Email:         "batch" + strconv.Itoa(i) + "@example.com",
			Enabled:       func() *bool { b := true; return &b }(),
			UploadLimit:   512 * 1024 * 1024, // 512MB
			DownloadLimit: 512 * 1024 * 1024, // 512MB
		}

		createReq := makeE2EAuthenticatedRequest(t, "POST", server.URL+"/inbounds/vless-reality/users", createUser)
		createResp, err := http.DefaultClient.Do(createReq)
		require.NoError(t, err)
		_ = createResp.Body.Close()

		assert.Equal(t, http.StatusCreated, createResp.StatusCode)
	}

	listReq := makeE2EAuthenticatedRequest(t, "GET", server.URL+"/inbounds/vless-reality/users", nil)
	listResp, err := http.DefaultClient.Do(listReq)
	require.NoError(t, err)
	defer func() { _ = listResp.Body.Close() }()

	var listResult handlers.Response
	err = json.NewDecoder(listResp.Body).Decode(&listResult)
	require.NoError(t, err)
	assert.True(t, listResult.Success)

	var users []models.User
	usersBytes, err := json.Marshal(listResult.Data)
	require.NoError(t, err)

	err = json.Unmarshal(usersBytes, &users)
	require.NoError(t, err)
	assert.Equal(t, numUsers, len(users), "Should have all created users")

	for i := numUsers - 1; i >= 0; i-- {
		deleteReq := makeE2EAuthenticatedRequest(t, "DELETE", server.URL+"/inbounds/vless-reality/users/batch-user-"+strconv.Itoa(i), nil)
		deleteResp, err := http.DefaultClient.Do(deleteReq)
		require.NoError(t, err)
		_ = deleteResp.Body.Close()

		assert.Equal(t, http.StatusNoContent, deleteResp.StatusCode)
	}

	listReq2 := makeE2EAuthenticatedRequest(t, "GET", server.URL+"/inbounds/vless-reality/users", nil)
	listResp2, err := http.DefaultClient.Do(listReq2)
	require.NoError(t, err)
	defer func() { _ = listResp2.Body.Close() }()

	var listResult2 handlers.Response
	err = json.NewDecoder(listResp2.Body).Decode(&listResult2)
	require.NoError(t, err)
	assert.True(t, listResult2.Success)

	var users2 []models.User
	usersBytes2, err := json.Marshal(listResult2.Data)
	require.NoError(t, err)

	err = json.Unmarshal(usersBytes2, &users2)
	require.NoError(t, err)
	assert.Equal(t, 0, len(users2), "Should have no users after batch deletion")
}

func TestUserWorkflow_ConcurrentOperations(t *testing.T) {
	t.Parallel()

	if checkAgentAvailable(t) {
		t.Skip("Agent running; skipping test server-based E2E test")
	}

	server := setupE2EServer(t)
	defer server.Close()

	numOperations := 10
	done := make(chan bool, numOperations)

	for i := 0; i < numOperations; i++ {
		go func(index int) {
			createUser := handlers.UserRequest{
				SubID:         "concurrent-user-" + strconv.Itoa(index),
				Email:         "concurrent" + strconv.Itoa(index) + "@example.com",
				Enabled:       func() *bool { b := true; return &b }(),
				UploadLimit:   256 * 1024 * 1024, // 256MB
				DownloadLimit: 256 * 1024 * 1024, // 256MB
			}

			createReq := makeE2EAuthenticatedRequest(t, "POST", server.URL+"/inbounds/vless-reality/users", createUser)
			createResp, err := http.DefaultClient.Do(createReq)
			if err == nil {
				_ = createResp.Body.Close()
			}
			done <- true
		}(i)
	}

	for i := 0; i < numOperations; i++ {
		<-done
	}

	listReq := makeE2EAuthenticatedRequest(t, "GET", server.URL+"/inbounds/vless-reality/users", nil)
	listResp, err := http.DefaultClient.Do(listReq)
	require.NoError(t, err)
	defer func() { _ = listResp.Body.Close() }()

	var listResult handlers.Response
	err = json.NewDecoder(listResp.Body).Decode(&listResult)
	require.NoError(t, err)
	assert.True(t, listResult.Success)

	var users []models.User
	usersBytes, err := json.Marshal(listResult.Data)
	require.NoError(t, err)

	err = json.Unmarshal(usersBytes, &users)
	require.NoError(t, err)

	assert.Equal(t, numOperations, len(users), "Should have all concurrently created users")
}
