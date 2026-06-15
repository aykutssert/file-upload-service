package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestPostgreSQLResolverResolvesActiveKey(t *testing.T) {
	database := &resolverDatabase{
		row: resolverRow{
			values: []any{
				"tenant-a",
				"user-a",
				"user",
				"member",
				[]string{"file:create", "file:read"},
			},
		},
	}
	resolver := NewPostgreSQLResolver(database)

	principal, err := resolver.Resolve(context.Background(), "secret-key")
	if err != nil {
		t.Fatalf("resolve API key: %v", err)
	}

	expectedHash := sha256.Sum256([]byte("secret-key"))
	if len(database.arguments) != 1 {
		t.Fatalf("query arguments = %d", len(database.arguments))
	}
	if !reflect.DeepEqual(database.arguments[0], expectedHash[:]) {
		t.Fatal("resolver did not query with the API key SHA-256 hash")
	}
	if principal.TenantID != "tenant-a" {
		t.Fatalf("TenantID = %q", principal.TenantID)
	}
	if principal.SubjectID != "user-a" {
		t.Fatalf("SubjectID = %q", principal.SubjectID)
	}
	if principal.Type != PrincipalTypeUser {
		t.Fatalf("Type = %q", principal.Type)
	}
	if principal.Role != "member" {
		t.Fatalf("Role = %q", principal.Role)
	}
	if !principal.HasPermission("file:create") {
		t.Fatal("missing file:create permission")
	}
	if !principal.HasPermission("file:read") {
		t.Fatal("missing file:read permission")
	}
}

func TestPostgreSQLResolverRejectsUnknownOrInactiveKey(t *testing.T) {
	resolver := NewPostgreSQLResolver(&resolverDatabase{
		row: resolverRow{err: pgx.ErrNoRows},
	})

	_, err := resolver.Resolve(context.Background(), "invalid-key")

	if !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("error = %v", err)
	}
}

func TestPostgreSQLResolverReturnsDatabaseFailure(t *testing.T) {
	databaseErr := errors.New("database unavailable")
	resolver := NewPostgreSQLResolver(&resolverDatabase{
		row: resolverRow{err: databaseErr},
	})

	_, err := resolver.Resolve(context.Background(), "secret-key")

	if !errors.Is(err, databaseErr) {
		t.Fatalf("error = %v", err)
	}
}

type resolverDatabase struct {
	arguments []any
	row       resolverRow
}

func (database *resolverDatabase) QueryRow(
	_ context.Context,
	_ string,
	arguments ...any,
) pgx.Row {
	database.arguments = arguments
	return database.row
}

type resolverRow struct {
	values []any
	err    error
}

func (row resolverRow) Scan(destinations ...any) error {
	if row.err != nil {
		return row.err
	}

	for index, value := range row.values {
		switch destination := destinations[index].(type) {
		case *string:
			*destination = value.(string)
		case *[]string:
			*destination = value.([]string)
		default:
			panic("unsupported scan destination")
		}
	}

	return nil
}
