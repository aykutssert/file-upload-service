package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type stubReadinessChecker struct {
	err error
}

func (s stubReadinessChecker) Ping(context.Context) error {
	return s.err
}

func TestLiveness(t *testing.T) {
	router := NewRouter(stubReadinessChecker{err: errors.New("database down")})

	response := request(t, router, "/health/live")

	assertHealthResponse(t, response, http.StatusOK, "ok")
}

func TestReadinessAvailable(t *testing.T) {
	router := NewRouter(stubReadinessChecker{})

	response := request(t, router, "/health/ready")

	assertHealthResponse(t, response, http.StatusOK, "ready")
}

func TestReadinessUnavailable(t *testing.T) {
	router := NewRouter(stubReadinessChecker{err: errors.New("database down")})

	response := request(t, router, "/health/ready")

	assertHealthResponse(
		t,
		response,
		http.StatusServiceUnavailable,
		"unavailable",
	)
}

func request(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, path, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertHealthResponse(
	t *testing.T,
	response *httptest.ResponseRecorder,
	statusCode int,
	status string,
) {
	t.Helper()

	if response.Code != statusCode {
		t.Fatalf("status code = %d", response.Code)
	}
	if response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf(
			"Content-Type = %q",
			response.Header().Get("Content-Type"),
		)
	}

	var body healthResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != status {
		t.Fatalf("status = %q", body.Status)
	}
}
