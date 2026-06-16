package auth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var ErrAPIKeyNotFound = errors.New("API key not found")

type KeyRevoker struct {
	database QueryRower
}

func NewKeyRevoker(database QueryRower) *KeyRevoker {
	return &KeyRevoker{database: database}
}

// Revoke sets revoked_at on a key owned by callerTenantID.
// Returns ErrAPIKeyNotFound when the key is absent, already revoked,
// or belongs to a different tenant.
func (revoker *KeyRevoker) Revoke(
	ctx context.Context,
	keyID string,
	callerTenantID string,
) error {
	var revokedID string
	err := revoker.database.QueryRow(ctx, `
		UPDATE api_keys
		SET revoked_at = now()
		FROM principals
		WHERE api_keys.id = $1
			AND principals.id = api_keys.principal_id
			AND principals.tenant_id = $2
			AND api_keys.revoked_at IS NULL
		RETURNING api_keys.id::text
	`, keyID, callerTenantID).Scan(&revokedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAPIKeyNotFound
	}
	return err
}
