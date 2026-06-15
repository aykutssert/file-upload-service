package readiness

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPChecker(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{name: "available", statusCode: http.StatusOK},
		{
			name:       "unavailable",
			statusCode: http.StatusServiceUnavailable,
			wantErr:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(test.statusCode)
				},
			))
			defer server.Close()

			checker := NewHTTPChecker(server.Client(), server.URL)
			err := checker.Ping(context.Background())
			if (err != nil) != test.wantErr {
				t.Fatalf("Ping() error = %v", err)
			}
		})
	}
}
