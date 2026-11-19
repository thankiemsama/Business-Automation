package telemetry

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsAuthenticationAndStreaming(t *testing.T) {
	token := strings.Repeat("a", 32)
	handler := Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/events" {
			w.Header().Set("Content-Type", "text/event-stream")
			if err := http.NewResponseController(w).Flush(); err != nil {
				t.Errorf("streaming unsupported: %v", err)
			}
			return
		}
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}), token)
	request := func(path, auth string) *httptest.ResponseRecorder {
		t.Helper()
		out := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		handler.ServeHTTP(out, req)
		return out
	}
	if request("/metrics", "").Code != http.StatusNotFound {
		t.Fatal("unauthenticated scraper can see metrics")
	}
	if !request("/events", "").Flushed || request("/unavailable", "").Code != http.StatusServiceUnavailable {
		t.Fatal("middleware did not preserve the handlers' behavior")
	}
	got := request("/metrics", token)
	for _, expected := range []string{
		"platform_http_requests_total{status_class=\"5xx\"} 1",
		"platform_http_request_duration_seconds_count 1",
		"platform_postgres_write_lock_wait_seconds_count",
	} {
		if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), expected) {
			t.Fatalf("missing metric %q: %s", expected, got.Body.String())
		}
	}
}
