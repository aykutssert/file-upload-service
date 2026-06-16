package files

import (
	"context"
	"errors"
	"testing"

	"github.com/aykutssert/file-upload-service/internal/auth"
)

func TestMultipartRepositoryValidatesCreateInput(t *testing.T) {
	repo := NewMultipartRepository(nil)

	for _, input := range []CreateMultipartSessionInput{
		{},
		{
			Principal:      auth.Principal{ID: "id", TenantID: "tid"},
			IdempotencyKey: "key",
			S3UploadID:     "s3id",
			ObjectKey:      "obj",
			OriginalName:   "file.mp4",
			ContentType:    "video/mp4",
			ExpectedSize:   0,
			PartSize:       5242879, // below minimum
		},
		{
			Principal:      auth.Principal{ID: "id", TenantID: "tid"},
			IdempotencyKey: "key",
			S3UploadID:     "",
			ObjectKey:      "obj",
			OriginalName:   "file.mp4",
			ContentType:    "video/mp4",
			ExpectedSize:   0,
			PartSize:       5242880,
		},
	} {
		_, err := repo.CreateMultipartSession(context.Background(), input)
		if !errors.Is(err, ErrInvalidMultipartInput) {
			t.Fatalf("input %+v: error = %v", input, err)
		}
	}
}

func TestMultipartRepositoryValidatesFindInput(t *testing.T) {
	repo := NewMultipartRepository(nil)

	_, err := repo.FindMultipartSession(context.Background(), auth.Principal{}, "")
	if !errors.Is(err, ErrInvalidMultipartInput) {
		t.Fatalf("error = %v", err)
	}
}

func TestMultipartRepositoryValidatesAddPartInput(t *testing.T) {
	repo := NewMultipartRepository(nil)

	for _, input := range []AddPartInput{
		{},
		{Principal: auth.Principal{ID: "id", TenantID: "tid"}, SessionID: "sid", PartNumber: 0, ETag: "etag", Size: 1},
		{Principal: auth.Principal{ID: "id", TenantID: "tid"}, SessionID: "sid", PartNumber: 1, ETag: "", Size: 1},
		{Principal: auth.Principal{ID: "id", TenantID: "tid"}, SessionID: "sid", PartNumber: 1, ETag: "etag", Size: 0},
		{Principal: auth.Principal{ID: "id", TenantID: "tid"}, SessionID: "sid", PartNumber: 10001, ETag: "etag", Size: 1},
	} {
		_, err := repo.AddPart(context.Background(), input)
		if !errors.Is(err, ErrInvalidMultipartInput) {
			t.Fatalf("input %+v: error = %v", input, err)
		}
	}
}

func TestMultipartRepositoryValidatesListPartsInput(t *testing.T) {
	repo := NewMultipartRepository(nil)

	_, err := repo.ListParts(context.Background(), auth.Principal{}, "")
	if !errors.Is(err, ErrInvalidMultipartInput) {
		t.Fatalf("error = %v", err)
	}
}

func TestMultipartRepositoryValidatesCompleteInput(t *testing.T) {
	repo := NewMultipartRepository(nil)

	_, err := repo.CompleteMultipartSession(context.Background(), auth.Principal{}, "")
	if !errors.Is(err, ErrInvalidMultipartInput) {
		t.Fatalf("error = %v", err)
	}
}

func TestMultipartRepositoryValidatesAbortInput(t *testing.T) {
	repo := NewMultipartRepository(nil)

	err := repo.AbortMultipartSession(context.Background(), auth.Principal{}, "")
	if !errors.Is(err, ErrInvalidMultipartInput) {
		t.Fatalf("error = %v", err)
	}
}
