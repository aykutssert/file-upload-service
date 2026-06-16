package storage

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TestNewPresignerRejectsInvalidConfig(t *testing.T) {
	_, err := NewPresigner(Config{})

	if !strings.Contains(err.Error(), ErrInvalidPresignerConfig.Error()) {
		t.Fatalf("error = %v", err)
	}
}

func TestPresignPutObject(t *testing.T) {
	presigner := &Presigner{
		bucket:    "file-upload",
		expiresIn: time.Minute,
		s3Presigner: stubPutObjectPresigner{
			response: &v4.PresignedHTTPRequest{
				Method: http.MethodPut,
				URL: "http://127.0.0.1:8333/file-upload/tenants/tenant/objects/object-id" +
					"?X-Amz-Signature=test",
				SignedHeader: http.Header{
					"Content-Type": []string{"application/pdf"},
					"Host":         []string{"127.0.0.1:8333"},
				},
			},
		},
	}

	request, err := presigner.PresignPutObject(context.Background(), PutObjectInput{
		Key:           "tenants/tenant/objects/object-id",
		ContentType:   "application/pdf",
		ContentLength: 42,
	})
	if err != nil {
		t.Fatalf("presign put object: %v", err)
	}

	if request.Method != http.MethodPut {
		t.Fatalf("Method = %q", request.Method)
	}
	if !strings.HasPrefix(request.URL, "http://127.0.0.1:8333/") {
		t.Fatalf("URL = %q", request.URL)
	}
	if request.Headers["Content-Type"] != "application/pdf" {
		t.Fatalf("Content-Type = %q", request.Headers["Content-Type"])
	}
	if _, ok := request.Headers["Host"]; ok {
		t.Fatal("Host header must not be returned to clients")
	}
}

func TestPresignGetObject(t *testing.T) {
	presigner := &Presigner{
		bucket:    "file-upload",
		expiresIn: time.Minute,
		s3Presigner: stubPutObjectPresigner{
			getResponse: &v4.PresignedHTTPRequest{
				Method: http.MethodGet,
				URL: "http://127.0.0.1:8333/file-upload/tenants/tenant/objects/object-id" +
					"?X-Amz-Signature=test",
				SignedHeader: http.Header{
					"Host": []string{"127.0.0.1:8333"},
				},
			},
		},
	}

	request, err := presigner.PresignGetObject(context.Background(), GetObjectInput{
		Key: "tenants/tenant/objects/object-id",
	})
	if err != nil {
		t.Fatalf("presign get object: %v", err)
	}

	if request.Method != http.MethodGet {
		t.Fatalf("Method = %q", request.Method)
	}
	if !strings.HasPrefix(request.URL, "http://127.0.0.1:8333/") {
		t.Fatalf("URL = %q", request.URL)
	}
	if _, ok := request.Headers["Host"]; ok {
		t.Fatal("Host header must not be returned to clients")
	}
}

func TestHeadObject(t *testing.T) {
	presigner := &Presigner{
		bucket: "file-upload",
		objectStore: stubObjectStore{
			output: &s3.HeadObjectOutput{
				ContentLength: aws.Int64(42),
				ContentType:   aws.String("application/pdf"),
				ETag:          aws.String("etag"),
			},
		},
	}

	metadata, err := presigner.HeadObject(
		context.Background(),
		"tenants/tenant/objects/object-id",
	)
	if err != nil {
		t.Fatalf("head object: %v", err)
	}

	if metadata.ContentLength != 42 {
		t.Fatalf("ContentLength = %d", metadata.ContentLength)
	}
	if metadata.ContentType != "application/pdf" {
		t.Fatalf("ContentType = %q", metadata.ContentType)
	}
}

func TestHeadObjectMapsNotFound(t *testing.T) {
	presigner := &Presigner{
		bucket: "file-upload",
		objectStore: stubObjectStore{
			err: &types.NotFound{},
		},
	}

	_, err := presigner.HeadObject(
		context.Background(),
		"tenants/tenant/objects/object-id",
	)
	if !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("error = %v", err)
	}
}

type stubPutObjectPresigner struct {
	response    *v4.PresignedHTTPRequest
	getResponse *v4.PresignedHTTPRequest
	err         error
}

func (stub stubPutObjectPresigner) PresignPutObject(
	context.Context,
	*s3.PutObjectInput,
	...func(*s3.PresignOptions),
) (*v4.PresignedHTTPRequest, error) {
	return stub.response, stub.err
}

func (stub stubPutObjectPresigner) PresignGetObject(
	context.Context,
	*s3.GetObjectInput,
	...func(*s3.PresignOptions),
) (*v4.PresignedHTTPRequest, error) {
	return stub.getResponse, stub.err
}

type stubObjectStore struct {
	output *s3.HeadObjectOutput
	err    error
}

func (stub stubObjectStore) HeadObject(
	context.Context,
	*s3.HeadObjectInput,
	...func(*s3.Options),
) (*s3.HeadObjectOutput, error) {
	return stub.output, stub.err
}
