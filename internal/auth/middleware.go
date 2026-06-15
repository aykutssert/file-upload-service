package auth

import (
	"errors"
	"net/http"
	"strings"
)

func Middleware(resolver Resolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			apiKey, ok := bearerToken(request.Header.Get("Authorization"))
			if !ok {
				writeUnauthorized(w)
				return
			}

			principal, err := resolver.Resolve(request.Context(), apiKey)
			if err != nil {
				if errors.Is(err, ErrInvalidAPIKey) {
					writeUnauthorized(w)
					return
				}
				http.Error(w, "authentication unavailable", http.StatusServiceUnavailable)
				return
			}

			next.ServeHTTP(
				w,
				request.WithContext(
					WithPrincipal(request.Context(), principal),
				),
			)
		})
	}
}

func bearerToken(value string) (string, bool) {
	parts := strings.Fields(value)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	return parts[1], true
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"valid API key required"}}`))
}
