package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/aykutssert/file-upload-service/internal/auth"
	"github.com/aykutssert/file-upload-service/internal/files"
	"github.com/aykutssert/file-upload-service/internal/storage"
	"github.com/go-chi/chi/v5"
)

type multipartCreator interface {
	CreateMultipartSession(context.Context, files.CreateMultipartSessionInput) (files.MultipartSession, error)
	FindMultipartSession(context.Context, auth.Principal, string) (files.MultipartSession, error)
	AddPart(context.Context, files.AddPartInput) (files.MultipartPart, error)
	ListParts(context.Context, auth.Principal, string) ([]files.MultipartPart, error)
	CompleteMultipartSession(context.Context, auth.Principal, string) (files.MultipartSession, error)
	AbortMultipartSession(context.Context, auth.Principal, string) error
	CreateReadyFile(context.Context, files.CreateReadyFileInput) (files.Upload, error)
	FindUploadByObjectKey(context.Context, auth.Principal, string) (files.Upload, error)
}

type multipartPresigner interface {
	CreateMultipartUpload(context.Context, storage.CreateMultipartUploadInput) (string, error)
	PresignUploadPart(context.Context, storage.UploadPartInput) (storage.PresignedRequest, error)
	CompleteMultipartUpload(context.Context, storage.CompleteMultipartUploadInput) error
	AbortMultipartUpload(context.Context, storage.AbortMultipartUploadInput) error
}

type multipartSessionResponse struct {
	ID           string `json:"id"`
	ObjectKey    string `json:"object_key"`
	OriginalName string `json:"original_name"`
	ContentType  string `json:"content_type"`
	ExpectedSize int64  `json:"expected_size"`
	PartSize     int64  `json:"part_size"`
	Status       string `json:"status"`
	Reused       bool   `json:"reused"`
}

type presignPartResponse struct {
	PartNumber             int32             `json:"part_number"`
	UploadURL              string            `json:"upload_url"`
	UploadMethod           string            `json:"upload_method"`
	UploadHeaders          map[string]string `json:"upload_headers"`
	UploadExpiresInSeconds int64             `json:"upload_expires_in_seconds"`
	Size                   int64             `json:"size"`
}

type partResponse struct {
	PartNumber int32  `json:"part_number"`
	ETag       string `json:"etag"`
	Size       int64  `json:"size"`
}

type listPartsResponse struct {
	Parts []partResponse `json:"parts"`
}

type completeMultipartResponse struct {
	FileID       string `json:"file_id"`
	ObjectKey    string `json:"object_key"`
	OriginalName string `json:"original_name"`
	ContentType  string `json:"content_type"`
	ExpectedSize int64  `json:"expected_size"`
	Status       string `json:"status"`
}

// POST /v1/multipart-sessions
func createMultipartSessionHandler(
	repo multipartCreator,
	store multipartPresigner,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "valid API key required")
			return
		}

		idempotencyKey := strings.TrimSpace(r.Header.Get(idempotencyKeyHeader))
		if idempotencyKey == "" {
			writeError(w, http.StatusBadRequest, "missing_idempotency_key", "Idempotency-Key header is required")
			return
		}

		var body struct {
			OriginalName string `json:"original_name"`
			ContentType  string `json:"content_type"`
			ExpectedSize int64  `json:"expected_size"`
			PartSize     int64  `json:"part_size"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "request body is invalid")
			return
		}

		var randomBytes [16]byte
		if _, err := rand.Read(randomBytes[:]); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}
		objectKey := fmt.Sprintf("tenants/%s/objects/%s", principal.TenantID, hex.EncodeToString(randomBytes[:]))

		s3UploadID, err := store.CreateMultipartUpload(r.Context(), storage.CreateMultipartUploadInput{
			Key:         objectKey,
			ContentType: body.ContentType,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}

		session, err := repo.CreateMultipartSession(r.Context(), files.CreateMultipartSessionInput{
			Principal:      principal,
			IdempotencyKey: idempotencyKey,
			S3UploadID:     s3UploadID,
			ObjectKey:      objectKey,
			OriginalName:   body.OriginalName,
			ContentType:    body.ContentType,
			ExpectedSize:   body.ExpectedSize,
			PartSize:       body.PartSize,
		})
		if errors.Is(err, files.ErrInvalidMultipartInput) {
			writeError(w, http.StatusBadRequest, "invalid_multipart_input", "multipart session request is invalid")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}

		status := http.StatusCreated
		if session.Reused {
			status = http.StatusOK
		}
		writeJSON(w, status, multipartSessionResponse{
			ID:           session.ID,
			ObjectKey:    session.ObjectKey,
			OriginalName: session.OriginalName,
			ContentType:  session.ContentType,
			ExpectedSize: session.ExpectedSize,
			PartSize:     session.PartSize,
			Status:       session.Status,
			Reused:       session.Reused,
		})
	}
}

// GET /v1/multipart-sessions/{id}/parts/{n}?size=N
func presignPartHandler(
	repo multipartCreator,
	store multipartPresigner,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "valid API key required")
			return
		}

		sessionID := strings.TrimSpace(chi.URLParam(r, "id"))
		partNumber, err := parsePartNumber(chi.URLParam(r, "n"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_part_number", "part number must be between 1 and 10000")
			return
		}

		sizeStr := strings.TrimSpace(r.URL.Query().Get("size"))
		if sizeStr == "" {
			writeError(w, http.StatusBadRequest, "missing_size", "size query parameter is required")
			return
		}
		size, err := strconv.ParseInt(sizeStr, 10, 64)
		if err != nil || size <= 0 {
			writeError(w, http.StatusBadRequest, "invalid_size", "size must be a positive integer")
			return
		}

		session, err := repo.FindMultipartSession(r.Context(), principal, sessionID)
		if errors.Is(err, files.ErrInvalidMultipartInput) {
			writeError(w, http.StatusBadRequest, "invalid_session_id", "session ID is invalid")
			return
		}
		if errors.Is(err, files.ErrMultipartSessionNotFound) {
			writeError(w, http.StatusNotFound, "session_not_found", "multipart session was not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}
		if session.Status != "pending" {
			writeError(w, http.StatusConflict, "session_not_pending", "only pending sessions can upload parts")
			return
		}

		presigned, err := store.PresignUploadPart(r.Context(), storage.UploadPartInput{
			Key:        session.ObjectKey,
			UploadID:   session.S3UploadID,
			PartNumber: partNumber,
			Size:       size,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}

		writeJSON(w, http.StatusOK, presignPartResponse{
			PartNumber:             partNumber,
			UploadURL:              presigned.URL,
			UploadMethod:           presigned.Method,
			UploadHeaders:          presigned.Headers,
			UploadExpiresInSeconds: int64(presigned.ExpiresIn.Seconds()),
			Size:                   size,
		})
	}
}

// POST /v1/multipart-sessions/{id}/parts/{n}
func confirmPartHandler(repo multipartCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "valid API key required")
			return
		}

		sessionID := strings.TrimSpace(chi.URLParam(r, "id"))
		partNumber, err := parsePartNumber(chi.URLParam(r, "n"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_part_number", "part number must be between 1 and 10000")
			return
		}

		var body struct {
			ETag string `json:"etag"`
			Size int64  `json:"size"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "request body is invalid")
			return
		}

		part, err := repo.AddPart(r.Context(), files.AddPartInput{
			Principal:  principal,
			SessionID:  sessionID,
			PartNumber: partNumber,
			ETag:       body.ETag,
			Size:       body.Size,
		})
		if errors.Is(err, files.ErrInvalidMultipartInput) {
			writeError(w, http.StatusBadRequest, "invalid_part", "part data is invalid")
			return
		}
		if errors.Is(err, files.ErrMultipartSessionNotFound) {
			writeError(w, http.StatusNotFound, "session_not_found", "multipart session was not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}

		writeJSON(w, http.StatusOK, partResponse{
			PartNumber: part.PartNumber,
			ETag:       part.ETag,
			Size:       part.Size,
		})
	}
}

// GET /v1/multipart-sessions/{id}/parts
func listPartsHandler(repo multipartCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "valid API key required")
			return
		}

		sessionID := strings.TrimSpace(chi.URLParam(r, "id"))

		parts, err := repo.ListParts(r.Context(), principal, sessionID)
		if errors.Is(err, files.ErrInvalidMultipartInput) {
			writeError(w, http.StatusBadRequest, "invalid_session_id", "session ID is invalid")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}

		response := listPartsResponse{
			Parts: make([]partResponse, 0, len(parts)),
		}
		for _, p := range parts {
			response.Parts = append(response.Parts, partResponse{
				PartNumber: p.PartNumber,
				ETag:       p.ETag,
				Size:       p.Size,
			})
		}
		writeJSON(w, http.StatusOK, response)
	}
}

// POST /v1/multipart-sessions/{id}/complete
func completeMultipartSessionHandler(
	repo multipartCreator,
	store multipartPresigner,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "valid API key required")
			return
		}

		sessionID := strings.TrimSpace(chi.URLParam(r, "id"))

		session, err := repo.FindMultipartSession(r.Context(), principal, sessionID)
		if errors.Is(err, files.ErrInvalidMultipartInput) {
			writeError(w, http.StatusBadRequest, "invalid_session_id", "session ID is invalid")
			return
		}
		if errors.Is(err, files.ErrMultipartSessionNotFound) {
			writeError(w, http.StatusNotFound, "session_not_found", "multipart session was not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}

		// Natural idempotency: already completed — find the created file and return it.
		if session.Status == "completed" {
			file, err := repo.FindUploadByObjectKey(r.Context(), principal, session.ObjectKey)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
				return
			}
			writeJSON(w, http.StatusOK, completeMultipartResponse{
				FileID:       file.ID,
				ObjectKey:    file.ObjectKey,
				OriginalName: file.OriginalName,
				ContentType:  file.ContentType,
				ExpectedSize: file.ExpectedSize,
				Status:       file.Status,
			})
			return
		}
		if session.Status != "pending" {
			writeError(w, http.StatusConflict, "session_not_pending", "only pending sessions can be completed")
			return
		}

		parts, err := repo.ListParts(r.Context(), principal, sessionID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}
		if len(parts) == 0 {
			writeError(w, http.StatusConflict, "no_parts", "at least one part must be uploaded before completing")
			return
		}

		completeParts := make([]storage.CompletePart, len(parts))
		for i, p := range parts {
			completeParts[i] = storage.CompletePart{PartNumber: p.PartNumber, ETag: p.ETag}
		}

		err = store.CompleteMultipartUpload(r.Context(), storage.CompleteMultipartUploadInput{
			Key:      session.ObjectKey,
			UploadID: session.S3UploadID,
			Parts:    completeParts,
		})
		// ErrMultipartUploadNotFound: S3 already completed on a prior attempt; proceed to DB update.
		if err != nil && !errors.Is(err, storage.ErrMultipartUploadNotFound) {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}

		file, err := repo.CreateReadyFile(r.Context(), files.CreateReadyFileInput{
			Principal:      principal,
			IdempotencyKey: session.ID,
			ObjectKey:      session.ObjectKey,
			OriginalName:   session.OriginalName,
			ContentType:    session.ContentType,
			ExpectedSize:   session.ExpectedSize,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}

		_, err = repo.CompleteMultipartSession(r.Context(), principal, sessionID)
		if err != nil && !errors.Is(err, files.ErrMultipartSessionConflict) {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}

		writeJSON(w, http.StatusOK, completeMultipartResponse{
			FileID:       file.ID,
			ObjectKey:    file.ObjectKey,
			OriginalName: file.OriginalName,
			ContentType:  file.ContentType,
			ExpectedSize: file.ExpectedSize,
			Status:       file.Status,
		})
	}
}

// DELETE /v1/multipart-sessions/{id}
func abortMultipartSessionHandler(
	repo multipartCreator,
	store multipartPresigner,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "valid API key required")
			return
		}

		sessionID := strings.TrimSpace(chi.URLParam(r, "id"))

		session, err := repo.FindMultipartSession(r.Context(), principal, sessionID)
		if errors.Is(err, files.ErrMultipartSessionNotFound) {
			// Not found or already aborted — idempotent success.
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}
		if session.Status == "completed" {
			writeError(w, http.StatusConflict, "session_completed", "completed sessions cannot be aborted")
			return
		}

		err = store.AbortMultipartUpload(r.Context(), storage.AbortMultipartUploadInput{
			Key:      session.ObjectKey,
			UploadID: session.S3UploadID,
		})
		if err != nil && !errors.Is(err, storage.ErrMultipartUploadNotFound) {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}

		err = repo.AbortMultipartSession(r.Context(), principal, sessionID)
		if err != nil && !errors.Is(err, files.ErrMultipartSessionConflict) {
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func parsePartNumber(raw string) (int32, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 32)
	if err != nil || n < 1 || n > 10000 {
		return 0, errors.New("invalid part number")
	}
	return int32(n), nil
}
