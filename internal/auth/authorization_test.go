package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequirePermissionAllowsAuthorizedPrincipal(t *testing.T) {
	handler := RequirePermission("file:create")(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	))
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	request = request.WithContext(WithPrincipal(request.Context(), Principal{
		Permissions: map[string]struct{}{"file:create": {}},
	}))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status code = %d", response.Code)
	}
}

func TestRequirePermissionRejectsMissingPermission(t *testing.T) {
	handler := RequirePermission("file:create")(http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) {
			t.Fatal("next handler called")
		},
	))

	for _, principal := range []Principal{
		{},
		{Permissions: map[string]struct{}{"file:read": {}}},
	} {
		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request = request.WithContext(WithPrincipal(request.Context(), principal))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		if response.Code != http.StatusForbidden {
			t.Fatalf("status code = %d", response.Code)
		}
	}
}
