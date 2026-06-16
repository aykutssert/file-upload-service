package files

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aykutssert/file-upload-service/internal/auth"
)

func TestRepositoryValidatesCreateUploadInput(t *testing.T) {
	repository := NewRepository(nil)

	for _, input := range []CreateUploadInput{
		{},
		{
			Principal:      auth.Principal{ID: "principal-id", TenantID: "tenant-id"},
			IdempotencyKey: "   ",
			OriginalName:   "document.pdf",
			ContentType:    "application/pdf",
		},
		{
			Principal:      auth.Principal{ID: "principal-id", TenantID: "tenant-id"},
			IdempotencyKey: strings.Repeat("a", 201),
			OriginalName:   "document.pdf",
			ContentType:    "application/pdf",
		},
	} {
		_, err := repository.CreateUpload(context.Background(), input)
		if !errors.Is(err, ErrInvalidUpload) {
			t.Fatalf("error = %v", err)
		}
	}
}

func TestRepositoryBuildsTenantScopedObjectKey(t *testing.T) {
	repository := NewRepository(nil)
	repository.random = bytes.NewReader(bytes.Repeat([]byte{0xab}, 16))

	objectKey, err := repository.objectKey("tenant-id")
	if err != nil {
		t.Fatalf("object key: %v", err)
	}

	if !strings.HasPrefix(objectKey, "tenants/tenant-id/objects/") {
		t.Fatalf("objectKey = %q", objectKey)
	}
}

func TestRepositoryValidatesMarkReadyInput(t *testing.T) {
	repository := NewRepository(nil)

	_, err := repository.MarkReady(context.Background(), auth.Principal{}, "")
	if !errors.Is(err, ErrInvalidUpload) {
		t.Fatalf("error = %v", err)
	}
}

func TestCreateRequestHashChangesWithRequestBody(t *testing.T) {
	input := CreateUploadInput{
		Principal: auth.Principal{
			ID:       "principal-id",
			TenantID: "tenant-id",
		},
		IdempotencyKey: "create",
		OriginalName:   "document.pdf",
		ContentType:    "application/pdf",
		ExpectedSize:   10,
	}

	first := createRequestHash(input)
	input.ExpectedSize = 11
	second := createRequestHash(input)

	if first == second {
		t.Fatal("request hash did not change")
	}
}
