package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type stubResolver struct {
	principal Principal
	err       error
	key       string
}

func (s *stubResolver) Resolve(
	_ context.Context,
	key string,
) (Principal, error) {
	s.key = key
	return s.principal, s.err
}

func TestMiddlewareAuthenticatesBearerKey(t *testing.T) {
	resolver := &stubResolver{
		principal: Principal{TenantID: "tenant-a", SubjectID: "user-a"},
	}
	handler := Middleware(resolver)(http.HandlerFunc(
		func(w http.ResponseWriter, request *http.Request) {
			principal, ok := PrincipalFromContext(request.Context())
			if !ok {
				t.Fatal("principal not found")
			}
			if principal.TenantID != "tenant-a" {
				t.Fatalf("TenantID = %q", principal.TenantID)
			}
			w.WriteHeader(http.StatusNoContent)
		},
	))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer secret-key")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status code = %d", response.Code)
	}
	if resolver.key != "secret-key" {
		t.Fatalf("resolved key = %q", resolver.key)
	}
}

func TestMiddlewareRejectsMissingOrMalformedKey(t *testing.T) {
	for _, value := range []string{
		"",
		"Basic key",
		"Bearer",
		"Bearer   ",
		"Bearer key extra",
	} {
		t.Run(value, func(t *testing.T) {
			handler := Middleware(&stubResolver{})(http.HandlerFunc(
				func(http.ResponseWriter, *http.Request) {
					t.Fatal("next handler called")
				},
			))
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("Authorization", value)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status code = %d", response.Code)
			}
		})
	}
}

func TestMiddlewareRejectsInvalidKey(t *testing.T) {
	handler := Middleware(&stubResolver{err: ErrInvalidAPIKey})(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("next handler called")
		}),
	)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer invalid")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status code = %d", response.Code)
	}
}

func TestMiddlewareReturnsUnavailableOnResolverFailure(t *testing.T) {
	handler := Middleware(&stubResolver{err: errors.New("database down")})(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("next handler called")
		}),
	)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer key")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status code = %d", response.Code)
	}
}
