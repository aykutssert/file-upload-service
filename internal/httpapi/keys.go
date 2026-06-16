package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/aykutssert/file-upload-service/internal/auth"
	"github.com/go-chi/chi/v5"
)

type keyCreatorInterface interface {
	Create(context.Context, string, *time.Time) (auth.CreatedAPIKey, error)
}

type keyRevokerInterface interface {
	Revoke(context.Context, string, string) error
}

type createKeyRequest struct {
	ExpiresAt *time.Time `json:"expires_at"`
}

type createKeyResponse struct {
	ID        string  `json:"id"`
	RawKey    string  `json:"raw_key"`
	Prefix    string  `json:"prefix"`
	ExpiresAt *string `json:"expires_at,omitempty"`
}

func createKeyHandler(creator keyCreatorInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		principal, ok := auth.PrincipalFromContext(request.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "valid API key required")
			return
		}

		var body createKeyRequest
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "request body is invalid")
			return
		}

		created, err := creator.Create(request.Context(), principal.ID, body.ExpiresAt)
		if errors.Is(err, auth.ErrInvalidAPIKeyExpiration) {
			writeError(
				w,
				http.StatusBadRequest,
				"invalid_expiration",
				"expiration must be in the future",
			)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}

		response := createKeyResponse{
			ID:     created.ID,
			RawKey: created.RawKey,
			Prefix: created.Prefix,
		}
		if created.ExpiresAt != nil {
			formatted := created.ExpiresAt.Format(time.RFC3339Nano)
			response.ExpiresAt = &formatted
		}

		writeJSON(w, http.StatusCreated, response)
	}
}

func revokeKeyHandler(revoker keyRevokerInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		principal, ok := auth.PrincipalFromContext(request.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "valid API key required")
			return
		}

		keyID := strings.TrimSpace(chi.URLParam(request, "id"))
		if keyID == "" {
			writeError(w, http.StatusBadRequest, "invalid_key_id", "key ID is required")
			return
		}

		err := revoker.Revoke(request.Context(), keyID, principal.TenantID)
		if errors.Is(err, auth.ErrAPIKeyNotFound) {
			writeError(w, http.StatusNotFound, "key_not_found", "API key was not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
