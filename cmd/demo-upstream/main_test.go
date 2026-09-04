package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDemoHandler(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{name: "ready", method: http.MethodGet, path: "/ready", want: http.StatusOK},
		{name: "unhealthy", method: http.MethodGet, path: "/unhealthy", want: http.StatusServiceUnavailable},
		{name: "unknown path", method: http.MethodGet, path: "/missing", want: http.StatusNotFound},
		{name: "wrong method", method: http.MethodPost, path: "/ready", want: http.StatusMethodNotAllowed},
	}

	handler := newHandler()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != test.want {
				t.Fatalf("status = %d, want %d", response.Code, test.want)
			}
		})
	}
}
