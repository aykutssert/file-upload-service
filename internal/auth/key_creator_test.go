package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestKeyCreatorStoresHashAndReturnsRawKey(t *testing.T) {
	randomBytes := bytes.Repeat([]byte{0x7a}, apiKeyRandomSize)
	database := &resolverDatabase{
		row: resolverRow{values: []any{"key-id"}},
	}
	creator := NewKeyCreator(database)
	creator.random = bytes.NewReader(randomBytes)
	now := time.Date(2026, time.June, 15, 12, 0, 0, 0, time.UTC)
	creator.now = func() time.Time { return now }
	expiresAt := now.Add(time.Hour)

	created, err := creator.Create(
		context.Background(),
		"principal-id",
		&expiresAt,
	)
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}

	if created.ID != "key-id" {
		t.Fatalf("ID = %q", created.ID)
	}
	if !strings.HasPrefix(created.RawKey, apiKeyMarker) {
		t.Fatalf("RawKey = %q", created.RawKey)
	}
	if len(created.RawKey) != len(apiKeyMarker)+43 {
		t.Fatalf("RawKey length = %d", len(created.RawKey))
	}
	if created.Prefix != created.RawKey[:apiKeyPrefixSize] {
		t.Fatalf("Prefix = %q", created.Prefix)
	}
	if created.ExpiresAt == nil || !created.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("ExpiresAt = %v", created.ExpiresAt)
	}

	expectedHash := sha256.Sum256([]byte(created.RawKey))
	if len(database.arguments) != 4 {
		t.Fatalf("query arguments = %d", len(database.arguments))
	}
	if database.arguments[0] != "principal-id" {
		t.Fatalf("principal ID = %v", database.arguments[0])
	}
	if database.arguments[1] != created.Prefix {
		t.Fatalf("stored prefix = %v", database.arguments[1])
	}
	if !reflect.DeepEqual(database.arguments[2], expectedHash[:]) {
		t.Fatal("raw API key was not stored as a SHA-256 hash")
	}
	if database.arguments[3] != &expiresAt {
		t.Fatalf("stored expiry = %v", database.arguments[3])
	}
}

func TestKeyCreatorRejectsUnknownOrDisabledPrincipal(t *testing.T) {
	creator := NewKeyCreator(&resolverDatabase{
		row: resolverRow{err: pgx.ErrNoRows},
	})
	creator.random = bytes.NewReader(make([]byte, apiKeyRandomSize))

	_, err := creator.Create(context.Background(), "missing", nil)

	if !errors.Is(err, ErrPrincipalNotFound) {
		t.Fatalf("error = %v", err)
	}
}

func TestKeyCreatorReturnsRandomSourceFailure(t *testing.T) {
	creator := NewKeyCreator(&resolverDatabase{})
	creator.random = errorReader{err: errors.New("random source unavailable")}

	_, err := creator.Create(context.Background(), "principal-id", nil)

	if err == nil || err.Error() != "random source unavailable" {
		t.Fatalf("error = %v", err)
	}
}

func TestKeyCreatorRejectsNonFutureExpiration(t *testing.T) {
	creator := NewKeyCreator(&resolverDatabase{})
	now := time.Date(2026, time.June, 15, 12, 0, 0, 0, time.UTC)
	creator.now = func() time.Time { return now }

	_, err := creator.Create(context.Background(), "principal-id", &now)

	if !errors.Is(err, ErrInvalidAPIKeyExpiration) {
		t.Fatalf("error = %v", err)
	}
}

type errorReader struct {
	err error
}

func (reader errorReader) Read([]byte) (int, error) {
	return 0, reader.err
}
