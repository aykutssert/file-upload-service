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
