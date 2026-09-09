package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func newAuthedMetricsRequest(username, password string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.SetBasicAuth(username, password)
	return req
}

func TestMetricsHandlerWithoutAuthConfigServesOpen(t *testing.T) {
	handler := MetricsHandler("", "")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("expected open access when no credentials configured, got %d", rec.Code)
	}
}

func TestMetricsHandlerRejectsMissingCredentials(t *testing.T) {
	handler := MetricsHandler("prometheus", "secret-value")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without credentials, got %d", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Error("expected WWW-Authenticate header on 401 response")
	}
}

func TestMetricsHandlerRejectsWrongPassword(t *testing.T) {
	handler := MetricsHandler("prometheus", "secret-value")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newAuthedMetricsRequest("prometheus", "wrong"))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with wrong password, got %d", rec.Code)
	}
}

func TestMetricsHandlerRejectsWrongUsername(t *testing.T) {
	handler := MetricsHandler("prometheus", "secret-value")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newAuthedMetricsRequest("intruder", "secret-value"))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with wrong username, got %d", rec.Code)
	}
}

func TestMetricsHandlerAcceptsCorrectCredentials(t *testing.T) {
	handler := MetricsHandler("prometheus", "secret-value")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newAuthedMetricsRequest("prometheus", "secret-value"))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with correct credentials, got %d", rec.Code)
	}
}

func TestMetricsHandlerPartialConfigStaysOpen(t *testing.T) {
	handler := MetricsHandler("prometheus", "")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("expected open access when only username configured, got %d", rec.Code)
	}
}
