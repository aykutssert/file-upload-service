package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aykutssert/file-upload-service/internal/auth"
	"github.com/aykutssert/file-upload-service/internal/files"
	"github.com/aykutssert/file-upload-service/internal/storage"
)

type stubReadinessChecker struct {
	err error
}

func (s stubReadinessChecker) Ping(context.Context) error {
	return s.err
}

func TestLiveness(t *testing.T) {
	router := NewRouter(
		stubReadinessChecker{err: errors.New("database down")},
		nil,
		nil,
		nil,
	)

	response := request(t, router, "/health/live")

	assertHealthResponse(t, response, http.StatusOK, "ok")
}

func TestReadinessAvailable(t *testing.T) {
	router := NewRouter(stubReadinessChecker{}, nil, nil, nil)

	response := request(t, router, "/health/ready")

	assertHealthResponse(t, response, http.StatusOK, "ready")
}

func TestReadinessUnavailable(t *testing.T) {
	router := NewRouter(
		stubReadinessChecker{err: errors.New("database down")},
		nil,
		nil,
		nil,
	)

	response := request(t, router, "/health/ready")

	assertHealthResponse(
		t,
		response,
		http.StatusServiceUnavailable,
		"unavailable",
	)
}

func TestCreateUploadSession(t *testing.T) {
	resolver := &stubResolver{
		principal: auth.Principal{
			ID:       "principal-id",
			TenantID: "tenant-id",
			Permissions: map[string]struct{}{
				"file:create": {},
			},
		},
	}
	creator := &stubUploadCreator{
		upload: files.Upload{
			ID:           "file-id",
			ObjectKey:    "tenants/tenant-id/objects/object-id",
			OriginalName: "document.pdf",
			ContentType:  "application/pdf",
			ExpectedSize: 10,
			Status:       "pending",
			CreatedAt:    time.Now(),
		},
	}
	router := NewRouter(stubReadinessChecker{}, resolver, creator, stubUploadPresigner{})
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/upload-sessions",
		bytes.NewBufferString(`{
			"original_name":"document.pdf",
			"content_type":"application/pdf",
			"expected_size":10
		}`),
	)
	request.Header.Set("Authorization", "Bearer secret-key")
	request.Header.Set("Idempotency-Key", "create-document")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status code = %d", response.Code)
	}
	if resolver.key != "secret-key" {
		t.Fatalf("resolved key = %q", resolver.key)
	}
	if creator.input.IdempotencyKey != "create-document" {
		t.Fatalf("IdempotencyKey = %q", creator.input.IdempotencyKey)
	}
	if creator.input.Principal.ID != "principal-id" {
		t.Fatalf("Principal.ID = %q", creator.input.Principal.ID)
	}
	var body uploadResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.ID != "file-id" {
		t.Fatalf("ID = %q", body.ID)
	}
	if body.UploadMethod != http.MethodPut {
		t.Fatalf("UploadMethod = %q", body.UploadMethod)
	}
	if body.UploadURL != "http://127.0.0.1:8333/file-upload/object" {
		t.Fatalf("UploadURL = %q", body.UploadURL)
	}
	if body.UploadHeaders["Content-Type"] != "application/pdf" {
		t.Fatalf("UploadHeaders = %v", body.UploadHeaders)
	}
}

func TestCreateUploadSessionRejectsMissingIdempotencyKey(t *testing.T) {
	router := NewRouter(
		stubReadinessChecker{},
		&stubResolver{principal: principalWithPermission()},
		&stubUploadCreator{},
		stubUploadPresigner{},
	)
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/upload-sessions",
		bytes.NewBufferString(`{"original_name":"a","content_type":"text/plain"}`),
	)
	request.Header.Set("Authorization", "Bearer secret-key")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d", response.Code)
	}
}

func TestCreateUploadSessionRejectsMissingPermission(t *testing.T) {
	router := NewRouter(
		stubReadinessChecker{},
		&stubResolver{principal: auth.Principal{ID: "principal-id", TenantID: "tenant-id"}},
		&stubUploadCreator{},
		stubUploadPresigner{},
	)
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/upload-sessions",
		bytes.NewBufferString(`{"original_name":"a","content_type":"text/plain"}`),
	)
	request.Header.Set("Authorization", "Bearer secret-key")
	request.Header.Set("Idempotency-Key", "create")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status code = %d", response.Code)
	}
}

func TestCreateUploadSessionReportsIdempotencyConflict(t *testing.T) {
	router := NewRouter(
		stubReadinessChecker{},
		&stubResolver{principal: principalWithPermission()},
		&stubUploadCreator{err: files.ErrIdempotencyConflict},
		stubUploadPresigner{},
	)
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/upload-sessions",
		bytes.NewBufferString(`{"original_name":"a","content_type":"text/plain"}`),
	)
	request.Header.Set("Authorization", "Bearer secret-key")
	request.Header.Set("Idempotency-Key", "create")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("status code = %d", response.Code)
	}
}

func TestCompleteUpload(t *testing.T) {
	uploads := &stubUploadCreator{
		upload: files.Upload{
			ID:           "file-id",
			ObjectKey:    "tenants/tenant-id/objects/object-id",
			OriginalName: "document.pdf",
			ContentType:  "application/pdf",
			ExpectedSize: 10,
			Status:       "pending",
			CreatedAt:    time.Now(),
		},
		completed: files.Upload{
			ID:           "file-id",
			ObjectKey:    "tenants/tenant-id/objects/object-id",
			OriginalName: "document.pdf",
			ContentType:  "application/pdf",
			ExpectedSize: 10,
			Status:       "uploaded",
			CreatedAt:    time.Now(),
		},
	}
	router := NewRouter(
		stubReadinessChecker{},
		&stubResolver{principal: principalWithPermission()},
		uploads,
		stubUploadPresigner{
			metadata: storage.ObjectMetadata{
				ContentLength: 10,
				ContentType:   "application/pdf",
			},
		},
	)
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/files/file-id/complete",
		nil,
	)
	request.Header.Set("Authorization", "Bearer secret-key")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d", response.Code)
	}
	if uploads.findFileID != "file-id" {
		t.Fatalf("findFileID = %q", uploads.findFileID)
	}
	if uploads.markFileID != "file-id" {
		t.Fatalf("markFileID = %q", uploads.markFileID)
	}
	var body completeUploadResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != "uploaded" {
		t.Fatalf("Status = %q", body.Status)
	}
}

func TestCompleteUploadRejectsMissingObject(t *testing.T) {
	router := NewRouter(
		stubReadinessChecker{},
		&stubResolver{principal: principalWithPermission()},
		&stubUploadCreator{
			upload: files.Upload{
				ID:           "file-id",
				ObjectKey:    "tenants/tenant-id/objects/object-id",
				ContentType:  "application/pdf",
				ExpectedSize: 10,
				Status:       "pending",
			},
		},
		stubUploadPresigner{headErr: storage.ErrObjectNotFound},
	)
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/files/file-id/complete",
		nil,
	)
	request.Header.Set("Authorization", "Bearer secret-key")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("status code = %d", response.Code)
	}
}

func TestCompleteUploadRejectsMetadataMismatch(t *testing.T) {
	router := NewRouter(
		stubReadinessChecker{},
		&stubResolver{principal: principalWithPermission()},
		&stubUploadCreator{
			upload: files.Upload{
				ID:           "file-id",
				ObjectKey:    "tenants/tenant-id/objects/object-id",
				ContentType:  "application/pdf",
				ExpectedSize: 10,
				Status:       "pending",
			},
		},
		stubUploadPresigner{
			metadata: storage.ObjectMetadata{
				ContentLength: 9,
				ContentType:   "application/pdf",
			},
		},
	)
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/files/file-id/complete",
		nil,
	)
	request.Header.Set("Authorization", "Bearer secret-key")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("status code = %d", response.Code)
	}
}

func TestCompleteUploadRejectsStateConflict(t *testing.T) {
	router := NewRouter(
		stubReadinessChecker{},
		&stubResolver{principal: principalWithPermission()},
		&stubUploadCreator{
			upload: files.Upload{
				ID:           "file-id",
				ObjectKey:    "tenants/tenant-id/objects/object-id",
				ContentType:  "application/pdf",
				ExpectedSize: 10,
				Status:       "uploaded",
			},
		},
		stubUploadPresigner{},
	)
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/files/file-id/complete",
		nil,
	)
	request.Header.Set("Authorization", "Bearer secret-key")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("status code = %d", response.Code)
	}
}

func TestDownloadReadyFile(t *testing.T) {
	router := NewRouter(
		stubReadinessChecker{},
		&stubResolver{principal: principalWithReadPermission()},
		&stubUploadCreator{
			upload: files.Upload{
				ID:           "file-id",
				ObjectKey:    "tenants/tenant-id/objects/object-id",
				OriginalName: "document.pdf",
				ContentType:  "application/pdf",
				ExpectedSize: 10,
				Status:       "ready",
			},
		},
		stubUploadPresigner{},
	)
	request := httptest.NewRequest(
		http.MethodGet,
		"/v1/files/file-id/download",
		nil,
	)
	request.Header.Set("Authorization", "Bearer secret-key")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d", response.Code)
	}
	var body downloadResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.DownloadMethod != http.MethodGet {
		t.Fatalf("DownloadMethod = %q", body.DownloadMethod)
	}
	if body.DownloadURL != "http://127.0.0.1:8333/file-upload/object" {
		t.Fatalf("DownloadURL = %q", body.DownloadURL)
	}
	if body.Status != "ready" {
		t.Fatalf("Status = %q", body.Status)
	}
}

func TestDownloadRejectsMissingReadPermission(t *testing.T) {
	router := NewRouter(
		stubReadinessChecker{},
		&stubResolver{principal: principalWithPermission()},
		&stubUploadCreator{},
		stubUploadPresigner{},
	)
	request := httptest.NewRequest(
		http.MethodGet,
		"/v1/files/file-id/download",
		nil,
	)
	request.Header.Set("Authorization", "Bearer secret-key")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status code = %d", response.Code)
	}
}

func TestDownloadRejectsFileThatIsNotReady(t *testing.T) {
	router := NewRouter(
		stubReadinessChecker{},
		&stubResolver{principal: principalWithReadPermission()},
		&stubUploadCreator{
			upload: files.Upload{
				ID:           "file-id",
				ObjectKey:    "tenants/tenant-id/objects/object-id",
				ContentType:  "application/pdf",
				ExpectedSize: 10,
				Status:       "uploaded",
			},
		},
		stubUploadPresigner{},
	)
	request := httptest.NewRequest(
		http.MethodGet,
		"/v1/files/file-id/download",
		nil,
	)
	request.Header.Set("Authorization", "Bearer secret-key")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("status code = %d", response.Code)
	}
}

func TestListUploads(t *testing.T) {
	createdAt := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	router := NewRouter(
		stubReadinessChecker{},
		&stubResolver{principal: principalWithReadPermission()},
		&stubUploadCreator{
			listResult: files.ListUploadsResult{
				Uploads: []files.Upload{
					{
						ID:               "file-id",
						OwnerPrincipalID: "principal-id",
						ObjectKey:        "tenants/tenant-id/objects/object-id",
						OriginalName:     "document.pdf",
						ContentType:      "application/pdf",
						ExpectedSize:     10,
						Status:           "ready",
						CreatedAt:        createdAt,
					},
				},
				NextCursor: "next-cursor",
			},
		},
		stubUploadPresigner{},
	)
	request := httptest.NewRequest(
		http.MethodGet,
		"/v1/files?status=ready&owner_id=principal-id&limit=25",
		nil,
	)
	request.Header.Set("Authorization", "Bearer secret-key")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d", response.Code)
	}
	var body listUploadsResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Files) != 1 {
		t.Fatalf("file count = %d", len(body.Files))
	}
	if body.Files[0].ID != "file-id" {
		t.Fatalf("ID = %q", body.Files[0].ID)
	}
	if body.NextCursor != "next-cursor" {
		t.Fatalf("NextCursor = %q", body.NextCursor)
	}
}

func TestListUploadsRejectsMissingReadPermission(t *testing.T) {
	router := NewRouter(
		stubReadinessChecker{},
		&stubResolver{principal: principalWithPermission()},
		&stubUploadCreator{},
		stubUploadPresigner{},
	)
	request := httptest.NewRequest(http.MethodGet, "/v1/files", nil)
	request.Header.Set("Authorization", "Bearer secret-key")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status code = %d", response.Code)
	}
}

func TestListUploadsRejectsInvalidLimit(t *testing.T) {
	router := NewRouter(
		stubReadinessChecker{},
		&stubResolver{principal: principalWithReadPermission()},
		&stubUploadCreator{},
		stubUploadPresigner{},
	)
	request := httptest.NewRequest(http.MethodGet, "/v1/files?limit=invalid", nil)
	request.Header.Set("Authorization", "Bearer secret-key")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d", response.Code)
	}
}

type stubResolver struct {
	principal auth.Principal
	err       error
	key       string
}

func (s *stubResolver) Resolve(
	_ context.Context,
	key string,
) (auth.Principal, error) {
	s.key = key
	return s.principal, s.err
}

type stubUploadCreator struct {
	input      files.CreateUploadInput
	upload     files.Upload
	completed  files.Upload
	err        error
	findErr    error
	findFileID string
	listResult files.ListUploadsResult
	listErr    error
	markErr    error
	markFileID string
}

func (s *stubUploadCreator) CreateUpload(
	_ context.Context,
	input files.CreateUploadInput,
) (files.Upload, error) {
	s.input = input
	return s.upload, s.err
}

func (s *stubUploadCreator) FindUpload(
	_ context.Context,
	_ auth.Principal,
	fileID string,
) (files.Upload, error) {
	s.findFileID = fileID
	return s.upload, s.findErr
}

func (s *stubUploadCreator) ListUploads(
	_ context.Context,
	_ files.ListUploadsInput,
) (files.ListUploadsResult, error) {
	return s.listResult, s.listErr
}

func (s *stubUploadCreator) MarkUploaded(
	_ context.Context,
	_ auth.Principal,
	fileID string,
) (files.Upload, error) {
	s.markFileID = fileID
	return s.completed, s.markErr
}

type stubUploadPresigner struct {
	request  storage.PutObjectInput
	err      error
	headKey  string
	metadata storage.ObjectMetadata
	headErr  error
}

func (s stubUploadPresigner) PresignPutObject(
	_ context.Context,
	input storage.PutObjectInput,
) (storage.PresignedRequest, error) {
	s.request = input
	return storage.PresignedRequest{
		Method:    http.MethodPut,
		URL:       "http://127.0.0.1:8333/file-upload/object",
		ExpiresIn: time.Minute,
		Headers: map[string]string{
			"Content-Type": "application/pdf",
		},
	}, s.err
}

func (s stubUploadPresigner) PresignGetObject(
	_ context.Context,
	input storage.GetObjectInput,
) (storage.PresignedRequest, error) {
	return storage.PresignedRequest{
		Method:    http.MethodGet,
		URL:       "http://127.0.0.1:8333/file-upload/object",
		ExpiresIn: time.Minute,
		Headers:   map[string]string{},
	}, s.err
}

func (s stubUploadPresigner) HeadObject(
	_ context.Context,
	key string,
) (storage.ObjectMetadata, error) {
	s.headKey = key
	return s.metadata, s.headErr
}

func principalWithPermission() auth.Principal {
	return auth.Principal{
		ID:       "principal-id",
		TenantID: "tenant-id",
		Permissions: map[string]struct{}{
			"file:create": {},
		},
	}
}

func principalWithReadPermission() auth.Principal {
	return auth.Principal{
		ID:       "principal-id",
		TenantID: "tenant-id",
		Permissions: map[string]struct{}{
			"file:read": {},
		},
	}
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
