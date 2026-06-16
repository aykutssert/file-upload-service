package files

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
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
	ErrUploadNotFound      = errors.New("upload not found")
	ErrUploadStateConflict = errors.New("upload state conflict")
	ErrInvalidListRequest  = errors.New("invalid list request")
	ErrInvalidBatchRequest = errors.New("invalid batch request")
)

type QueryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type Repository struct {
	database interface {
		QueryRower
		Queryer
	}
	random io.Reader
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
	UploadedAt       *time.Time
	Reused           bool
}

type ListUploadsInput struct {
	Principal        auth.Principal
	OwnerPrincipalID string
	Status           string
	Limit            int
	Cursor           string
}

type ListUploadsResult struct {
	Uploads    []Upload
	NextCursor string
}

type listCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

func NewRepository(database interface {
	QueryRower
	Queryer
}) *Repository {
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

func (repository *Repository) FindUpload(
	ctx context.Context,
	principal auth.Principal,
	fileID string,
) (Upload, error) {
	fileID = strings.TrimSpace(fileID)
	if principal.ID == "" || principal.TenantID == "" || fileID == "" {
		return Upload{}, ErrInvalidUpload
	}

	upload, err := repository.queryUpload(ctx, `
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
			uploaded_at
		FROM files
		WHERE id = $1
			AND tenant_id = $2
			AND owner_principal_id = $3
			AND status != 'deleted'
	`, fileID, principal.TenantID, principal.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Upload{}, ErrUploadNotFound
	}
	return upload, err
}

func (repository *Repository) MarkUploaded(
	ctx context.Context,
	principal auth.Principal,
	fileID string,
) (Upload, error) {
	fileID = strings.TrimSpace(fileID)
	if principal.ID == "" || principal.TenantID == "" || fileID == "" {
		return Upload{}, ErrInvalidUpload
	}

	upload, err := repository.queryUpload(ctx, `
		UPDATE files
		SET
			status = 'uploaded',
			uploaded_at = now(),
			updated_at = now()
		WHERE id = $1
			AND tenant_id = $2
			AND owner_principal_id = $3
			AND status = 'pending'
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
			uploaded_at
	`, fileID, principal.TenantID, principal.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Upload{}, ErrUploadStateConflict
	}
	return upload, err
}

func (repository *Repository) MarkReady(
	ctx context.Context,
	principal auth.Principal,
	fileID string,
) (Upload, error) {
	fileID = strings.TrimSpace(fileID)
	if principal.ID == "" || principal.TenantID == "" || fileID == "" {
		return Upload{}, ErrInvalidUpload
	}

	upload, err := repository.queryUpload(ctx, `
		UPDATE files
		SET
			status = 'ready',
			uploaded_at = now(),
			ready_at = now(),
			updated_at = now()
		WHERE id = $1
			AND tenant_id = $2
			AND owner_principal_id = $3
			AND status = 'pending'
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
			uploaded_at
	`, fileID, principal.TenantID, principal.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Upload{}, ErrUploadStateConflict
	}
	return upload, err
}

func (repository *Repository) ListUploads(
	ctx context.Context,
	input ListUploadsInput,
) (ListUploadsResult, error) {
	input.OwnerPrincipalID = strings.TrimSpace(input.OwnerPrincipalID)
	input.Status = strings.TrimSpace(input.Status)
	input.Cursor = strings.TrimSpace(input.Cursor)

	if input.Principal.TenantID == "" ||
		(input.OwnerPrincipalID != "" && !isUUID(input.OwnerPrincipalID)) ||
		(input.Status != "" && !isValidStatus(input.Status)) ||
		input.Limit < 1 ||
		input.Limit > 100 {
		return ListUploadsResult{}, ErrInvalidListRequest
	}

	var cursor listCursor
	if input.Cursor != "" {
		var err error
		cursor, err = decodeListCursor(input.Cursor)
		if err != nil {
			return ListUploadsResult{}, ErrInvalidListRequest
		}
	}

	rows, err := repository.database.Query(ctx, `
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
			uploaded_at
		FROM files
		WHERE tenant_id = $1
			AND ($2::uuid IS NULL OR owner_principal_id = $2)
			AND ($3::text IS NULL OR status = $3)
			AND (
				$4::timestamptz IS NULL
				OR (created_at, id) < ($4, $5::uuid)
			)
		ORDER BY created_at DESC, id DESC
		LIMIT $6
	`,
		input.Principal.TenantID,
		nullableString(input.OwnerPrincipalID),
		nullableString(input.Status),
		nullableTime(cursor.CreatedAt),
		nullableString(cursor.ID),
		input.Limit+1,
	)
	if err != nil {
		return ListUploadsResult{}, err
	}
	defer rows.Close()

	uploads := make([]Upload, 0, input.Limit)
	for rows.Next() {
		upload, err := scanUpload(rows)
		if err != nil {
			return ListUploadsResult{}, err
		}
		uploads = append(uploads, upload)
	}
	if err := rows.Err(); err != nil {
		return ListUploadsResult{}, err
	}

	result := ListUploadsResult{Uploads: uploads}
	if len(result.Uploads) > input.Limit {
		next := result.Uploads[input.Limit-1]
		result.Uploads = result.Uploads[:input.Limit]
		nextCursor, err := encodeListCursor(listCursor{
			CreatedAt: next.CreatedAt,
			ID:        next.ID,
		})
		if err != nil {
			return ListUploadsResult{}, err
		}
		result.NextCursor = nextCursor
	}

	return result, nil
}

func (repository *Repository) DeleteUpload(
	ctx context.Context,
	principal auth.Principal,
	fileID string,
) error {
	fileID = strings.TrimSpace(fileID)
	if principal.ID == "" || principal.TenantID == "" || fileID == "" {
		return ErrInvalidUpload
	}

	var deletedID string
	err := repository.database.QueryRow(ctx, `
		UPDATE files
		SET status = 'deleted', deleted_at = now(), updated_at = now()
		WHERE id = $1
			AND tenant_id = $2
			AND owner_principal_id = $3
			AND status = 'ready'
		RETURNING id::text
	`, fileID, principal.TenantID, principal.ID).Scan(&deletedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUploadStateConflict
	}
	return err
}

func (repository *Repository) GetUploads(
	ctx context.Context,
	principal auth.Principal,
	ids []string,
) ([]Upload, error) {
	if principal.TenantID == "" {
		return nil, ErrInvalidBatchRequest
	}
	if len(ids) == 0 {
		return []Upload{}, nil
	}

	cleaned := make([]string, 0, len(ids))
	for _, id := range ids {
		if id = strings.TrimSpace(id); isUUID(id) {
			cleaned = append(cleaned, id)
		}
	}
	if len(cleaned) == 0 {
		return []Upload{}, nil
	}

	rows, err := repository.database.Query(ctx, `
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
			uploaded_at
		FROM files
		WHERE tenant_id = $1
			AND id::text = ANY($2)
		ORDER BY created_at DESC, id DESC
	`, principal.TenantID, cleaned)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	uploads := make([]Upload, 0, len(cleaned))
	for rows.Next() {
		upload, err := scanUpload(rows)
		if err != nil {
			return nil, err
		}
		uploads = append(uploads, upload)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return uploads, nil
}

func (repository *Repository) queryUpload(
	ctx context.Context,
	sql string,
	args ...any,
) (Upload, error) {
	var upload Upload
	err := repository.database.QueryRow(ctx, sql, args...).Scan(
		&upload.ID,
		&upload.TenantID,
		&upload.OwnerPrincipalID,
		&upload.ObjectKey,
		&upload.OriginalName,
		&upload.ContentType,
		&upload.ExpectedSize,
		&upload.Status,
		&upload.CreatedAt,
		&upload.UploadedAt,
	)
	return upload, err
}

type uploadScanner interface {
	Scan(...any) error
}

func scanUpload(scanner uploadScanner) (Upload, error) {
	var upload Upload
	err := scanner.Scan(
		&upload.ID,
		&upload.TenantID,
		&upload.OwnerPrincipalID,
		&upload.ObjectKey,
		&upload.OriginalName,
		&upload.ContentType,
		&upload.ExpectedSize,
		&upload.Status,
		&upload.CreatedAt,
		&upload.UploadedAt,
	)
	return upload, err
}

func encodeListCursor(cursor listCursor) (string, error) {
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeListCursor(value string) (listCursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return listCursor{}, err
	}
	var cursor listCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return listCursor{}, err
	}
	if cursor.CreatedAt.IsZero() || !isUUID(strings.TrimSpace(cursor.ID)) {
		return listCursor{}, ErrInvalidListRequest
	}
	return cursor, nil
}

func isValidStatus(status string) bool {
	switch status {
	case "pending", "uploaded", "processing", "ready", "rejected", "deleted":
		return true
	default:
		return false
	}
}

func isUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, character := range value {
		switch index {
		case 8, 13, 18, 23:
			if character != '-' {
				return false
			}
		default:
			if !isHex(character) {
				return false
			}
		}
	}
	return true
}

func isHex(character rune) bool {
	return (character >= '0' && character <= '9') ||
		(character >= 'a' && character <= 'f') ||
		(character >= 'A' && character <= 'F')
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
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
