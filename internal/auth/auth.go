package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrInvalidToken is returned when the bearer token is invalid.
	ErrInvalidToken = errors.New("invalid bearer token")

	// ErrInvalidSignature is returned when the signature verification fails.
	ErrInvalidSignature = errors.New("invalid signature")

	// ErrMissingAuthHeader is returned when the Authorization header is missing.
	ErrMissingAuthHeader = errors.New("missing authorization header")

	// ErrMalformedAuthHeader is returned when the Authorization header is malformed.
	ErrMalformedAuthHeader = errors.New("malformed authorization header")
)

// ValidateBearerToken validates the provided bearer token against the expected token.
// Returns ErrInvalidToken if the token does not match.
func ValidateBearerToken(providedToken, expectedToken string) error {
	if providedToken == "" {
		return ErrMissingAuthHeader
	}
	// Constant-time comparison to prevent timing attacks on the token.
	if subtle.ConstantTimeCompare([]byte(providedToken), []byte(expectedToken)) != 1 {
		return ErrInvalidToken
	}
	return nil
}

// ExtractBearerToken extracts the bearer token from the Authorization header value.
// The header value should be in the format "Bearer <token>".
// Returns ErrMissingAuthHeader if the header is empty, ErrMalformedAuthHeader if malformed.
func ExtractBearerToken(authHeader string) (string, error) {
	if authHeader == "" {
		return "", ErrMissingAuthHeader
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return "", ErrMalformedAuthHeader
	}

	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", ErrInvalidToken
	}

	return token, nil
}

// BuildCanonicalString constructs the canonical string for request signing.
// Format: {nonce}\n{timestamp}\n{method}\n{path}\n{body_hash}
func BuildCanonicalString(nonce, timestamp, method, path, bodyHash string) string {
	return fmt.Sprintf("%s\n%s\n%s\n%s\n%s", nonce, timestamp, method, path, bodyHash)
}

// ComputeBodyHash computes the SHA256 hash of the request body.
// Returns hex-encoded hash string.
func ComputeBodyHash(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	h := sha256.New()
	_, _ = h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// VerifySignature verifies the HMAC-SHA256 signature of the canonical string.
// The signature is computed as HMAC-SHA256(canonical_string, secret).hexdigest()
// Returns ErrInvalidSignature if verification fails.
func VerifySignature(canonicalString, signature, secret string) error {
	if canonicalString == "" || signature == "" || secret == "" {
		return ErrInvalidSignature
	}

	// Create HMAC-SHA256 hash
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonicalString))

	// Compute expected signature
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	// Constant-time comparison to prevent timing attacks
	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return ErrInvalidSignature
	}

	return nil
}

// SignRequest generates an HMAC-SHA256 signature for the canonical string.
// This is primarily used for testing and client-side signing.
func SignRequest(canonicalString, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonicalString))
	return hex.EncodeToString(mac.Sum(nil))
}
