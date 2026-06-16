package files

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/aykutssert/file-upload-service/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMultipartRepositoryIntegration(t *testing.T) {
	databaseURL := os.Getenv("UPLOAD_API_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("UPLOAD_API_TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open database pool: %v", err)
	}
	defer pool.Close()

	transaction, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer func() {
		_ = transaction.Rollback(ctx)
	}()

	var principal auth.Principal
	var otherPrincipal auth.Principal
	err = transaction.QueryRow(ctx, `
		WITH tenant AS (
			INSERT INTO tenants (name)
			VALUES ('Multipart Repo Tenant')
			RETURNING id
		)
		INSERT INTO principals (tenant_id, external_id, principal_type, role, permissions)
		SELECT id, 'multipart-repo-user', 'user', 'member', ARRAY['file:create']
		FROM tenant
		RETURNING id::text, tenant_id::text
	`).Scan(&principal.ID, &principal.TenantID)
	if err != nil {
		t.Fatalf("insert principal: %v", err)
	}

	err = transaction.QueryRow(ctx, `
		WITH tenant AS (
			INSERT INTO tenants (name)
			VALUES ('Other Multipart Tenant')
			RETURNING id
		)
		INSERT INTO principals (tenant_id, external_id, principal_type, role)
		SELECT id, 'other-multipart-user', 'user', 'member'
		FROM tenant
		RETURNING id::text, tenant_id::text
	`).Scan(&otherPrincipal.ID, &otherPrincipal.TenantID)
	if err != nil {
		t.Fatalf("insert other principal: %v", err)
	}

	repo := NewMultipartRepository(transaction)
	_ = bytes.NewReader // keep import if needed

	input := CreateMultipartSessionInput{
		Principal:      principal,
		IdempotencyKey: "upload-video-1",
		S3UploadID:     "s3-mpu-abc123",
		ObjectKey:      "tenants/" + principal.TenantID + "/objects/video-obj",
		OriginalName:   "video.mp4",
		ContentType:    "video/mp4",
		ExpectedSize:   104857600,
		PartSize:       10485760,
	}

	// Create session
	session, err := repo.CreateMultipartSession(ctx, input)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if session.Status != "pending" {
		t.Fatalf("status = %q", session.Status)
	}
	if session.S3UploadID != "s3-mpu-abc123" {
		t.Fatalf("S3UploadID = %q", session.S3UploadID)
	}
	if session.Reused {
		t.Fatal("first session should not be reused")
	}

	// Idempotent create returns same session
	reused, err := repo.CreateMultipartSession(ctx, input)
	if err != nil {
		t.Fatalf("reuse session: %v", err)
	}
	if reused.ID != session.ID {
		t.Fatalf("reused ID = %q, want %q", reused.ID, session.ID)
	}
	if !reused.Reused {
		t.Fatal("reused session should have Reused=true")
	}

	// FindMultipartSession
	found, err := repo.FindMultipartSession(ctx, principal, session.ID)
	if err != nil {
		t.Fatalf("find session: %v", err)
	}
	if found.ID != session.ID {
		t.Fatalf("found ID = %q", found.ID)
	}

	// Cross-tenant find returns not found
	_, err = repo.FindMultipartSession(ctx, otherPrincipal, session.ID)
	if !errors.Is(err, ErrMultipartSessionNotFound) {
		t.Fatalf("cross-tenant find error = %v", err)
	}

	// AddPart
	part1, err := repo.AddPart(ctx, AddPartInput{
		Principal:  principal,
		SessionID:  session.ID,
		PartNumber: 1,
		ETag:       `"etag-part-1"`,
		Size:       10485760,
	})
	if err != nil {
		t.Fatalf("add part 1: %v", err)
	}
	if part1.PartNumber != 1 {
		t.Fatalf("part number = %d", part1.PartNumber)
	}

	part2, err := repo.AddPart(ctx, AddPartInput{
		Principal:  principal,
		SessionID:  session.ID,
		PartNumber: 2,
		ETag:       `"etag-part-2"`,
		Size:       4194304,
	})
	if err != nil {
		t.Fatalf("add part 2: %v", err)
	}
	if part2.PartNumber != 2 {
		t.Fatalf("part number = %d", part2.PartNumber)
	}

	// Re-adding part 1 with new ETag updates it (S3 semantics)
	updatedPart, err := repo.AddPart(ctx, AddPartInput{
		Principal:  principal,
		SessionID:  session.ID,
		PartNumber: 1,
		ETag:       `"etag-part-1-retry"`,
		Size:       10485760,
	})
	if err != nil {
		t.Fatalf("re-add part 1: %v", err)
	}
	if updatedPart.ETag != `"etag-part-1-retry"` {
		t.Fatalf("updated ETag = %q", updatedPart.ETag)
	}

	// Cross-tenant AddPart returns not found
	_, err = repo.AddPart(ctx, AddPartInput{
		Principal:  otherPrincipal,
		SessionID:  session.ID,
		PartNumber: 1,
		ETag:       `"etag-cross"`,
		Size:       1048576,
	})
	if !errors.Is(err, ErrMultipartSessionNotFound) {
		t.Fatalf("cross-tenant add part error = %v", err)
	}

	// ListParts — returns parts in ascending part_number order
	parts, err := repo.ListParts(ctx, principal, session.ID)
	if err != nil {
		t.Fatalf("list parts: %v", err)
	}
	if len(parts) != 2 {
		t.Fatalf("part count = %d", len(parts))
	}
	if parts[0].PartNumber != 1 || parts[1].PartNumber != 2 {
		t.Fatalf("part order wrong: %d, %d", parts[0].PartNumber, parts[1].PartNumber)
	}
	if parts[0].ETag != `"etag-part-1-retry"` {
		t.Fatalf("part 1 ETag = %q", parts[0].ETag)
	}

	// Cross-tenant ListParts returns empty
	crossParts, err := repo.ListParts(ctx, otherPrincipal, session.ID)
	if err != nil {
		t.Fatalf("cross-tenant list parts: %v", err)
	}
	if len(crossParts) != 0 {
		t.Fatalf("cross-tenant parts count = %d", len(crossParts))
	}

	// CompleteMultipartSession
	completed, err := repo.CompleteMultipartSession(ctx, principal, session.ID)
	if err != nil {
		t.Fatalf("complete session: %v", err)
	}
	if completed.Status != "completed" {
		t.Fatalf("status = %q", completed.Status)
	}
	if completed.CompletedAt == nil {
		t.Fatal("CompletedAt is nil")
	}

	// Already completed — state conflict
	_, err = repo.CompleteMultipartSession(ctx, principal, session.ID)
	if !errors.Is(err, ErrMultipartSessionConflict) {
		t.Fatalf("double complete error = %v", err)
	}
}

func TestMultipartRepositoryAbortIntegration(t *testing.T) {
	databaseURL := os.Getenv("UPLOAD_API_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("UPLOAD_API_TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open database pool: %v", err)
	}
	defer pool.Close()

	transaction, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer func() {
		_ = transaction.Rollback(ctx)
	}()

	var principal auth.Principal
	err = transaction.QueryRow(ctx, `
		WITH tenant AS (
			INSERT INTO tenants (name)
			VALUES ('Abort Multipart Tenant')
			RETURNING id
		)
		INSERT INTO principals (tenant_id, external_id, principal_type, role)
		SELECT id, 'abort-multipart-user', 'user', 'member'
		FROM tenant
		RETURNING id::text, tenant_id::text
	`).Scan(&principal.ID, &principal.TenantID)
	if err != nil {
		t.Fatalf("insert principal: %v", err)
	}

	repo := NewMultipartRepository(transaction)
	session, err := repo.CreateMultipartSession(ctx, CreateMultipartSessionInput{
		Principal:      principal,
		IdempotencyKey: "abort-video-1",
		S3UploadID:     "s3-mpu-abort",
		ObjectKey:      "tenants/" + principal.TenantID + "/objects/abort-obj",
		OriginalName:   "video.mp4",
		ContentType:    "video/mp4",
		ExpectedSize:   52428800,
		PartSize:       5242880,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Abort
	err = repo.AbortMultipartSession(ctx, principal, session.ID)
	if err != nil {
		t.Fatalf("abort session: %v", err)
	}

	// Aborted session is not visible via Find
	_, err = repo.FindMultipartSession(ctx, principal, session.ID)
	if !errors.Is(err, ErrMultipartSessionNotFound) {
		t.Fatalf("post-abort find error = %v", err)
	}

	// Double abort is state conflict
	err = repo.AbortMultipartSession(ctx, principal, session.ID)
	if !errors.Is(err, ErrMultipartSessionConflict) {
		t.Fatalf("double abort error = %v", err)
	}
}
