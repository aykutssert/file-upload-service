package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aykutssert/file-upload-service/internal/auth"
	"github.com/aykutssert/file-upload-service/internal/files"
	"github.com/aykutssert/file-upload-service/internal/storage"
	"github.com/go-chi/chi/v5"
)

const idempotencyKeyHeader = "Idempotency-Key"
const timeFormat = time.RFC3339Nano

type uploadCreator interface {
	CreateUpload(context.Context, files.CreateUploadInput) (files.Upload, error)
	FindUpload(context.Context, auth.Principal, string) (files.Upload, error)
	ListUploads(context.Context, files.ListUploadsInput) (files.ListUploadsResult, error)
	MarkReady(context.Context, auth.Principal, string) (files.Upload, error)
	DeleteUpload(context.Context, auth.Principal, string) error
	GetUploads(context.Context, auth.Principal, []string) ([]files.Upload, error)
}

type uploadPresigner interface {
	PresignPutObject(context.Context, storage.PutObjectInput) (storage.PresignedRequest, error)
	PresignGetObject(context.Context, storage.GetObjectInput) (storage.PresignedRequest, error)
	HeadObject(context.Context, string) (storage.ObjectMetadata, error)
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

type completeUploadResponse struct {
	ID           string `json:"id"`
	ObjectKey    string `json:"object_key"`
	OriginalName string `json:"original_name"`
	ContentType  string `json:"content_type"`
	ExpectedSize int64  `json:"expected_size"`
	Status       string `json:"status"`
}

type downloadResponse struct {
	ID                       string            `json:"id"`
	DownloadURL              string            `json:"download_url"`
	DownloadMethod           string            `json:"download_method"`
	DownloadHeaders          map[string]string `json:"download_headers"`
	DownloadExpiresInSeconds int64             `json:"download_expires_in_seconds"`
	OriginalName             string            `json:"original_name"`
	ContentType              string            `json:"content_type"`
	ExpectedSize             int64             `json:"expected_size"`
	Status                   string            `json:"status"`
}

type listUploadsResponse struct {
	Files      []fileResponse `json:"files"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

type fileResponse struct {
	ID               string `json:"id"`
	OwnerPrincipalID string `json:"owner_principal_id"`
	ObjectKey        string `json:"object_key"`
	OriginalName     string `json:"original_name"`
	ContentType      string `json:"content_type"`
	ExpectedSize     int64  `json:"expected_size"`
	Status           string `json:"status"`
	CreatedAt        string `json:"created_at"`
	UploadedAt       string `json:"uploaded_at,omitempty"`
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

func completeUploadHandler(uploads uploadCreator, objectStore uploadPresigner) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		principal, ok := auth.PrincipalFromContext(request.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "valid API key required")
			return
		}

		fileID := strings.TrimSpace(chi.URLParam(request, "id"))
		upload, err := uploads.FindUpload(request.Context(), principal, fileID)
		if errors.Is(err, files.ErrInvalidUpload) {
			writeError(w, http.StatusBadRequest, "invalid_upload", "upload request is invalid")
			return
		}
		if errors.Is(err, files.ErrUploadNotFound) {
			writeError(w, http.StatusNotFound, "upload_not_found", "upload was not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}
		// Natural idempotency: if already completed, return current state.
		if upload.Status == "ready" {
			writeJSON(w, http.StatusOK, completeUploadResponse{
				ID:           upload.ID,
				ObjectKey:    upload.ObjectKey,
				OriginalName: upload.OriginalName,
				ContentType:  upload.ContentType,
				ExpectedSize: upload.ExpectedSize,
				Status:       upload.Status,
			})
			return
		}
		if upload.Status != "pending" {
			writeError(
				w,
				http.StatusConflict,
				"upload_state_conflict",
				"only pending uploads can be completed",
			)
			return
		}

		objectMetadata, err := objectStore.HeadObject(
			request.Context(),
			upload.ObjectKey,
		)
		if errors.Is(err, storage.ErrObjectNotFound) {
			writeError(
				w,
				http.StatusConflict,
				"object_not_found",
				"uploaded object was not found",
			)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}
		if objectMetadata.ContentLength != upload.ExpectedSize ||
			objectMetadata.ContentType != upload.ContentType {
			writeError(
				w,
				http.StatusConflict,
				"object_metadata_mismatch",
				"uploaded object metadata does not match the upload session",
			)
			return
		}

		upload, err = uploads.MarkReady(request.Context(), principal, upload.ID)
		if errors.Is(err, files.ErrUploadStateConflict) {
			writeError(
				w,
				http.StatusConflict,
				"upload_state_conflict",
				"only pending uploads can be completed",
			)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}

		writeJSON(w, http.StatusOK, completeUploadResponse{
			ID:           upload.ID,
			ObjectKey:    upload.ObjectKey,
			OriginalName: upload.OriginalName,
			ContentType:  upload.ContentType,
			ExpectedSize: upload.ExpectedSize,
			Status:       upload.Status,
		})
	}
}

func downloadHandler(uploads uploadCreator, presigner uploadPresigner) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		principal, ok := auth.PrincipalFromContext(request.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "valid API key required")
			return
		}

		fileID := strings.TrimSpace(chi.URLParam(request, "id"))
		upload, err := uploads.FindUpload(request.Context(), principal, fileID)
		if errors.Is(err, files.ErrInvalidUpload) {
			writeError(w, http.StatusBadRequest, "invalid_upload", "file request is invalid")
			return
		}
		if errors.Is(err, files.ErrUploadNotFound) {
			writeError(w, http.StatusNotFound, "file_not_found", "file was not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}
		if upload.Status != "ready" {
			writeError(
				w,
				http.StatusConflict,
				"file_not_ready",
				"only ready files can be downloaded",
			)
			return
		}

		presignedDownload, err := presigner.PresignGetObject(
			request.Context(),
			storage.GetObjectInput{Key: upload.ObjectKey},
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}

		writeJSON(w, http.StatusOK, downloadResponse{
			ID:                       upload.ID,
			DownloadURL:              presignedDownload.URL,
			DownloadMethod:           presignedDownload.Method,
			DownloadHeaders:          presignedDownload.Headers,
			DownloadExpiresInSeconds: int64(presignedDownload.ExpiresIn.Seconds()),
			OriginalName:             upload.OriginalName,
			ContentType:              upload.ContentType,
			ExpectedSize:             upload.ExpectedSize,
			Status:                   upload.Status,
		})
	}
}

func listUploadsHandler(uploads uploadCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		principal, ok := auth.PrincipalFromContext(request.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "valid API key required")
			return
		}

		limit := 50
		if rawLimit := strings.TrimSpace(request.URL.Query().Get("limit")); rawLimit != "" {
			parsedLimit, err := strconv.Atoi(rawLimit)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_list_request", "list request is invalid")
				return
			}
			limit = parsedLimit
		}

		result, err := uploads.ListUploads(request.Context(), files.ListUploadsInput{
			Principal:        principal,
			OwnerPrincipalID: request.URL.Query().Get("owner_id"),
			Status:           request.URL.Query().Get("status"),
			Limit:            limit,
			Cursor:           request.URL.Query().Get("cursor"),
		})
		if errors.Is(err, files.ErrInvalidListRequest) {
			writeError(w, http.StatusBadRequest, "invalid_list_request", "list request is invalid")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}

		response := listUploadsResponse{
			Files:      make([]fileResponse, 0, len(result.Uploads)),
			NextCursor: result.NextCursor,
		}
		for _, upload := range result.Uploads {
			response.Files = append(response.Files, newFileResponse(upload))
		}

		writeJSON(w, http.StatusOK, response)
	}
}

func newFileResponse(upload files.Upload) fileResponse {
	response := fileResponse{
		ID:               upload.ID,
		OwnerPrincipalID: upload.OwnerPrincipalID,
		ObjectKey:        upload.ObjectKey,
		OriginalName:     upload.OriginalName,
		ContentType:      upload.ContentType,
		ExpectedSize:     upload.ExpectedSize,
		Status:           upload.Status,
		CreatedAt:        upload.CreatedAt.Format(timeFormat),
	}
	if upload.UploadedAt != nil {
		response.UploadedAt = upload.UploadedAt.Format(timeFormat)
	}
	return response
}

func deleteUploadHandler(uploads uploadCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		principal, ok := auth.PrincipalFromContext(request.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "valid API key required")
			return
		}

		fileID := strings.TrimSpace(chi.URLParam(request, "id"))
		upload, err := uploads.FindUpload(request.Context(), principal, fileID)
		if errors.Is(err, files.ErrInvalidUpload) {
			writeError(w, http.StatusBadRequest, "invalid_upload", "file request is invalid")
			return
		}
		if errors.Is(err, files.ErrUploadNotFound) {
			writeError(w, http.StatusNotFound, "file_not_found", "file was not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}
		if upload.Status != "ready" {
			writeError(
				w,
				http.StatusConflict,
				"file_not_ready",
				"only ready files can be deleted",
			)
			return
		}

		err = uploads.DeleteUpload(request.Context(), principal, upload.ID)
		if errors.Is(err, files.ErrUploadStateConflict) {
			writeError(
				w,
				http.StatusConflict,
				"file_not_ready",
				"only ready files can be deleted",
			)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

type batchLookupRequest struct {
	IDs []string `json:"ids"`
}

type batchLookupResponse struct {
	Files []fileResponse `json:"files"`
}

const maxBatchSize = 100

func batchLookupHandler(uploads uploadCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		principal, ok := auth.PrincipalFromContext(request.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "valid API key required")
			return
		}

		var body batchLookupRequest
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "request body is invalid")
			return
		}
		if len(body.IDs) == 0 || len(body.IDs) > maxBatchSize {
			writeError(
				w,
				http.StatusBadRequest,
				"invalid_batch_request",
				"ids must contain between 1 and 100 entries",
			)
			return
		}

		found, err := uploads.GetUploads(request.Context(), principal, body.IDs)
		if errors.Is(err, files.ErrInvalidBatchRequest) {
			writeError(w, http.StatusBadRequest, "invalid_batch_request", "batch request is invalid")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}

		response := batchLookupResponse{
			Files: make([]fileResponse, 0, len(found)),
		}
		for _, upload := range found {
			response.Files = append(response.Files, newFileResponse(upload))
		}
		writeJSON(w, http.StatusOK, response)
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
