package e2e

import (
	"net/http"
	"testing"
)

// checkAgentAvailable attempts to contact the local agent health endpoint.
// If the agent is not reachable the test is skipped with a clear message.
func checkAgentAvailable(t *testing.T) bool {
	t.Helper()
	resp, err := http.Get("http://localhost:8080/healthz")
	if err != nil {
		t.Skip("Agent not running, skipping E2E test: ", err)
		return false
	}
	_ = resp.Body.Close()
	return true
}
