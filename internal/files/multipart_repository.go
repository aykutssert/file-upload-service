package files

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aykutssert/file-upload-service/internal/auth"
	"github.com/jackc/pgx/v5"
)

var (
	ErrInvalidMultipartInput    = errors.New("invalid multipart input")
	ErrMultipartSessionNotFound = errors.New("multipart session not found")
	ErrMultipartSessionConflict = errors.New("multipart session state conflict")
)

type MultipartSession struct {
	ID               string
	TenantID         string
	OwnerPrincipalID string
	S3UploadID       string
	ObjectKey        string
	OriginalName     string
	ContentType      string
	ExpectedSize     int64
	PartSize         int64
	Status           string
	IdempotencyKey   string
	CreatedAt        time.Time
	CompletedAt      *time.Time
	AbortedAt        *time.Time
	Reused           bool
}

type MultipartPart struct {
	PartNumber int32
	ETag       string
	Size       int64
	CreatedAt  time.Time
}

type CreateMultipartSessionInput struct {
	Principal      auth.Principal
	IdempotencyKey string
	S3UploadID     string
	ObjectKey      string
	OriginalName   string
	ContentType    string
	ExpectedSize   int64
	PartSize       int64
}

type AddPartInput struct {
	Principal  auth.Principal
	SessionID  string
	PartNumber int32
	ETag       string
	Size       int64
}

type MultipartRepository struct {
	database interface {
		QueryRower
		Queryer
	}
}

func NewMultipartRepository(database interface {
	QueryRower
	Queryer
}) *MultipartRepository {
	return &MultipartRepository{database: database}
}

func (r *MultipartRepository) CreateMultipartSession(
	ctx context.Context,
	input CreateMultipartSessionInput,
) (MultipartSession, error) {
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.S3UploadID = strings.TrimSpace(input.S3UploadID)
	input.OriginalName = strings.TrimSpace(input.OriginalName)
	input.ContentType = strings.TrimSpace(input.ContentType)
	input.ObjectKey = strings.TrimSpace(input.ObjectKey)

	if input.Principal.ID == "" ||
		input.Principal.TenantID == "" ||
		input.IdempotencyKey == "" ||
		len(input.IdempotencyKey) > 200 ||
		input.S3UploadID == "" ||
		input.ObjectKey == "" ||
		input.OriginalName == "" ||
		input.ContentType == "" ||
		input.ExpectedSize < 0 ||
		input.PartSize < 5242880 {
		return MultipartSession{}, ErrInvalidMultipartInput
	}

	var session MultipartSession
	err := r.database.QueryRow(ctx, `
		INSERT INTO multipart_uploads (
			tenant_id,
			owner_principal_id,
			s3_upload_id,
			object_key,
			original_name,
			content_type,
			expected_size,
			part_size,
			idempotency_key
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (tenant_id, owner_principal_id, idempotency_key)
		DO NOTHING
		RETURNING
			id::text,
			tenant_id::text,
			owner_principal_id::text,
			s3_upload_id,
			object_key,
			original_name,
			content_type,
			expected_size,
			part_size,
			status,
			idempotency_key,
			created_at,
			completed_at,
			aborted_at
	`, input.Principal.TenantID,
		input.Principal.ID,
		input.S3UploadID,
		input.ObjectKey,
		input.OriginalName,
		input.ContentType,
		input.ExpectedSize,
		input.PartSize,
		input.IdempotencyKey,
	).Scan(
		&session.ID,
		&session.TenantID,
		&session.OwnerPrincipalID,
		&session.S3UploadID,
		&session.ObjectKey,
		&session.OriginalName,
		&session.ContentType,
		&session.ExpectedSize,
		&session.PartSize,
		&session.Status,
		&session.IdempotencyKey,
		&session.CreatedAt,
		&session.CompletedAt,
		&session.AbortedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		// Conflict: idempotency key used — fetch existing session
		existing, fetchErr := r.fetchExistingSession(ctx, input)
		if fetchErr != nil {
			return MultipartSession{}, fetchErr
		}
		existing.Reused = true
		return existing, nil
	}
	return session, err
}

// fetchExistingSession retrieves the session that caused the idempotency conflict.
func (r *MultipartRepository) fetchExistingSession(
	ctx context.Context,
	input CreateMultipartSessionInput,
) (MultipartSession, error) {
	var session MultipartSession
	err := r.database.QueryRow(ctx, `
		SELECT
			id::text,
			tenant_id::text,
			owner_principal_id::text,
			s3_upload_id,
			object_key,
			original_name,
			content_type,
			expected_size,
			part_size,
			status,
			idempotency_key,
			created_at,
			completed_at,
			aborted_at
		FROM multipart_uploads
		WHERE tenant_id = $1
			AND owner_principal_id = $2
			AND idempotency_key = $3
	`, input.Principal.TenantID, input.Principal.ID, input.IdempotencyKey,
	).Scan(
		&session.ID,
		&session.TenantID,
		&session.OwnerPrincipalID,
		&session.S3UploadID,
		&session.ObjectKey,
		&session.OriginalName,
		&session.ContentType,
		&session.ExpectedSize,
		&session.PartSize,
		&session.Status,
		&session.IdempotencyKey,
		&session.CreatedAt,
		&session.CompletedAt,
		&session.AbortedAt,
	)
	return session, err
}

func (r *MultipartRepository) FindMultipartSession(
	ctx context.Context,
	principal auth.Principal,
	sessionID string,
) (MultipartSession, error) {
	sessionID = strings.TrimSpace(sessionID)
	if principal.ID == "" || principal.TenantID == "" || sessionID == "" {
		return MultipartSession{}, ErrInvalidMultipartInput
	}

	var session MultipartSession
	err := r.database.QueryRow(ctx, `
		SELECT
			id::text,
			tenant_id::text,
			owner_principal_id::text,
			s3_upload_id,
			object_key,
			original_name,
			content_type,
			expected_size,
			part_size,
			status,
			idempotency_key,
			created_at,
			completed_at,
			aborted_at
		FROM multipart_uploads
		WHERE id = $1
			AND tenant_id = $2
			AND owner_principal_id = $3
			AND status != 'aborted'
	`, sessionID, principal.TenantID, principal.ID,
	).Scan(
		&session.ID,
		&session.TenantID,
		&session.OwnerPrincipalID,
		&session.S3UploadID,
		&session.ObjectKey,
		&session.OriginalName,
		&session.ContentType,
		&session.ExpectedSize,
		&session.PartSize,
		&session.Status,
		&session.IdempotencyKey,
		&session.CreatedAt,
		&session.CompletedAt,
		&session.AbortedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return MultipartSession{}, ErrMultipartSessionNotFound
	}
	return session, err
}

func (r *MultipartRepository) AddPart(
	ctx context.Context,
	input AddPartInput,
) (MultipartPart, error) {
	input.ETag = strings.TrimSpace(input.ETag)
	input.SessionID = strings.TrimSpace(input.SessionID)

	if input.Principal.ID == "" ||
		input.Principal.TenantID == "" ||
		input.SessionID == "" ||
		input.PartNumber < 1 ||
		input.PartNumber > 10000 ||
		input.ETag == "" ||
		input.Size <= 0 {
		return MultipartPart{}, ErrInvalidMultipartInput
	}

	var part MultipartPart
	err := r.database.QueryRow(ctx, `
		INSERT INTO multipart_parts (multipart_upload_id, part_number, etag, size)
		SELECT $1, $2, $3, $4
		FROM multipart_uploads
		WHERE id = $1
			AND tenant_id = $5
			AND owner_principal_id = $6
			AND status = 'pending'
		ON CONFLICT (multipart_upload_id, part_number)
		DO UPDATE SET etag = EXCLUDED.etag, size = EXCLUDED.size
		RETURNING part_number, etag, size, created_at
	`, input.SessionID,
		input.PartNumber,
		input.ETag,
		input.Size,
		input.Principal.TenantID,
		input.Principal.ID,
	).Scan(&part.PartNumber, &part.ETag, &part.Size, &part.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return MultipartPart{}, ErrMultipartSessionNotFound
	}
	return part, err
}

func (r *MultipartRepository) ListParts(
	ctx context.Context,
	principal auth.Principal,
	sessionID string,
) ([]MultipartPart, error) {
	sessionID = strings.TrimSpace(sessionID)
	if principal.ID == "" || principal.TenantID == "" || sessionID == "" {
		return nil, ErrInvalidMultipartInput
	}

	rows, err := r.database.Query(ctx, `
		SELECT p.part_number, p.etag, p.size, p.created_at
		FROM multipart_parts p
		JOIN multipart_uploads u ON u.id = p.multipart_upload_id
		WHERE p.multipart_upload_id = $1
			AND u.tenant_id = $2
			AND u.owner_principal_id = $3
		ORDER BY p.part_number ASC
	`, sessionID, principal.TenantID, principal.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	parts := make([]MultipartPart, 0)
	for rows.Next() {
		var part MultipartPart
		if err := rows.Scan(&part.PartNumber, &part.ETag, &part.Size, &part.CreatedAt); err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}
	return parts, rows.Err()
}

func (r *MultipartRepository) CompleteMultipartSession(
	ctx context.Context,
	principal auth.Principal,
	sessionID string,
) (MultipartSession, error) {
	sessionID = strings.TrimSpace(sessionID)
	if principal.ID == "" || principal.TenantID == "" || sessionID == "" {
		return MultipartSession{}, ErrInvalidMultipartInput
	}

	var session MultipartSession
	err := r.database.QueryRow(ctx, `
		UPDATE multipart_uploads
		SET
			status = 'completed',
			completed_at = now(),
			updated_at = now()
		WHERE id = $1
			AND tenant_id = $2
			AND owner_principal_id = $3
			AND status = 'pending'
		RETURNING
			id::text,
			tenant_id::text,
			owner_principal_id::text,
			s3_upload_id,
			object_key,
			original_name,
			content_type,
			expected_size,
			part_size,
			status,
			idempotency_key,
			created_at,
			completed_at,
			aborted_at
	`, sessionID, principal.TenantID, principal.ID,
	).Scan(
		&session.ID,
		&session.TenantID,
		&session.OwnerPrincipalID,
		&session.S3UploadID,
		&session.ObjectKey,
		&session.OriginalName,
		&session.ContentType,
		&session.ExpectedSize,
		&session.PartSize,
		&session.Status,
		&session.IdempotencyKey,
		&session.CreatedAt,
		&session.CompletedAt,
		&session.AbortedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return MultipartSession{}, ErrMultipartSessionConflict
	}
	return session, err
}

func (r *MultipartRepository) AbortMultipartSession(
	ctx context.Context,
	principal auth.Principal,
	sessionID string,
) error {
	sessionID = strings.TrimSpace(sessionID)
	if principal.ID == "" || principal.TenantID == "" || sessionID == "" {
		return ErrInvalidMultipartInput
	}

	var abortedID string
	err := r.database.QueryRow(ctx, `
		UPDATE multipart_uploads
		SET
			status = 'aborted',
			aborted_at = now(),
			updated_at = now()
		WHERE id = $1
			AND tenant_id = $2
			AND owner_principal_id = $3
			AND status = 'pending'
		RETURNING id::text
	`, sessionID, principal.TenantID, principal.ID,
	).Scan(&abortedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMultipartSessionConflict
	}
	return err
}

type CreateReadyFileInput struct {
	Principal      auth.Principal
	IdempotencyKey string
	ObjectKey      string
	OriginalName   string
	ContentType    string
	ExpectedSize   int64
}

func (r *MultipartRepository) CreateReadyFile(
	ctx context.Context,
	input CreateReadyFileInput,
) (Upload, error) {
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.ObjectKey = strings.TrimSpace(input.ObjectKey)
	input.OriginalName = strings.TrimSpace(input.OriginalName)
	input.ContentType = strings.TrimSpace(input.ContentType)

	if input.Principal.ID == "" ||
		input.Principal.TenantID == "" ||
		input.IdempotencyKey == "" ||
		input.ObjectKey == "" ||
		input.OriginalName == "" ||
		input.ContentType == "" ||
		input.ExpectedSize < 0 {
		return Upload{}, ErrInvalidMultipartInput
	}

	requestHash := createRequestHash(CreateUploadInput{
		OriginalName: input.OriginalName,
		ContentType:  input.ContentType,
		ExpectedSize: input.ExpectedSize,
	})

	return scanUpload(r.database.QueryRow(ctx, `
		INSERT INTO files (
			tenant_id,
			owner_principal_id,
			object_key,
			original_name,
			content_type,
			expected_size,
			status,
			idempotency_key,
			create_request_hash,
			uploaded_at,
			ready_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, 'ready', $7, $8, now(), now())
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
	`, input.Principal.TenantID,
		input.Principal.ID,
		input.ObjectKey,
		input.OriginalName,
		input.ContentType,
		input.ExpectedSize,
		input.IdempotencyKey,
		requestHash[:],
	))
}

func (r *MultipartRepository) FindUploadByObjectKey(
	ctx context.Context,
	principal auth.Principal,
	objectKey string,
) (Upload, error) {
	objectKey = strings.TrimSpace(objectKey)
	if principal.TenantID == "" || objectKey == "" {
		return Upload{}, ErrInvalidMultipartInput
	}

	upload, err := scanUpload(r.database.QueryRow(ctx, `
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
		WHERE object_key = $1
			AND tenant_id = $2
	`, objectKey, principal.TenantID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Upload{}, ErrUploadNotFound
	}
	return upload, err
}
