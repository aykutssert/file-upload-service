package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/aykutssert/file-upload-service/internal/auth"
	"github.com/aykutssert/file-upload-service/internal/files"
	"github.com/aykutssert/file-upload-service/internal/storage"
)

const idempotencyKeyHeader = "Idempotency-Key"

type uploadCreator interface {
	CreateUpload(context.Context, files.CreateUploadInput) (files.Upload, error)
}

type uploadPresigner interface {
	PresignPutObject(context.Context, storage.PutObjectInput) (storage.PresignedRequest, error)
}

type createUploadRequest struct {
	OriginalName string `json:"original_name"`
	ContentType  string `json:"content_type"`
	ExpectedSize int64  `json:"expected_size"`
}

type uploadResponse struct {
	ID                     string            `json:"id"`
	ObjectKey              string            `json:"object_key"`
	UploadURL              string            `json:"upload_url"`
	UploadMethod           string            `json:"upload_method"`
	UploadHeaders          map[string]string `json:"upload_headers"`
	UploadExpiresInSeconds int64             `json:"upload_expires_in_seconds"`
	OriginalName           string            `json:"original_name"`
	ContentType            string            `json:"content_type"`
	ExpectedSize           int64             `json:"expected_size"`
	Status                 string            `json:"status"`
	Reused                 bool              `json:"reused"`
}

func createUploadHandler(
	creator uploadCreator,
	presigner uploadPresigner,
) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		principal, ok := auth.PrincipalFromContext(request.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "valid API key required")
			return
		}

		idempotencyKey := strings.TrimSpace(request.Header.Get(idempotencyKeyHeader))
		if idempotencyKey == "" {
			writeError(
				w,
				http.StatusBadRequest,
				"missing_idempotency_key",
				"Idempotency-Key header is required",
			)
			return
		}

		var body createUploadRequest
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "request body is invalid")
			return
		}

		upload, err := creator.CreateUpload(
			request.Context(),
			files.CreateUploadInput{
				Principal:      principal,
				IdempotencyKey: idempotencyKey,
				OriginalName:   body.OriginalName,
				ContentType:    body.ContentType,
				ExpectedSize:   body.ExpectedSize,
			},
		)
		if errors.Is(err, files.ErrInvalidUpload) {
			writeError(w, http.StatusBadRequest, "invalid_upload", "upload request is invalid")
			return
		}
		if errors.Is(err, files.ErrIdempotencyConflict) {
			writeError(
				w,
				http.StatusConflict,
				"idempotency_conflict",
				"Idempotency-Key was already used with a different request",
			)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}

		presignedUpload, err := presigner.PresignPutObject(
			request.Context(),
			storage.PutObjectInput{
				Key:           upload.ObjectKey,
				ContentType:   upload.ContentType,
				ContentLength: upload.ExpectedSize,
			},
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}

		status := http.StatusCreated
		if upload.Reused {
			status = http.StatusOK
		}
		writeJSON(w, status, uploadResponse{
			ID:                     upload.ID,
			ObjectKey:              upload.ObjectKey,
			UploadURL:              presignedUpload.URL,
			UploadMethod:           presignedUpload.Method,
			UploadHeaders:          presignedUpload.Headers,
			UploadExpiresInSeconds: int64(presignedUpload.ExpiresIn.Seconds()),
			OriginalName:           upload.OriginalName,
			ContentType:            upload.ContentType,
			ExpectedSize:           upload.ExpectedSize,
			Status:                 upload.Status,
			Reused:                 upload.Reused,
		})
	}
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}
