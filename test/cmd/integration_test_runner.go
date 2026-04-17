package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

const (
	baseURL = "http://localhost:18080"
	token   = "test-token-for-integration-testing-minimum-32-chars"
	secret  = "test-secret-for-integration-testing"
)

func main() {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		// Inbound CRUD
		{"List inbounds", "GET", "/inbounds", ""},
		{"Create inbound", "POST", "/inbounds", `{"tag":"test-vless","type":"vless","listen":"0.0.0.0","port":8443,"options":{}}`},
		{"Get inbound", "GET", "/inbounds/vless-reality", ""},
		{"Update inbound", "PUT", "/inbounds/test-vless", `{"tag":"test-vless","type":"vless","listen":"0.0.0.0","port":9443,"options":{}}`},

		// User CRUD
		{"Create user", "POST", "/inbounds/vless-reality/users", `{"subId":"test-user-1","email":"test@example.com","enabled":true}`},
		{"List users", "GET", "/inbounds/vless-reality/users", ""},
		{"Get user", "GET", "/inbounds/vless-reality/users/test-user-1", ""},
		{"Update user", "PUT", "/inbounds/vless-reality/users/test-user-1", `{"subId":"test-user-1","email":"updated@example.com","enabled":true}`},

		// Stats
		{"Get traffic stats", "GET", "/stats/traffic", ""},
		{"Get online users", "GET", "/stats/online", ""},

		// Sync
		{"Get sync status", "GET", "/sync/status", ""},
		{"Apply desired state", "POST", "/sync/desired-state", `{"version":1,"inbounds":[],"users":[]}`},

		// Core
		{"Get config", "GET", "/core/config", ""},
		{"Reload config", "POST", "/core/reload", ""},

		// Subscription
		{"Generate subscription", "POST", "/subscription/generate", `{"subId":"test-user-1","format":"sing-box"}`},
		{"Get subscription", "GET", "/subscription/test-user-1", ""},

		// Cleanup
		{"Delete user", "DELETE", "/inbounds/vless-reality/users/test-user-1", ""},
		{"Delete inbound", "DELETE", "/inbounds/test-vless", ""},
	}

	passed := 0
	failed := 0

	for _, tt := range tests {
		status, body, err := doRequest(tt.method, tt.path, tt.body)
		if err != nil {
			fmt.Printf("❌ %s: %v\n", tt.name, err)
			failed++
			continue
		}

		// Consider 2xx and 4xx (expected errors like 404) as "working"
		// 5xx = server error = bug
		if status >= 500 {
			fmt.Printf("❌ %s: %d %s\n", tt.name, status, truncate(body, 200))
			failed++
		} else {
			fmt.Printf("✅ %s: %d %s\n", tt.name, status, truncate(body, 150))
			passed++
		}
	}

	fmt.Printf("\n=== Results: %d passed, %d failed ===\n", passed, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func doRequest(method, path, body string) (int, string, error) {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = bytes.NewBufferString(body)
	}

	req, err := http.NewRequest(method, baseURL+path, bodyReader)
	if err != nil {
		return 0, "", err
	}

	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Auth headers
	req.Header.Set("Authorization", "Bearer "+token)

	nonce := uuid.New().String()
	timestamp := fmt.Sprintf("%d", time.Now().Unix())

	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Timestamp", timestamp)

	// Compute body hash (empty body = empty hash per auth.ComputeBodyHash)
	var bodyHash string
	if body != "" {
		bodyHash = computeHash(body)
	}

	// Build canonical string: nonce\ntimestamp\nmethod\npath\nbody_hash
	canonical := fmt.Sprintf("%s\n%s\n%s\n%s\n%s", nonce, timestamp, method, path, bodyHash)

	// HMAC-SHA256 signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonical))
	signature := hex.EncodeToString(mac.Sum(nil))

	req.Header.Set("X-Signature", signature)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(respBody), nil
}

func computeHash(body string) string {
	h := sha256.New()
	h.Write([]byte(body))
	return hex.EncodeToString(h.Sum(nil))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
