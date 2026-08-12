package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithCORSSupportsBrowserPreflight(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := withCORS(next)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/v1/simulation/control", nil)
	request.Header.Set("Origin", "http://localhost:9100")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "content-type, ngrok-skip-browser-warning")
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:9100" {
		t.Fatalf("allow-origin = %q", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Fatal("allow-headers is empty")
	}
}
