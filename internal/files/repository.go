package files

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/aykutssert/file-upload-service/internal/auth"
	"github.com/jackc/pgx/v5"
)

var (
	ErrIdempotencyConflict = errors.New("idempotency key already used with different request")
	ErrInvalidUpload       = errors.New("invalid upload request")
)

type QueryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Repository struct {
	database QueryRower
	random   io.Reader
}

type CreateUploadInput struct {
	Principal      auth.Principal
	IdempotencyKey string
	OriginalName   string
	ContentType    string
	ExpectedSize   int64
}

type Upload struct {
	ID               string
	TenantID         string
	OwnerPrincipalID string
	ObjectKey        string
	OriginalName     string
	ContentType      string
	ExpectedSize     int64
	Status           string
	CreatedAt        time.Time
	Reused           bool
}

func NewRepository(database QueryRower) *Repository {
	return &Repository{
		database: database,
		random:   rand.Reader,
	}
}

func (repository *Repository) CreateUpload(
	ctx context.Context,
	input CreateUploadInput,
) (Upload, error) {
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.OriginalName = strings.TrimSpace(input.OriginalName)
	input.ContentType = strings.TrimSpace(input.ContentType)

	if input.Principal.ID == "" ||
		input.Principal.TenantID == "" ||
		input.IdempotencyKey == "" ||
		input.OriginalName == "" ||
		input.ContentType == "" ||
		len(input.IdempotencyKey) > 200 ||
		input.ExpectedSize < 0 {
		return Upload{}, ErrInvalidUpload
	}

	requestHash := createRequestHash(input)
	objectKey, err := repository.objectKey(input.Principal.TenantID)
	if err != nil {
		return Upload{}, err
	}

	var upload Upload
	var storedRequestHash []byte
	err = repository.database.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO files (
				tenant_id,
				owner_principal_id,
				object_key,
				original_name,
				content_type,
				expected_size,
				idempotency_key,
				create_request_hash
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (
				tenant_id,
				owner_principal_id,
				idempotency_key
			)
			DO NOTHING
			RETURNING
				id::text,
				tenant_id::text,
				owner_principal_id::text,
				object_key,
				original_name,
				content_type,
				expected_size,
				status,
				created_at,
				create_request_hash,
				false AS reused
		),
		existing AS (
			SELECT
				id::text,
				tenant_id::text,
				owner_principal_id::text,
				object_key,
				original_name,
				content_type,
				expected_size,
				status,
				created_at,
				create_request_hash,
				true AS reused
			FROM files
			WHERE tenant_id = $1
				AND owner_principal_id = $2
				AND idempotency_key = $7
				AND NOT EXISTS (SELECT 1 FROM inserted)
		)
		SELECT * FROM inserted
		UNION ALL
		SELECT * FROM existing
	`, input.Principal.TenantID,
		input.Principal.ID,
		objectKey,
		input.OriginalName,
		input.ContentType,
		input.ExpectedSize,
		input.IdempotencyKey,
		requestHash[:],
	).Scan(
		&upload.ID,
		&upload.TenantID,
		&upload.OwnerPrincipalID,
		&upload.ObjectKey,
		&upload.OriginalName,
		&upload.ContentType,
		&upload.ExpectedSize,
		&upload.Status,
		&upload.CreatedAt,
		&storedRequestHash,
		&upload.Reused,
	)
	if err != nil {
		return Upload{}, err
	}
	if string(storedRequestHash) != string(requestHash[:]) {
		return Upload{}, ErrIdempotencyConflict
	}

	return upload, nil
}

func (repository *Repository) objectKey(tenantID string) (string, error) {
	randomBytes := make([]byte, 16)
	if _, err := io.ReadFull(repository.random, randomBytes); err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"tenants/%s/objects/%s",
		tenantID,
		hex.EncodeToString(randomBytes),
	), nil
}

func createRequestHash(input CreateUploadInput) [32]byte {
	hash := sha256.New()
	hash.Write([]byte(input.OriginalName))
	hash.Write([]byte{0})
	hash.Write([]byte(input.ContentType))
	hash.Write([]byte{0})
	hash.Write([]byte(strconv.FormatInt(input.ExpectedSize, 10)))
	var output [32]byte
	copy(output[:], hash.Sum(nil))
	return output
}
