package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthz(t *testing.T) {
	handler := NewHandler(func(context.Context) error {
		t.Fatal("health check pinged PostgreSQL")
		return nil
	}, time.Second)
	checkResponse(t, handler, "/healthz", http.StatusOK, "ok")
}

func TestReadyz(t *testing.T) {
	tests := []struct {
		name       string
		ping       func(context.Context) error
		wantCode   int
		wantStatus string
	}{
		{
			name:       "database reachable",
			ping:       func(context.Context) error { return nil },
			wantCode:   http.StatusOK,
			wantStatus: "ready",
		},
		{
			name:       "connection fails",
			ping:       func(context.Context) error { return errors.New("connection refused") },
			wantCode:   http.StatusServiceUnavailable,
			wantStatus: "unavailable",
		},
		{
			name: "ping times out",
			ping: func(ctx context.Context) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Second):
					return nil
				}
			},
			wantCode:   http.StatusServiceUnavailable,
			wantStatus: "unavailable",
		},
		{
			name:       "database URL missing",
			wantCode:   http.StatusServiceUnavailable,
			wantStatus: "unavailable",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := NewHandler(test.ping, 25*time.Millisecond)
			checkResponse(t, handler, "/readyz", test.wantCode, test.wantStatus)
		})
	}
}

func checkResponse(t *testing.T, handler http.Handler, path string, wantCode int, wantStatus string) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	if response.Code != wantCode {
		t.Errorf("status code = %d, want %d", response.Code, wantCode)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode JSON response: %v", err)
	}
	if len(body) != 1 || body["status"] != wantStatus {
		t.Errorf("response = %v, want only status %q", body, wantStatus)
	}
}
