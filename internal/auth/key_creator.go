package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	apiKeyMarker     = "fus_"
	apiKeyRandomSize = 32
	apiKeyPrefixSize = 12
)

var (
	ErrInvalidAPIKeyExpiration = errors.New("API key expiration must be in the future")
	ErrPrincipalNotFound       = errors.New("principal not found")
)

type KeyCreator struct {
	database QueryRower
	now      func() time.Time
	random   io.Reader
}

type CreatedAPIKey struct {
	ID        string
	RawKey    string
	Prefix    string
	ExpiresAt *time.Time
}

func NewKeyCreator(database QueryRower) *KeyCreator {
	return &KeyCreator{
		database: database,
		now:      time.Now,
		random:   rand.Reader,
	}
}

func (creator *KeyCreator) Create(
	ctx context.Context,
	principalID string,
	expiresAt *time.Time,
) (CreatedAPIKey, error) {
	if expiresAt != nil && !expiresAt.After(creator.now()) {
		return CreatedAPIKey{}, ErrInvalidAPIKeyExpiration
	}

	randomBytes := make([]byte, apiKeyRandomSize)
	if _, err := io.ReadFull(creator.random, randomBytes); err != nil {
		return CreatedAPIKey{}, err
	}

	rawKey := apiKeyMarker + base64.RawURLEncoding.EncodeToString(randomBytes)
	prefix := rawKey[:apiKeyPrefixSize]
	keyHash := sha256.Sum256([]byte(rawKey))

	created := CreatedAPIKey{
		RawKey:    rawKey,
		Prefix:    prefix,
		ExpiresAt: expiresAt,
	}

	err := creator.database.QueryRow(ctx, `
		INSERT INTO api_keys (
			principal_id,
			key_prefix,
			key_hash,
			expires_at
		)
		SELECT id, $2, $3, $4
		FROM principals
		WHERE id = $1
			AND disabled_at IS NULL
		RETURNING id::text
	`, principalID, prefix, keyHash[:], expiresAt).Scan(&created.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return CreatedAPIKey{}, ErrPrincipalNotFound
	}
	if err != nil {
		return CreatedAPIKey{}, err
	}

	return created, nil
}
