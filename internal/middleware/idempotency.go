package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/the-lpn-foundation/sing-box-agent/internal/store"
)

const (
	HeaderIdempotencyKey              = "Idempotency-Key"
	CodeIdempotencyConflict ErrorCode = "IDEMPOTENCY_CONFLICT"
)

func IdempotencyMiddleware(idempotencyStore *store.IdempotencyStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if idempotencyStore == nil || !isMutatingMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			key := strings.TrimSpace(r.Header.Get(HeaderIdempotencyKey))
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				writeIdempotencyConflict(w, "failed to read request body")
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			hash := computeRequestHash(r.Method, r.URL.Path, body)

			if storedHash, storedResponse, found := idempotencyStore.Get(key); found {
				if storedHash != hash {
					writeIdempotencyConflict(w, "idempotency key was reused with a different payload")
					return
				}

				writeStoredResponse(w, storedResponse)
				return
			}

			recorder := newResponseRecorder()
			next.ServeHTTP(recorder, r)

			storeErr := idempotencyStore.Store(key, hash, store.StoredResponse{
				StatusCode: recorder.statusCode,
				Headers:    recorder.header.Clone(),
				Body:       recorder.body.Bytes(),
			})

			if errors.Is(storeErr, store.ErrIdempotencyConflict) {
				writeIdempotencyConflict(w, "idempotency key was reused with a different payload")
				return
			}

			writeStoredResponse(w, store.StoredResponse{
				StatusCode: recorder.statusCode,
				Headers:    recorder.header,
				Body:       recorder.body.Bytes(),
			})
		})
	}
}

func isMutatingMethod(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
}

func computeRequestHash(method, path string, body []byte) string {
	hasher := sha256.New()
	hasher.Write([]byte(method))
	hasher.Write([]byte("\n"))
	hasher.Write([]byte(path))
	hasher.Write([]byte("\n"))
	hasher.Write(body)
	return hex.EncodeToString(hasher.Sum(nil))
}

func writeStoredResponse(w http.ResponseWriter, response store.StoredResponse) {
	for key, values := range response.Headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	statusCode := response.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}

	w.WriteHeader(statusCode)
	_, _ = w.Write(response.Body)
}

func writeIdempotencyConflict(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)

	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Success: false,
		Error: ErrorDetail{
			Code:    CodeIdempotencyConflict,
			Message: message,
		},
	})
}

type responseRecorder struct {
	header     http.Header
	body       bytes.Buffer
	statusCode int
	wroteHead  bool
}

func newResponseRecorder() *responseRecorder {
	return &responseRecorder{
		header:     make(http.Header),
		statusCode: http.StatusOK,
	}
}

func (r *responseRecorder) Header() http.Header {
	return r.header
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	if !r.wroteHead {
		r.WriteHeader(http.StatusOK)
	}
	return r.body.Write(data)
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	if r.wroteHead {
		return
	}
	r.wroteHead = true
	r.statusCode = statusCode
}
