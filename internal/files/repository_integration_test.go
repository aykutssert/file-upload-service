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

func TestRepositoryCreateUploadIntegration(t *testing.T) {
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
			VALUES ('Upload Repository Tenant')
			RETURNING id
		)
		INSERT INTO principals (
			tenant_id,
			external_id,
			principal_type,
			role,
			permissions
		)
		SELECT
			id,
			'upload-repository-user',
			'user',
			'member',
			ARRAY['file:create']
		FROM tenant
		RETURNING id::text, tenant_id::text
	`).Scan(&principal.ID, &principal.TenantID)
	if err != nil {
		t.Fatalf("insert principal: %v", err)
	}

	repository := NewRepository(transaction)
	repository.random = bytes.NewReader(bytes.Repeat([]byte{0xab}, 48))
	input := CreateUploadInput{
		Principal:      principal,
		IdempotencyKey: "create-document",
		OriginalName:   "document.pdf",
		ContentType:    "application/pdf",
		ExpectedSize:   0,
	}

	first, err := repository.CreateUpload(ctx, input)
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}
	if first.Reused {
		t.Fatal("first upload was reused")
	}
	if first.Status != "pending" {
		t.Fatalf("Status = %q", first.Status)
	}
	if first.TenantID != principal.TenantID {
		t.Fatalf("TenantID = %q", first.TenantID)
	}
	if first.OwnerPrincipalID != principal.ID {
		t.Fatalf("OwnerPrincipalID = %q", first.OwnerPrincipalID)
	}

	second, err := repository.CreateUpload(ctx, input)
	if err != nil {
		t.Fatalf("reuse upload: %v", err)
	}
	if !second.Reused {
		t.Fatal("second upload was not reused")
	}
	if second.ID != first.ID {
		t.Fatalf("reused ID = %q", second.ID)
	}

	input.ExpectedSize = 1
	_, err = repository.CreateUpload(ctx, input)
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflict error = %v", err)
	}
}

func TestRepositoryCompleteUploadIntegration(t *testing.T) {
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
			VALUES ('Upload Completion Tenant')
			RETURNING id
		)
		INSERT INTO principals (
			tenant_id,
			external_id,
			principal_type,
			role,
			permissions
		)
		SELECT
			id,
			'upload-completion-user',
			'user',
			'member',
			ARRAY['file:create']
		FROM tenant
		RETURNING id::text, tenant_id::text
	`).Scan(&principal.ID, &principal.TenantID)
	if err != nil {
		t.Fatalf("insert principal: %v", err)
	}

	repository := NewRepository(transaction)
	repository.random = bytes.NewReader(bytes.Repeat([]byte{0xcd}, 16))
	created, err := repository.CreateUpload(ctx, CreateUploadInput{
		Principal:      principal,
		IdempotencyKey: "complete-document",
		OriginalName:   "document.pdf",
		ContentType:    "application/pdf",
		ExpectedSize:   12,
	})
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}

	found, err := repository.FindUpload(ctx, principal, created.ID)
	if err != nil {
		t.Fatalf("find upload: %v", err)
	}
	if found.Status != "pending" {
		t.Fatalf("found Status = %q", found.Status)
	}

	completed, err := repository.MarkUploaded(ctx, principal, created.ID)
	if err != nil {
		t.Fatalf("mark uploaded: %v", err)
	}
	if completed.Status != "uploaded" {
		t.Fatalf("completed Status = %q", completed.Status)
	}
	if completed.UploadedAt == nil {
		t.Fatal("UploadedAt is nil")
	}

	_, err = repository.MarkUploaded(ctx, principal, created.ID)
	if !errors.Is(err, ErrUploadStateConflict) {
		t.Fatalf("state conflict error = %v", err)
	}
}

func TestRepositoryMarkReadyIntegration(t *testing.T) {
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
			VALUES ('Mark Ready Tenant')
			RETURNING id
		)
		INSERT INTO principals (
			tenant_id,
			external_id,
			principal_type,
			role,
			permissions
		)
		SELECT
			id,
			'mark-ready-user',
			'user',
			'member',
			ARRAY['file:create']
		FROM tenant
		RETURNING id::text, tenant_id::text
	`).Scan(&principal.ID, &principal.TenantID)
	if err != nil {
		t.Fatalf("insert principal: %v", err)
	}

	repository := NewRepository(transaction)
	repository.random = bytes.NewReader(bytes.Repeat([]byte{0xee}, 16))
	created, err := repository.CreateUpload(ctx, CreateUploadInput{
		Principal:      principal,
		IdempotencyKey: "mark-ready-document",
		OriginalName:   "document.pdf",
		ContentType:    "application/pdf",
		ExpectedSize:   512,
	})
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}

	ready, err := repository.MarkReady(ctx, principal, created.ID)
	if err != nil {
		t.Fatalf("mark ready: %v", err)
	}
	if ready.Status != "ready" {
		t.Fatalf("status = %q", ready.Status)
	}
	if ready.UploadedAt == nil {
		t.Fatal("UploadedAt is nil")
	}

	// Retry: already ready → ErrUploadStateConflict
	_, err = repository.MarkReady(ctx, principal, created.ID)
	if !errors.Is(err, ErrUploadStateConflict) {
		t.Fatalf("retry error = %v", err)
	}

	// File is still visible after ready
	found, err := repository.FindUpload(ctx, principal, created.ID)
	if err != nil {
		t.Fatalf("find after ready: %v", err)
	}
	if found.Status != "ready" {
		t.Fatalf("found status = %q", found.Status)
	}
}

func TestRepositoryListUploadsIntegration(t *testing.T) {
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
			VALUES ('List Uploads Tenant')
			RETURNING id
		)
		INSERT INTO principals (
			tenant_id,
			external_id,
			principal_type,
			role,
			permissions
		)
		SELECT
			id,
			'list-uploads-user',
			'user',
			'member',
			ARRAY['file:read']
		FROM tenant
		RETURNING id::text, tenant_id::text
	`).Scan(&principal.ID, &principal.TenantID)
	if err != nil {
		t.Fatalf("insert principal: %v", err)
	}

	err = transaction.QueryRow(ctx, `
		WITH tenant AS (
			INSERT INTO tenants (name)
			VALUES ('Other List Uploads Tenant')
			RETURNING id
		)
		INSERT INTO principals (
			tenant_id,
			external_id,
			principal_type,
			role,
			permissions
		)
		SELECT
			id,
			'other-list-uploads-user',
			'user',
			'member',
			ARRAY['file:read']
		FROM tenant
		RETURNING id::text, tenant_id::text
	`).Scan(&otherPrincipal.ID, &otherPrincipal.TenantID)
	if err != nil {
		t.Fatalf("insert other principal: %v", err)
	}

	repository := NewRepository(transaction)
	repository.random = bytes.NewReader([]byte{
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x02,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x03,
	})

	first, err := repository.CreateUpload(ctx, CreateUploadInput{
		Principal:      principal,
		IdempotencyKey: "list-first",
		OriginalName:   "first.txt",
		ContentType:    "text/plain",
		ExpectedSize:   1,
	})
	if err != nil {
		t.Fatalf("create first upload: %v", err)
	}
	_, err = repository.CreateUpload(ctx, CreateUploadInput{
		Principal:      principal,
		IdempotencyKey: "list-second",
		OriginalName:   "second.txt",
		ContentType:    "text/plain",
		ExpectedSize:   1,
	})
	if err != nil {
		t.Fatalf("create second upload: %v", err)
	}
	_, err = repository.CreateUpload(ctx, CreateUploadInput{
		Principal:      otherPrincipal,
		IdempotencyKey: "list-other",
		OriginalName:   "other.txt",
		ContentType:    "text/plain",
		ExpectedSize:   1,
	})
	if err != nil {
		t.Fatalf("create other upload: %v", err)
	}

	list, err := repository.ListUploads(ctx, ListUploadsInput{
		Principal: principal,
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("list uploads: %v", err)
	}
	if len(list.Uploads) != 2 {
		t.Fatalf("upload count = %d", len(list.Uploads))
	}
	for _, upload := range list.Uploads {
		if upload.TenantID != principal.TenantID {
			t.Fatalf("cross-tenant upload returned: %q", upload.TenantID)
		}
	}

	filtered, err := repository.ListUploads(ctx, ListUploadsInput{
		Principal:        principal,
		OwnerPrincipalID: principal.ID,
		Status:           "pending",
		Limit:            1,
	})
	if err != nil {
		t.Fatalf("filtered list: %v", err)
	}
	if len(filtered.Uploads) != 1 {
		t.Fatalf("filtered upload count = %d", len(filtered.Uploads))
	}
	if filtered.NextCursor == "" {
		t.Fatal("NextCursor is empty")
	}

	nextPage, err := repository.ListUploads(ctx, ListUploadsInput{
		Principal: principal,
		Limit:     10,
		Cursor:    filtered.NextCursor,
	})
	if err != nil {
		t.Fatalf("next page: %v", err)
	}
	if len(nextPage.Uploads) != 1 {
		t.Fatalf("next page upload count = %d", len(nextPage.Uploads))
	}
	if nextPage.Uploads[0].ID == first.ID && filtered.Uploads[0].ID == first.ID {
		t.Fatal("cursor returned the same row twice")
	}
}

func TestRepositoryDeleteUploadIntegration(t *testing.T) {
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
			VALUES ('Delete Upload Tenant')
			RETURNING id
		)
		INSERT INTO principals (
			tenant_id, external_id, principal_type, role, permissions
		)
		SELECT id, 'delete-user', 'user', 'member', ARRAY['file:delete']
		FROM tenant
		RETURNING id::text, tenant_id::text
	`).Scan(&principal.ID, &principal.TenantID)
	if err != nil {
		t.Fatalf("insert principal: %v", err)
	}

	err = transaction.QueryRow(ctx, `
		WITH tenant AS (
			INSERT INTO tenants (name)
			VALUES ('Other Delete Tenant')
			RETURNING id
		)
		INSERT INTO principals (
			tenant_id, external_id, principal_type, role
		)
		SELECT id, 'other-delete-user', 'user', 'member'
		FROM tenant
		RETURNING id::text, tenant_id::text
	`).Scan(&otherPrincipal.ID, &otherPrincipal.TenantID)
	if err != nil {
		t.Fatalf("insert other principal: %v", err)
	}

	repository := NewRepository(transaction)
	repository.random = bytes.NewReader(bytes.Repeat([]byte{0xde}, 16))
	created, err := repository.CreateUpload(ctx, CreateUploadInput{
		Principal:      principal,
		IdempotencyKey: "delete-document",
		OriginalName:   "document.pdf",
		ContentType:    "application/pdf",
		ExpectedSize:   1024,
	})
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}

	// Transition to ready via SQL to bypass app-layer state machine
	_, err = transaction.Exec(ctx, `
		UPDATE files
		SET
			status = 'ready',
			uploaded_at = now(),
			ready_at = now(),
			updated_at = now()
		WHERE id = $1
	`, created.ID)
	if err != nil {
		t.Fatalf("force file to ready: %v", err)
	}

	// Cross-tenant delete must fail
	err = repository.DeleteUpload(ctx, otherPrincipal, created.ID)
	if !errors.Is(err, ErrUploadStateConflict) {
		t.Fatalf("cross-tenant delete error = %v", err)
	}

	// Verify file still exists and is ready
	found, err := repository.FindUpload(ctx, principal, created.ID)
	if err != nil {
		t.Fatalf("find upload after failed cross-tenant delete: %v", err)
	}
	if found.Status != "ready" {
		t.Fatalf("status after failed delete = %q", found.Status)
	}

	// Successful delete
	err = repository.DeleteUpload(ctx, principal, created.ID)
	if err != nil {
		t.Fatalf("delete upload: %v", err)
	}

	// File no longer found as owner
	_, err = repository.FindUpload(ctx, principal, created.ID)
	if !errors.Is(err, ErrUploadNotFound) {
		t.Fatalf("post-delete find error = %v", err)
	}

	// Double delete returns state conflict
	err = repository.DeleteUpload(ctx, principal, created.ID)
	if !errors.Is(err, ErrUploadStateConflict) {
		t.Fatalf("double delete error = %v", err)
	}
}

func TestRepositoryGetUploadsIntegration(t *testing.T) {
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
			VALUES ('Batch Lookup Tenant')
			RETURNING id
		)
		INSERT INTO principals (
			tenant_id, external_id, principal_type, role
		)
		SELECT id, 'batch-user', 'user', 'member'
		FROM tenant
		RETURNING id::text, tenant_id::text
	`).Scan(&principal.ID, &principal.TenantID)
	if err != nil {
		t.Fatalf("insert principal: %v", err)
	}

	err = transaction.QueryRow(ctx, `
		WITH tenant AS (
			INSERT INTO tenants (name)
			VALUES ('Other Batch Tenant')
			RETURNING id
		)
		INSERT INTO principals (
			tenant_id, external_id, principal_type, role
		)
		SELECT id, 'other-batch-user', 'user', 'member'
		FROM tenant
		RETURNING id::text, tenant_id::text
	`).Scan(&otherPrincipal.ID, &otherPrincipal.TenantID)
	if err != nil {
		t.Fatalf("insert other principal: %v", err)
	}

	randomSrc := bytes.NewReader([]byte{
		0xb1, 0xb1, 0xb1, 0xb1, 0xb1, 0xb1, 0xb1, 0xb1,
		0xb1, 0xb1, 0xb1, 0xb1, 0xb1, 0xb1, 0xb1, 0xb1,
		0xb2, 0xb2, 0xb2, 0xb2, 0xb2, 0xb2, 0xb2, 0xb2,
		0xb2, 0xb2, 0xb2, 0xb2, 0xb2, 0xb2, 0xb2, 0xb2,
		0xb3, 0xb3, 0xb3, 0xb3, 0xb3, 0xb3, 0xb3, 0xb3,
		0xb3, 0xb3, 0xb3, 0xb3, 0xb3, 0xb3, 0xb3, 0xb3,
	})
	repository := NewRepository(transaction)
	repository.random = randomSrc

	file1, err := repository.CreateUpload(ctx, CreateUploadInput{
		Principal:      principal,
		IdempotencyKey: "batch-first",
		OriginalName:   "first.pdf",
		ContentType:    "application/pdf",
		ExpectedSize:   1,
	})
	if err != nil {
		t.Fatalf("create file1: %v", err)
	}

	file2, err := repository.CreateUpload(ctx, CreateUploadInput{
		Principal:      principal,
		IdempotencyKey: "batch-second",
		OriginalName:   "second.pdf",
		ContentType:    "application/pdf",
		ExpectedSize:   1,
	})
	if err != nil {
		t.Fatalf("create file2: %v", err)
	}

	otherFile, err := repository.CreateUpload(ctx, CreateUploadInput{
		Principal:      otherPrincipal,
		IdempotencyKey: "batch-other",
		OriginalName:   "other.pdf",
		ContentType:    "application/pdf",
		ExpectedSize:   1,
	})
	if err != nil {
		t.Fatalf("create other file: %v", err)
	}

	// Batch lookup for own tenant returns own files
	found, err := repository.GetUploads(ctx, principal, []string{file1.ID, file2.ID})
	if err != nil {
		t.Fatalf("get uploads: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("found count = %d", len(found))
	}
	for _, u := range found {
		if u.TenantID != principal.TenantID {
			t.Fatalf("cross-tenant file returned: tenant_id = %q", u.TenantID)
		}
	}

	// Cross-tenant batch: querying other tenant's file returns nothing
	crossResult, err := repository.GetUploads(ctx, principal, []string{otherFile.ID})
	if err != nil {
		t.Fatalf("cross-tenant get uploads: %v", err)
	}
	if len(crossResult) != 0 {
		t.Fatalf("cross-tenant file returned in batch: count = %d", len(crossResult))
	}

	// Empty result for unknown IDs
	emptyResult, err := repository.GetUploads(ctx, principal, []string{"a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"})
	if err != nil {
		t.Fatalf("unknown id get uploads: %v", err)
	}
	if len(emptyResult) != 0 {
		t.Fatalf("unknown id returned: count = %d", len(emptyResult))
	}
}
