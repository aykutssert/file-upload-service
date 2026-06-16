package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aykutssert/file-upload-service/internal/auth"
	"github.com/aykutssert/file-upload-service/internal/files"
	"github.com/aykutssert/file-upload-service/internal/storage"
)

// stubMultipartCreator implements multipartCreator for tests.
type stubMultipartCreator struct {
	session            files.MultipartSession
	sessionErr         error
	parts              []files.MultipartPart
	listErr            error
	addedPart          files.MultipartPart
	addPartErr         error
	completedSession   files.MultipartSession
	completeSessionErr error
	abortErr           error
	createdFile        files.Upload
	createFileErr      error
	foundFile          files.Upload
	findFileErr        error
}

func (s *stubMultipartCreator) CreateMultipartSession(_ context.Context, _ files.CreateMultipartSessionInput) (files.MultipartSession, error) {
	return s.session, s.sessionErr
}

func (s *stubMultipartCreator) FindMultipartSession(_ context.Context, _ auth.Principal, _ string) (files.MultipartSession, error) {
	return s.session, s.sessionErr
}

func (s *stubMultipartCreator) AddPart(_ context.Context, _ files.AddPartInput) (files.MultipartPart, error) {
	return s.addedPart, s.addPartErr
}

func (s *stubMultipartCreator) ListParts(_ context.Context, _ auth.Principal, _ string) ([]files.MultipartPart, error) {
	return s.parts, s.listErr
}

func (s *stubMultipartCreator) CompleteMultipartSession(_ context.Context, _ auth.Principal, _ string) (files.MultipartSession, error) {
	return s.completedSession, s.completeSessionErr
}

func (s *stubMultipartCreator) AbortMultipartSession(_ context.Context, _ auth.Principal, _ string) error {
	return s.abortErr
}

func (s *stubMultipartCreator) CreateReadyFile(_ context.Context, _ files.CreateReadyFileInput) (files.Upload, error) {
	return s.createdFile, s.createFileErr
}

func (s *stubMultipartCreator) FindUploadByObjectKey(_ context.Context, _ auth.Principal, _ string) (files.Upload, error) {
	return s.foundFile, s.findFileErr
}

// stubMultipartPresigner implements multipartPresigner for tests.
type stubMultipartPresigner struct {
	uploadID    string
	createErr   error
	partReq     storage.PresignedRequest
	presignErr  error
	completeErr error
	abortErr    error
}

func (s stubMultipartPresigner) CreateMultipartUpload(_ context.Context, _ storage.CreateMultipartUploadInput) (string, error) {
	return s.uploadID, s.createErr
}

func (s stubMultipartPresigner) PresignUploadPart(_ context.Context, _ storage.UploadPartInput) (storage.PresignedRequest, error) {
	return s.partReq, s.presignErr
}

func (s stubMultipartPresigner) CompleteMultipartUpload(_ context.Context, _ storage.CompleteMultipartUploadInput) error {
	return s.completeErr
}

func (s stubMultipartPresigner) AbortMultipartUpload(_ context.Context, _ storage.AbortMultipartUploadInput) error {
	return s.abortErr
}

func pendingSession() files.MultipartSession {
	return files.MultipartSession{
		ID:           "session-id",
		ObjectKey:    "tenants/tenant-id/objects/obj",
		S3UploadID:   "s3-upload-id",
		OriginalName: "video.mp4",
		ContentType:  "video/mp4",
		ExpectedSize: 104857600,
		PartSize:     10485760,
		Status:       "pending",
	}
}

func TestCreateMultipartSession(t *testing.T) {
	repo := &stubMultipartCreator{session: pendingSession()}
	store := stubMultipartPresigner{uploadID: "s3-upload-id"}
	router := NewRouter(
		stubReadinessChecker{},
		&stubResolver{principal: principalWithPermission()},
		&stubUploadCreator{},
		stubUploadPresigner{},
		nil,
		nil,
		repo,
		store,
	)
	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/multipart-sessions",
		bytes.NewBufferString(`{
			"original_name":"video.mp4",
			"content_type":"video/mp4",
			"expected_size":104857600,
			"part_size":10485760
		}`),
	)
	req.Header.Set("Authorization", "Bearer secret-key")
	req.Header.Set("Idempotency-Key", "upload-video-1")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	var body multipartSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ID != "session-id" {
		t.Fatalf("ID = %q", body.ID)
	}
	if body.Status != "pending" {
		t.Fatalf("status = %q", body.Status)
	}
}

func TestCreateMultipartSessionRejectsMissingIdempotencyKey(t *testing.T) {
	router := NewRouter(
		stubReadinessChecker{},
		&stubResolver{principal: principalWithPermission()},
		&stubUploadCreator{},
		stubUploadPresigner{},
		nil,
		nil,
		&stubMultipartCreator{},
		stubMultipartPresigner{},
	)
	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/multipart-sessions",
		bytes.NewBufferString(`{"original_name":"a","content_type":"video/mp4","expected_size":1,"part_size":5242880}`),
	)
	req.Header.Set("Authorization", "Bearer secret-key")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.Code)
	}
}

func TestCompleteMultipartSessionIdempotent(t *testing.T) {
	completedSession := pendingSession()
	completedSession.Status = "completed"
	repo := &stubMultipartCreator{
		session:   completedSession,
		foundFile: files.Upload{ID: "file-id", ObjectKey: "tenants/tenant-id/objects/obj", Status: "ready"},
	}
	router := NewRouter(
		stubReadinessChecker{},
		&stubResolver{principal: principalWithPermission()},
		&stubUploadCreator{},
		stubUploadPresigner{},
		nil,
		nil,
		repo,
		stubMultipartPresigner{},
	)
	req := httptest.NewRequest(http.MethodPost, "/v1/multipart-sessions/session-id/complete", nil)
	req.Header.Set("Authorization", "Bearer secret-key")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	var body completeMultipartResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.FileID != "file-id" {
		t.Fatalf("FileID = %q", body.FileID)
	}
}

func TestAbortMultipartSessionIdempotent(t *testing.T) {
	repo := &stubMultipartCreator{sessionErr: files.ErrMultipartSessionNotFound}
	router := NewRouter(
		stubReadinessChecker{},
		&stubResolver{principal: principalWithPermission()},
		&stubUploadCreator{},
		stubUploadPresigner{},
		nil,
		nil,
		repo,
		stubMultipartPresigner{},
	)
	req := httptest.NewRequest(http.MethodDelete, "/v1/multipart-sessions/already-aborted", nil)
	req.Header.Set("Authorization", "Bearer secret-key")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d", resp.Code)
	}
}

// BenchmarkCreateMultipartSession measures per-request handler allocations.
// The expected_size field (1 MB vs 10 GB) must not affect allocation count —
// file bytes never pass through the API.
func BenchmarkCreateMultipartSession_1MB(b *testing.B) {
	benchmarkCreateSession(b, 1<<20)
}

func BenchmarkCreateMultipartSession_10GB(b *testing.B) {
	benchmarkCreateSession(b, 10<<30)
}

func benchmarkCreateSession(b *testing.B, expectedSize int64) {
	b.Helper()
	repo := &stubMultipartCreator{
		session: files.MultipartSession{
			ID:           "session-id",
			ObjectKey:    "tenants/t/objects/o",
			S3UploadID:   "s3-id",
			OriginalName: "video.mp4",
			ContentType:  "video/mp4",
			ExpectedSize: expectedSize,
			PartSize:     10 << 20,
			Status:       "pending",
		},
	}
	store := stubMultipartPresigner{uploadID: "s3-id"}
	handler := createMultipartSessionHandler(repo, store)
	principal := principalWithPermission()

	body := []byte(`{"original_name":"video.mp4","content_type":"video/mp4","expected_size":` +
		string(rune('0'+expectedSize%10)) + `,"part_size":10485760}`)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		req := httptest.NewRequest(http.MethodPost, "/v1/multipart-sessions", bytes.NewReader(body))
		req = req.WithContext(auth.WithPrincipal(req.Context(), principal))
		req.Header.Set(idempotencyKeyHeader, "bench-key")
		w := httptest.NewRecorder()
		handler(w, req)
	}
}

// BenchmarkConfirmPart measures per-part confirmation overhead.
func BenchmarkConfirmPart(b *testing.B) {
	repo := &stubMultipartCreator{
		addedPart: files.MultipartPart{
			PartNumber: 1,
			ETag:       `"etag-abc123"`,
			Size:       10 << 20,
			CreatedAt:  time.Now(),
		},
	}
	handler := confirmPartHandler(repo)
	principal := principalWithPermission()
	body := []byte(`{"etag":"\"etag-abc123\"","size":10485760}`)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		req := httptest.NewRequest(http.MethodPost, "/v1/multipart-sessions/sid/parts/1", bytes.NewReader(body))
		req = req.WithContext(auth.WithPrincipal(req.Context(), principal))
		w := httptest.NewRecorder()
		handler(w, req)
	}
}
