//nolint:errcheck
package middleware

//nolint:errcheck // Test file uses type assertions

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/the-lpn-foundation/sing-box-agent/internal/store"
)

func TestIdempotencyMiddleware_NilStore(t *testing.T) {
	handler := IdempotencyMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"test":"data"}`))
	req.Header.Set(HeaderIdempotencyKey, "test-key")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"success":true}`, w.Body.String())
}

func TestIdempotencyMiddleware_NonMutatingMethods(t *testing.T) {
	methods := []string{http.MethodGet, http.MethodHead, http.MethodOptions}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			idempotencyStore := store.NewIdempotencyStore()

			handler := IdempotencyMiddleware(idempotencyStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"success":true}`))
			}))

			req := httptest.NewRequest(method, "/test", nil)
			req.Header.Set(HeaderIdempotencyKey, "test-key")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, `{"success":true}`, w.Body.String())

			// Verify nothing was stored
			_, _, found := idempotencyStore.Get("test-key")
			assert.False(t, found)
		})
	}
}

func TestIdempotencyMiddleware_MissingIdempotencyKey(t *testing.T) {
	idempotencyStore := store.NewIdempotencyStore()

	handler := IdempotencyMiddleware(idempotencyStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":123}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"test":"data"}`))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, `{"id":123}`, w.Body.String())
}

func TestIdempotencyMiddleware_EmptyIdempotencyKey(t *testing.T) {
	idempotencyStore := store.NewIdempotencyStore()

	handler := IdempotencyMiddleware(idempotencyStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":123}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"test":"data"}`))
	req.Header.Set(HeaderIdempotencyKey, "   ")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, `{"id":123}`, w.Body.String())
}

func TestIdempotencyMiddleware_FirstRequest(t *testing.T) {
	idempotencyStore := store.NewIdempotencyStore()

	handler := IdempotencyMiddleware(idempotencyStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Header().Set("X-Custom-Header", "custom-value")
		_, _ = w.Write([]byte(`{"id":123}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"test":"data"}`))
	req.Header.Set(HeaderIdempotencyKey, "test-key")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "custom-value", w.Header().Get("X-Custom-Header"))
	assert.Equal(t, `{"id":123}`, w.Body.String())

	// Verify response was stored
	storedHash, storedResponse, found := idempotencyStore.Get("test-key")
	assert.True(t, found)
	assert.NotEmpty(t, storedHash)
	assert.Equal(t, http.StatusCreated, storedResponse.StatusCode)
	assert.Equal(t, "custom-value", storedResponse.Headers.Get("X-Custom-Header"))
	assert.Equal(t, []byte(`{"id":123}`), storedResponse.Body)
}

func TestIdempotencyMiddleware_SecondRequestSameHash(t *testing.T) {
	idempotencyStore := store.NewIdempotencyStore()

	// First request
	handler := IdempotencyMiddleware(idempotencyStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Header().Set("X-Custom-Header", "custom-value")
		_, _ = w.Write([]byte(`{"id":123}`))
	}))

	req1 := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"test":"data"}`))
	req1.Header.Set(HeaderIdempotencyKey, "test-key")
	w1 := httptest.NewRecorder()

	handler.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusCreated, w1.Code)
	assert.Equal(t, `{"id":123}`, w1.Body.String())

	// Second request with same key and same body
	req2 := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"test":"data"}`))
	req2.Header.Set(HeaderIdempotencyKey, "test-key")
	w2 := httptest.NewRecorder()

	handler.ServeHTTP(w2, req2)

	// Should return cached response
	assert.Equal(t, http.StatusCreated, w2.Code)
	assert.Equal(t, "custom-value", w2.Header().Get("X-Custom-Header"))
	assert.Equal(t, `{"id":123}`, w2.Body.String())
}

func TestIdempotencyMiddleware_SecondRequestDifferentHash(t *testing.T) {
	idempotencyStore := store.NewIdempotencyStore()

	// First request
	handler := IdempotencyMiddleware(idempotencyStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":123}`))
	}))

	req1 := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"test":"data"}`))
	req1.Header.Set(HeaderIdempotencyKey, "test-key")
	w1 := httptest.NewRecorder()

	handler.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusCreated, w1.Code)

	// Second request with same key but different body
	req2 := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"test":"different"}`))
	req2.Header.Set(HeaderIdempotencyKey, "test-key")
	w2 := httptest.NewRecorder()

	handler.ServeHTTP(w2, req2)

	// Should return conflict
	assert.Equal(t, http.StatusConflict, w2.Code)

	var resp ErrorResponse
	err := json.NewDecoder(w2.Body).Decode(&resp)
	require.NoError(t, err)

	assert.False(t, resp.Success)
	assert.Equal(t, CodeIdempotencyConflict, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "idempotency key was reused with a different payload")
}

func TestIdempotencyMiddleware_StoreConflictError(t *testing.T) {
	// This test verifies the conflict error path when the same key is reused
	// with a different payload. The actual conflict is tested in
	// TestIdempotencyMiddleware_SecondRequestDifferentHash.
	// Here we just verify that the error handling path exists.

	idempotencyStore := store.NewIdempotencyStore()

	handler := IdempotencyMiddleware(idempotencyStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":123}`))
	}))

	// First request
	req1 := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"test":"data"}`))
	req1.Header.Set(HeaderIdempotencyKey, "test-key")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusCreated, w1.Code)

	// Second request with same key but different body - should return conflict
	req2 := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"test":"different"}`))
	req2.Header.Set(HeaderIdempotencyKey, "test-key")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusConflict, w2.Code)

	var resp ErrorResponse
	err := json.NewDecoder(w2.Body).Decode(&resp)
	require.NoError(t, err)

	assert.False(t, resp.Success)
	assert.Equal(t, CodeIdempotencyConflict, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "idempotency key was reused with a different payload")
}

func TestIdempotencyMiddleware_BodyReadError(t *testing.T) {
	idempotencyStore := store.NewIdempotencyStore()

	handler := IdempotencyMiddleware(idempotencyStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Create a request body that will fail to read
	req := httptest.NewRequest(http.MethodPost, "/test", &errorReader{})
	req.Header.Set(HeaderIdempotencyKey, "test-key")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should return conflict
	assert.Equal(t, http.StatusConflict, w.Code)

	var resp ErrorResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	assert.False(t, resp.Success)
	assert.Equal(t, CodeIdempotencyConflict, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "failed to read request body")
}

func TestIdempotencyMiddleware_BodyRestored(t *testing.T) {
	idempotencyStore := store.NewIdempotencyStore()

	bodyRead := false
	handler := IdempotencyMiddleware(idempotencyStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to read the body again - it should still be readable
		data, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		bodyRead = true
		assert.Equal(t, `{"test":"data"}`, string(data))
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":123}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"test":"data"}`))
	req.Header.Set(HeaderIdempotencyKey, "test-key")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.True(t, bodyRead, "Body should be readable by downstream handler")
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestIsMutatingMethod(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		expected bool
	}{
		{"POST", http.MethodPost, true},
		{"PUT", http.MethodPut, true},
		{"PATCH", http.MethodPatch, true},
		{"DELETE", http.MethodDelete, true},
		{"GET", http.MethodGet, false},
		{"HEAD", http.MethodHead, false},
		{"OPTIONS", http.MethodOptions, false},
		{"TRACE", http.MethodTrace, false},
		{"CONNECT", http.MethodConnect, false},
		{"custom", "CUSTOM", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isMutatingMethod(tt.method))
		})
	}
}

func TestComputeRequestHash(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		body     []byte
		expected string
	}{
		{
			name:     "POST with body",
			method:   http.MethodPost,
			path:     "/api/users",
			body:     []byte(`{"name":"test"}`),
			expected: "a1b2c3d4e5f6", // placeholder - actual hash will be computed
		},
		{
			name:     "GET without body",
			method:   http.MethodGet,
			path:     "/api/users",
			body:     nil,
			expected: "f6e5d4c3b2a1", // placeholder
		},
		{
			name:     "empty body",
			method:   http.MethodPost,
			path:     "/api/users",
			body:     []byte{},
			expected: "emptybodyhash", // placeholder
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := computeRequestHash(tt.method, tt.path, tt.body)
			assert.NotEmpty(t, hash)
			assert.Len(t, hash, 64) // SHA256 produces 64 hex characters

			// Same inputs should produce same hash
			hash2 := computeRequestHash(tt.method, tt.path, tt.body)
			assert.Equal(t, hash, hash2)
		})
	}
}

func TestComputeRequestHash_Deterministic(t *testing.T) {
	method := http.MethodPost
	path := "/api/test"
	body := []byte(`{"test":"data"}`)

	hash1 := computeRequestHash(method, path, body)
	hash2 := computeRequestHash(method, path, body)
	hash3 := computeRequestHash(method, path, body)

	assert.Equal(t, hash1, hash2)
	assert.Equal(t, hash2, hash3)
}

func TestComputeRequestHash_DifferentInputs(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{"different method", http.MethodPut, "/api/test", []byte(`{"test":"data"}`)},
		{"different path", http.MethodPost, "/api/other", []byte(`{"test":"data"}`)},
		{"different body", http.MethodPost, "/api/test", []byte(`{"test":"other"}`)},
	}

	baseHash := computeRequestHash(http.MethodPost, "/api/test", []byte(`{"test":"data"}`))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := computeRequestHash(tt.method, tt.path, tt.body)
			assert.NotEqual(t, baseHash, hash)
		})
	}
}

func TestWriteStoredResponse(t *testing.T) {
	tests := []struct {
		name       string
		response   store.StoredResponse
		wantStatus int
		wantBody   string
		wantHeader string
	}{
		{
			name: "full response",
			response: store.StoredResponse{
				StatusCode: http.StatusCreated,
				Headers: http.Header{
					"Content-Type": []string{"application/json"},
					"X-Custom":     []string{"custom-value"},
				},
				Body: []byte(`{"id":123}`),
			},
			wantStatus: http.StatusCreated,
			wantBody:   `{"id":123}`,
			wantHeader: "custom-value",
		},
		{
			name: "zero status code defaults to 200",
			response: store.StoredResponse{
				StatusCode: 0,
				Headers:    http.Header{},
				Body:       []byte(`{"success":true}`),
			},
			wantStatus: http.StatusOK,
			wantBody:   `{"success":true}`,
		},
		{
			name: "empty body",
			response: store.StoredResponse{
				StatusCode: http.StatusNoContent,
				Headers:    http.Header{},
				Body:       []byte{},
			},
			wantStatus: http.StatusNoContent,
			wantBody:   "",
		},
		{
			name: "multiple header values",
			response: store.StoredResponse{
				StatusCode: http.StatusOK,
				Headers: http.Header{
					"Set-Cookie": []string{"cookie1=value1", "cookie2=value2"},
				},
				Body: []byte(`{"success":true}`),
			},
			wantStatus: http.StatusOK,
			wantBody:   `{"success":true}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeStoredResponse(w, tt.response)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantBody, w.Body.String())

			if tt.wantHeader != "" {
				assert.Equal(t, tt.wantHeader, w.Header().Get("X-Custom"))
			}
		})
	}
}

func TestWriteIdempotencyConflict(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{
			name:    "standard conflict message",
			message: "idempotency key was reused with a different payload",
		},
		{
			name:    "custom conflict message",
			message: "custom conflict message",
		},
		{
			name:    "empty message",
			message: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeIdempotencyConflict(w, tt.message)

			assert.Equal(t, http.StatusConflict, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

			var resp ErrorResponse
			err := json.NewDecoder(w.Body).Decode(&resp)
			require.NoError(t, err)

			assert.False(t, resp.Success)
			assert.Equal(t, CodeIdempotencyConflict, resp.Error.Code)
			assert.Equal(t, tt.message, resp.Error.Message)
		})
	}
}

func TestResponseRecorder_NewResponseRecorder(t *testing.T) {
	recorder := newResponseRecorder()

	assert.NotNil(t, recorder)
	assert.NotNil(t, recorder.header)
	assert.NotNil(t, recorder.body)
	assert.Equal(t, http.StatusOK, recorder.statusCode)
	assert.False(t, recorder.wroteHead)
}

func TestResponseRecorder_Header(t *testing.T) {
	recorder := newResponseRecorder()

	header := recorder.Header()
	assert.NotNil(t, header)

	header.Set("X-Custom", "value")

	// Should be the same header instance
	assert.Equal(t, "value", recorder.Header().Get("X-Custom"))
}

func TestResponseRecorder_Write(t *testing.T) {
	tests := []struct {
		name         string
		data         []byte
		wantStatus   int
		wantBody     string
		wroteHeadSet bool
	}{
		{
			name:         "write without explicit WriteHeader",
			data:         []byte("test data"),
			wantStatus:   http.StatusOK,
			wantBody:     "test data",
			wroteHeadSet: true,
		},
		{
			name:         "write after WriteHeader",
			data:         []byte("test data"),
			wantStatus:   http.StatusCreated,
			wantBody:     "test data",
			wroteHeadSet: true,
		},
		{
			name:         "multiple writes",
			data:         []byte("test data"),
			wantStatus:   http.StatusOK,
			wantBody:     "test datatest data",
			wroteHeadSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := newResponseRecorder()

			if tt.wantStatus != http.StatusOK {
				recorder.WriteHeader(tt.wantStatus)
			}

			n, err := recorder.Write(tt.data)
			require.NoError(t, err)
			assert.Equal(t, len(tt.data), n)

			if tt.name == "multiple writes" {
				n, err = recorder.Write(tt.data)
				require.NoError(t, err)
				assert.Equal(t, len(tt.data), n)
			}

			assert.Equal(t, tt.wroteHeadSet, recorder.wroteHead)
			assert.Equal(t, tt.wantBody, recorder.body.String())
		})
	}
}

func TestResponseRecorder_WriteHeader(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"OK", http.StatusOK},
		{"Created", http.StatusCreated},
		{"No Content", http.StatusNoContent},
		{"Internal Server Error", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := newResponseRecorder()

			recorder.WriteHeader(tt.statusCode)

			assert.True(t, recorder.wroteHead)
			assert.Equal(t, tt.statusCode, recorder.statusCode)
		})
	}
}

func TestResponseRecorder_WriteHeader_Idempotent(t *testing.T) {
	recorder := newResponseRecorder()

	recorder.WriteHeader(http.StatusCreated)
	assert.Equal(t, http.StatusCreated, recorder.statusCode)

	// Second call should be ignored
	recorder.WriteHeader(http.StatusInternalServerError)
	assert.Equal(t, http.StatusCreated, recorder.statusCode)
}

func TestResponseRecorder_WriteHeader_AfterWrite(t *testing.T) {
	recorder := newResponseRecorder()

	recorder.Write([]byte("data"))
	assert.Equal(t, http.StatusOK, recorder.statusCode)

	// WriteHeader after Write should be ignored
	recorder.WriteHeader(http.StatusCreated)
	assert.Equal(t, http.StatusOK, recorder.statusCode)
}

func TestIdempotencyMiddleware_PUT(t *testing.T) {
	idempotencyStore := store.NewIdempotencyStore()

	handler := IdempotencyMiddleware(idempotencyStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"updated":true}`))
	}))

	req := httptest.NewRequest(http.MethodPut, "/test", strings.NewReader(`{"test":"data"}`))
	req.Header.Set(HeaderIdempotencyKey, "test-key")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"updated":true}`, w.Body.String())

	// Verify response was stored
	_, _, found := idempotencyStore.Get("test-key")
	assert.True(t, found)
}

func TestIdempotencyMiddleware_PATCH(t *testing.T) {
	idempotencyStore := store.NewIdempotencyStore()

	handler := IdempotencyMiddleware(idempotencyStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"patched":true}`))
	}))

	req := httptest.NewRequest(http.MethodPatch, "/test", strings.NewReader(`{"test":"data"}`))
	req.Header.Set(HeaderIdempotencyKey, "test-key")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"patched":true}`, w.Body.String())

	// Verify response was stored
	_, _, found := idempotencyStore.Get("test-key")
	assert.True(t, found)
}

func TestIdempotencyMiddleware_DELETE(t *testing.T) {
	idempotencyStore := store.NewIdempotencyStore()

	handler := IdempotencyMiddleware(idempotencyStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodDelete, "/test", nil)
	req.Header.Set(HeaderIdempotencyKey, "test-key")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify response was stored
	_, _, found := idempotencyStore.Get("test-key")
	assert.True(t, found)
}

func TestIdempotencyMiddleware_DifferentPaths(t *testing.T) {
	idempotencyStore := store.NewIdempotencyStore()

	handler := IdempotencyMiddleware(idempotencyStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"path":"` + r.URL.Path + `"}`))
	}))

	// First request to /api/users
	req1 := httptest.NewRequest(http.MethodPost, "/api/users", strings.NewReader(`{"name":"test"}`))
	req1.Header.Set(HeaderIdempotencyKey, "key-1")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusCreated, w1.Code)
	assert.Contains(t, w1.Body.String(), "/api/users")

	// Second request to /api/posts with same key but different path
	req2 := httptest.NewRequest(http.MethodPost, "/api/posts", strings.NewReader(`{"name":"test"}`))
	req2.Header.Set(HeaderIdempotencyKey, "key-1")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	// Should return conflict because hash is different (different path)
	assert.Equal(t, http.StatusConflict, w2.Code)
}

func TestIdempotencyMiddleware_ResponseHeadersPreserved(t *testing.T) {
	idempotencyStore := store.NewIdempotencyStore()

	handler := IdempotencyMiddleware(idempotencyStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "12345")
		w.Header().Add("Set-Cookie", "session=abc")
		w.Header().Add("Set-Cookie", "token=xyz")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":123}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"test":"data"}`))
	req.Header.Set(HeaderIdempotencyKey, "test-key")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Equal(t, "12345", w.Header().Get("X-Request-ID"))

	cookies := w.Header().Values("Set-Cookie")
	assert.Len(t, cookies, 2)
	assert.Contains(t, cookies, "session=abc")
	assert.Contains(t, cookies, "token=xyz")
}

func TestIdempotencyMiddleware_CachedResponseHeadersPreserved(t *testing.T) {
	idempotencyStore := store.NewIdempotencyStore()

	handler := IdempotencyMiddleware(idempotencyStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "12345")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":123}`))
	}))

	// First request
	req1 := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"test":"data"}`))
	req1.Header.Set(HeaderIdempotencyKey, "test-key")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusCreated, w1.Code)
	assert.Equal(t, "12345", w1.Header().Get("X-Request-ID"))

	// Second request - should return cached response with headers
	req2 := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"test":"data"}`))
	req2.Header.Set(HeaderIdempotencyKey, "test-key")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusCreated, w2.Code)
	assert.Equal(t, "12345", w2.Header().Get("X-Request-ID"))
	assert.Equal(t, `{"id":123}`, w2.Body.String())
}

// Mock implementations

type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("read error")
}
