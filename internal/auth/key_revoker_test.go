package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestKeyRevokerSetsRevokedAt(t *testing.T) {
	database := &resolverDatabase{
		row: resolverRow{values: []any{"key-id"}},
	}
	revoker := NewKeyRevoker(database)

	err := revoker.Revoke(context.Background(), "key-id", "tenant-id")
	if err != nil {
		t.Fatalf("revoke API key: %v", err)
	}

	if len(database.arguments) != 2 {
		t.Fatalf("query arguments = %d", len(database.arguments))
	}
	if database.arguments[0] != "key-id" {
		t.Fatalf("key ID argument = %v", database.arguments[0])
	}
	if database.arguments[1] != "tenant-id" {
		t.Fatalf("tenant ID argument = %v", database.arguments[1])
	}
}

func TestKeyRevokerRejectsUnknownCrossTenantOrAlreadyRevokedKey(t *testing.T) {
	revoker := NewKeyRevoker(&resolverDatabase{
		row: resolverRow{err: pgx.ErrNoRows},
	})

	err := revoker.Revoke(context.Background(), "missing-key", "tenant-id")

	if !errors.Is(err, ErrAPIKeyNotFound) {
		t.Fatalf("error = %v", err)
	}
}

func TestKeyRevokerReturnsDatabaseFailure(t *testing.T) {
	databaseErr := errors.New("database unavailable")
	revoker := NewKeyRevoker(&resolverDatabase{
		row: resolverRow{err: databaseErr},
	})

	err := revoker.Revoke(context.Background(), "key-id", "tenant-id")

	if !errors.Is(err, databaseErr) {
		t.Fatalf("error = %v", err)
	}
}
