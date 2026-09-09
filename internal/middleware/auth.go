package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/the-lpn-foundation/sing-box-agent/internal/auth"
)

const (
	// HeaderAuthorization is the Authorization header key.
	HeaderAuthorization = "Authorization"

	// HeaderXSignature is the X-Signature header key.
	HeaderXSignature = "X-Signature"

	// HeaderXTimestamp is the X-Timestamp header key.
	HeaderXTimestamp = "X-Timestamp"

	// HeaderXNonce is the X-Nonce header key.
	HeaderXNonce = "X-Nonce"
)

// maxAuthBodySize is the maximum allowed request body size for
// authenticated requests (10 MiB). Bodies larger than this are rejected.
const maxAuthBodySize = 10 << 20

// ErrorCode represents an API error code.
type ErrorCode string

const (
	// CodeUnauthorized is the error code for unauthorized requests.
	CodeUnauthorized ErrorCode = "UNAUTHORIZED"

	// CodeReplayDetected is the error code for replay attacks.
	CodeReplayDetected ErrorCode = "REPLAY_DETECTED"
)

// ErrorResponse is the standard error response format.
type ErrorResponse struct {
	Success bool        `json:"success"`
	Error   ErrorDetail `json:"error"`
}

// ErrorDetail contains error information.
type ErrorDetail struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

// AuthConfig holds configuration for the auth middleware.
type AuthConfig struct {
	Token      string
	Secret     string
	NonceCache *auth.NonceCache
	Logger     *slog.Logger
}

// AuthMiddleware creates an authentication middleware that validates bearer tokens,
// verifies HMAC-SHA256 signatures, and protects against replay attacks.
func AuthMiddleware(config AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authErr := authenticateRequest(config, r)

			if authErr != nil {
				sendErrorResponse(w, authErr, config.Logger)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// authenticateRequest validates all authentication requirements for a request.
func authenticateRequest(config AuthConfig, r *http.Request) error {
	// Extract and validate bearer token
	token, err := auth.ExtractBearerToken(r.Header.Get(HeaderAuthorization))
	if err != nil {
		return &AuthError{
			Code:    CodeUnauthorized,
			Message: fmt.Sprintf("Invalid authorization header: %v", err),
			Err:     err,
		}
	}

	if err := auth.ValidateBearerToken(token, config.Token); err != nil {
		return &AuthError{
			Code:    CodeUnauthorized,
			Message: "Invalid bearer token",
			Err:     err,
		}
	}

	// Extract required headers
	signature := r.Header.Get(HeaderXSignature)
	timestampStr := r.Header.Get(HeaderXTimestamp)
	nonce := r.Header.Get(HeaderXNonce)

	if signature == "" {
		return &AuthError{
			Code:    CodeUnauthorized,
			Message: "Missing X-Signature header",
			Err:     errors.New("missing signature header"),
		}
	}

	if timestampStr == "" {
		return &AuthError{
			Code:    CodeUnauthorized,
			Message: "Missing X-Timestamp header",
			Err:     errors.New("missing timestamp header"),
		}
	}

	if nonce == "" {
		return &AuthError{
			Code:    CodeUnauthorized,
			Message: "Missing X-Nonce header",
			Err:     errors.New("missing nonce header"),
		}
	}

	// Parse timestamp
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return &AuthError{
			Code:    CodeUnauthorized,
			Message: "Invalid X-Timestamp format",
			Err:     fmt.Errorf("invalid timestamp: %w", err),
		}
	}

	// Check timestamp skew
	timestampTime := auth.ParseTimestamp(timestamp)
	if err := auth.CheckTimestampSkew(timestampTime); err != nil {
		return &AuthError{
			Code:    CodeUnauthorized,
			Message: fmt.Sprintf("Timestamp skew exceeded: %v", err),
			Err:     err,
		}
	}

	// Read request body for hash computation (with size limit to prevent
	// memory exhaustion from oversized bodies).
	body, err := io.ReadAll(io.LimitReader(r.Body, maxAuthBodySize+1))
	if err != nil {
		return &AuthError{
			Code:    CodeUnauthorized,
			Message: "Failed to read request body",
			Err:     fmt.Errorf("read body: %w", err),
		}
	}

	if len(body) > maxAuthBodySize {
		return &AuthError{
			Code:    CodeUnauthorized,
			Message: "Request body too large",
		}
	}

	// Restore body for downstream handlers
	r.Body = io.NopCloser(bytes.NewReader(body))

	// Compute body hash
	bodyHash := auth.ComputeBodyHash(body)

	// Build canonical string
	canonicalString := auth.BuildCanonicalString(
		nonce,
		timestampStr,
		r.Method,
		r.URL.Path,
		bodyHash,
	)

	// Verify signature
	if err := auth.VerifySignature(canonicalString, signature, config.Secret); err != nil {
		return &AuthError{
			Code:    CodeUnauthorized,
			Message: "Invalid signature",
			Err:     err,
		}
	}

	// Check nonce for replay attack. This must be the LAST step, after the
	// signature is verified, so that requests with invalid signatures cannot
	// burn nonce-cache entries and open eviction-based replay windows.
	if err := config.NonceCache.CheckAndStoreNonce(nonce); err != nil {
		return &AuthError{
			Code:    CodeReplayDetected,
			Message: fmt.Sprintf("Replay attack detected: %v", err),
			Err:     err,
		}
	}

	return nil
}

// AuthError represents an authentication error with an associated error code.
type AuthError struct {
	Code    ErrorCode
	Message string
	Err     error
}

// Error implements the error interface.
func (ae *AuthError) Error() string {
	if ae.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", ae.Code, ae.Message, ae.Err)
	}
	return fmt.Sprintf("[%s] %s", ae.Code, ae.Message)
}

// Unwrap returns the underlying error.
func (ae *AuthError) Unwrap() error {
	return ae.Err
}

// sendErrorResponse writes an error response to the client.
func sendErrorResponse(w http.ResponseWriter, err error, logger *slog.Logger) {
	var authErr *AuthError
	statusCode := http.StatusUnauthorized

	if errors.As(err, &authErr) {
		if authErr.Code == CodeReplayDetected {
			logger.LogAttrs(context.TODO(), slog.LevelWarn, "auth failed",
				slog.String("code", string(authErr.Code)),
				slog.String("message", authErr.Message),
			)
		} else {
			logger.LogAttrs(context.TODO(), slog.LevelInfo, "auth failed",
				slog.String("code", string(authErr.Code)),
				slog.String("message", authErr.Message),
			)
		}
	} else {
		logger.LogAttrs(context.TODO(), slog.LevelError, "unexpected auth error",
			slog.String("error", err.Error()),
		)
		authErr = &AuthError{
			Code:    CodeUnauthorized,
			Message: "Authentication failed",
			Err:     err,
		}
	}

	response := ErrorResponse{
		Success: false,
		Error: ErrorDetail{
			Code:    authErr.Code,
			Message: authErr.Message,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.LogAttrs(context.TODO(), slog.LevelError, "failed to write error response",
			slog.String("error", err.Error()),
		)
	}
}

// IsUnauthorized checks if an error is an authentication error.
func IsUnauthorized(err error) bool {
	var authErr *AuthError
	return errors.As(err, &authErr)
}

// GetErrorCode extracts the error code from an authentication error.
func GetErrorCode(err error) ErrorCode {
	var authErr *AuthError
	if errors.As(err, &authErr) {
		return authErr.Code
	}
	return CodeUnauthorized
}
