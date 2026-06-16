package storage

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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

type stubPutObjectPresigner struct {
	response *v4.PresignedHTTPRequest
	err      error
}

func (stub stubPutObjectPresigner) PresignPutObject(
	context.Context,
	*s3.PutObjectInput,
	...func(*s3.PresignOptions),
) (*v4.PresignedHTTPRequest, error) {
	return stub.response, stub.err
}
