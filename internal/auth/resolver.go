package auth

import (
	"context"
	"crypto/sha256"
	"errors"

	"github.com/jackc/pgx/v5"
)

var ErrInvalidAPIKey = errors.New("invalid API key")

type Resolver interface {
	Resolve(context.Context, string) (Principal, error)
}

type QueryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

type PostgreSQLResolver struct {
	database QueryRower
}

func NewPostgreSQLResolver(database QueryRower) *PostgreSQLResolver {
	return &PostgreSQLResolver{database: database}
}

func (resolver *PostgreSQLResolver) Resolve(
	ctx context.Context,
	apiKey string,
) (Principal, error) {
	keyHash := sha256.Sum256([]byte(apiKey))

	var principal Principal
	var principalType string
	var permissions []string

	err := resolver.database.QueryRow(ctx, `
		WITH candidate AS MATERIALIZED (
			SELECT
				api_keys.id,
				principals.tenant_id,
				principals.external_id,
				principals.principal_type,
				principals.role,
				principals.permissions
			FROM api_keys
			JOIN principals ON principals.id = api_keys.principal_id
			WHERE api_keys.key_hash = $1
				AND api_keys.revoked_at IS NULL
				AND (
					api_keys.expires_at IS NULL
					OR api_keys.expires_at > now()
				)
				AND principals.disabled_at IS NULL
		),
		touched_key AS (
			UPDATE api_keys
			SET last_used_at = now()
			FROM candidate
			WHERE api_keys.id = candidate.id
				AND (
					api_keys.last_used_at IS NULL
					OR api_keys.last_used_at < now() - interval '5 minutes'
				)
		)
		SELECT
			tenant_id::text,
			external_id,
			principal_type,
			role,
			permissions
		FROM candidate
	`, keyHash[:]).Scan(
		&principal.TenantID,
		&principal.SubjectID,
		&principalType,
		&principal.Role,
		&permissions,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrInvalidAPIKey
	}
	if err != nil {
		return Principal{}, err
	}

	principal.Type = PrincipalType(principalType)
	principal.Permissions = make(map[string]struct{}, len(permissions))
	for _, permission := range permissions {
		principal.Permissions[permission] = struct{}{}
	}

	return principal, nil
}
