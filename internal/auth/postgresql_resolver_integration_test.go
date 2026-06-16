package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgreSQLResolverIntegration(t *testing.T) {
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

	var tenantID string
	err = transaction.QueryRow(ctx, `
		INSERT INTO tenants (name)
		VALUES ('Resolver Integration Tenant')
		RETURNING id::text
	`).Scan(&tenantID)
	if err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	var principalID string
	err = transaction.QueryRow(ctx, `
		INSERT INTO principals (
			tenant_id,
			external_id,
			principal_type,
			role,
			permissions
		)
		VALUES ($1, 'integration-user', 'user', 'member', $2)
		RETURNING id::text
	`, tenantID, []string{"file:create", "file:read"}).Scan(&principalID)
	if err != nil {
		t.Fatalf("insert principal: %v", err)
	}

	rawKey := "integration-secret-key"
	keyHash := sha256.Sum256([]byte(rawKey))
	var keyID string
	err = transaction.QueryRow(ctx, `
		INSERT INTO api_keys (principal_id, key_prefix, key_hash)
		VALUES ($1, 'fus_integration', $2)
		RETURNING id::text
	`, principalID, keyHash[:]).Scan(&keyID)
	if err != nil {
		t.Fatalf("insert API key: %v", err)
	}

	resolver := NewPostgreSQLResolver(transaction)
	principal, err := resolver.Resolve(ctx, rawKey)
	if err != nil {
		t.Fatalf("resolve active key: %v", err)
	}
	if principal.TenantID != tenantID {
		t.Fatalf("TenantID = %q", principal.TenantID)
	}
	if principal.ID != principalID {
		t.Fatalf("ID = %q", principal.ID)
	}
	if principal.SubjectID != "integration-user" {
		t.Fatalf("SubjectID = %q", principal.SubjectID)
	}
	if !principal.HasPermission("file:create") {
		t.Fatal("missing file:create permission")
	}

	var lastUsedAt *time.Time
	err = transaction.QueryRow(ctx, `
		SELECT last_used_at
		FROM api_keys
		WHERE id = $1
	`, keyID).Scan(&lastUsedAt)
	if err != nil {
		t.Fatalf("read API key usage: %v", err)
	}
	if lastUsedAt == nil {
		t.Fatal("last_used_at was not updated")
	}

	_, err = transaction.Exec(ctx, `
		UPDATE api_keys
		SET
			created_at = now() - interval '2 hours',
			expires_at = now() - interval '1 hour'
		WHERE id = $1
	`, keyID)
	if err != nil {
		t.Fatalf("expire API key: %v", err)
	}

	_, err = resolver.Resolve(ctx, rawKey)
	if !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("expired key error = %v", err)
	}

	_, err = transaction.Exec(ctx, `
		UPDATE api_keys
		SET
			created_at = now(),
			expires_at = NULL
		WHERE id = $1
	`, keyID)
	if err != nil {
		t.Fatalf("restore API key expiry: %v", err)
	}
	_, err = transaction.Exec(ctx, `
		UPDATE api_keys
		SET revoked_at = now()
		WHERE id = $1
	`, keyID)
	if err != nil {
		t.Fatalf("revoke API key: %v", err)
	}

	_, err = resolver.Resolve(ctx, rawKey)
	if !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("revoked key error = %v", err)
	}

	_, err = transaction.Exec(ctx, `
		UPDATE api_keys
		SET revoked_at = NULL
		WHERE id = $1
	`, keyID)
	if err != nil {
		t.Fatalf("restore API key: %v", err)
	}
	_, err = transaction.Exec(ctx, `
		UPDATE principals
		SET disabled_at = now()
		WHERE id = $1
	`, principalID)
	if err != nil {
		t.Fatalf("disable principal: %v", err)
	}

	_, err = resolver.Resolve(ctx, rawKey)
	if !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("disabled principal error = %v", err)
	}

	if err := transaction.Rollback(ctx); !errors.Is(err, pgx.ErrTxClosed) && err != nil {
		t.Fatalf("rollback transaction: %v", err)
	}
}

func TestKeyCreatorIntegration(t *testing.T) {
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

	var principalID string
	err = transaction.QueryRow(ctx, `
		WITH tenant AS (
			INSERT INTO tenants (name)
			VALUES ('Key Creator Integration Tenant')
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
			'key-creator-user',
			'user',
			'member',
			ARRAY['file:create']
		FROM tenant
		RETURNING id::text
	`).Scan(&principalID)
	if err != nil {
		t.Fatalf("insert principal: %v", err)
	}

	expiresAt := time.Now().Add(time.Hour).UTC()
	creator := NewKeyCreator(transaction)
	created, err := creator.Create(ctx, principalID, &expiresAt)
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}

	var storedPrefix string
	var storedHash []byte
	err = transaction.QueryRow(ctx, `
		SELECT key_prefix, key_hash
		FROM api_keys
		WHERE id = $1
	`, created.ID).Scan(&storedPrefix, &storedHash)
	if err != nil {
		t.Fatalf("read API key: %v", err)
	}
	expectedHash := sha256.Sum256([]byte(created.RawKey))
	if storedPrefix != created.Prefix {
		t.Fatalf("stored prefix = %q", storedPrefix)
	}
	if string(storedHash) != string(expectedHash[:]) {
		t.Fatal("stored API key hash does not match the raw key")
	}
	if string(storedHash) == created.RawKey {
		t.Fatal("raw API key was stored")
	}

	principal, err := NewPostgreSQLResolver(transaction).Resolve(
		ctx,
		created.RawKey,
	)
	if err != nil {
		t.Fatalf("resolve created key: %v", err)
	}
	if principal.SubjectID != "key-creator-user" {
		t.Fatalf("SubjectID = %q", principal.SubjectID)
	}

	_, err = transaction.Exec(ctx, `
		UPDATE principals
		SET disabled_at = now()
		WHERE id = $1
	`, principalID)
	if err != nil {
		t.Fatalf("disable principal: %v", err)
	}

	_, err = creator.Create(ctx, principalID, nil)
	if !errors.Is(err, ErrPrincipalNotFound) {
		t.Fatalf("disabled principal error = %v", err)
	}
}

func TestKeyRevokerIntegration(t *testing.T) {
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

	var tenantID string
	err = transaction.QueryRow(ctx, `
		INSERT INTO tenants (name)
		VALUES ('Revoker Integration Tenant')
		RETURNING id::text
	`).Scan(&tenantID)
	if err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	var otherTenantID string
	err = transaction.QueryRow(ctx, `
		INSERT INTO tenants (name)
		VALUES ('Revoker Other Tenant')
		RETURNING id::text
	`).Scan(&otherTenantID)
	if err != nil {
		t.Fatalf("insert other tenant: %v", err)
	}

	var principalID string
	err = transaction.QueryRow(ctx, `
		INSERT INTO principals (
			tenant_id,
			external_id,
			principal_type,
			role
		)
		VALUES ($1, 'revoker-user', 'user', 'member')
		RETURNING id::text
	`, tenantID).Scan(&principalID)
	if err != nil {
		t.Fatalf("insert principal: %v", err)
	}

	rawKey := "revoker-integration-key"
	keyHash := sha256.Sum256([]byte(rawKey))
	var keyID string
	err = transaction.QueryRow(ctx, `
		INSERT INTO api_keys (principal_id, key_prefix, key_hash)
		VALUES ($1, 'fus_revoke', $2)
		RETURNING id::text
	`, principalID, keyHash[:]).Scan(&keyID)
	if err != nil {
		t.Fatalf("insert API key: %v", err)
	}

	resolver := NewPostgreSQLResolver(transaction)
	_, err = resolver.Resolve(ctx, rawKey)
	if err != nil {
		t.Fatalf("resolve active key before revocation: %v", err)
	}

	revoker := NewKeyRevoker(transaction)

	err = revoker.Revoke(ctx, keyID, otherTenantID)
	if !errors.Is(err, ErrAPIKeyNotFound) {
		t.Fatalf("cross-tenant revocation error = %v", err)
	}

	_, err = resolver.Resolve(ctx, rawKey)
	if err != nil {
		t.Fatalf("key must still be active after failed cross-tenant revocation: %v", err)
	}

	err = revoker.Revoke(ctx, keyID, tenantID)
	if err != nil {
		t.Fatalf("revoke API key: %v", err)
	}

	_, err = resolver.Resolve(ctx, rawKey)
	if !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("revoked key error = %v", err)
	}

	err = revoker.Revoke(ctx, keyID, tenantID)
	if !errors.Is(err, ErrAPIKeyNotFound) {
		t.Fatalf("already-revoked key error = %v", err)
	}
}
