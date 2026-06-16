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
			headOutput: &s3.HeadObjectOutput{
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
			headErr: &types.NotFound{},
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

func TestCreateMultipartUpload(t *testing.T) {
	presigner := &Presigner{
		bucket: "file-upload",
		objectStore: stubObjectStore{
			createOutput: &s3.CreateMultipartUploadOutput{
				UploadId: aws.String("mpu-upload-id"),
			},
		},
	}

	uploadID, err := presigner.CreateMultipartUpload(context.Background(), CreateMultipartUploadInput{
		Key:         "tenants/tenant/objects/object-id",
		ContentType: "video/mp4",
	})
	if err != nil {
		t.Fatalf("create multipart upload: %v", err)
	}
	if uploadID != "mpu-upload-id" {
		t.Fatalf("UploadID = %q", uploadID)
	}
}

func TestCreateMultipartUploadRejectsInvalidInput(t *testing.T) {
	presigner := &Presigner{bucket: "file-upload", objectStore: stubObjectStore{}}

	_, err := presigner.CreateMultipartUpload(context.Background(), CreateMultipartUploadInput{})
	if !errors.Is(err, ErrInvalidPresignerConfig) {
		t.Fatalf("error = %v", err)
	}
}

func TestPresignUploadPart(t *testing.T) {
	presigner := &Presigner{
		bucket:    "file-upload",
		expiresIn: time.Minute,
		s3Presigner: stubPutObjectPresigner{
			partResponse: &v4.PresignedHTTPRequest{
				Method: http.MethodPut,
				URL: "http://127.0.0.1:8333/file-upload/tenants/tenant/objects/object-id" +
					"?partNumber=1&uploadId=mpu-id&X-Amz-Signature=test",
				SignedHeader: http.Header{
					"Host": []string{"127.0.0.1:8333"},
				},
			},
		},
	}

	request, err := presigner.PresignUploadPart(context.Background(), UploadPartInput{
		Key:        "tenants/tenant/objects/object-id",
		UploadID:   "mpu-id",
		PartNumber: 1,
		Size:       10485760,
	})
	if err != nil {
		t.Fatalf("presign upload part: %v", err)
	}
	if request.Method != http.MethodPut {
		t.Fatalf("Method = %q", request.Method)
	}
	if !strings.HasPrefix(request.URL, "http://127.0.0.1:8333/") {
		t.Fatalf("URL = %q", request.URL)
	}
	if _, ok := request.Headers["Host"]; ok {
		t.Fatal("Host header must not be returned to clients")
	}
}

func TestPresignUploadPartRejectsInvalidInput(t *testing.T) {
	presigner := &Presigner{bucket: "file-upload", s3Presigner: stubPutObjectPresigner{}}

	for _, input := range []UploadPartInput{
		{},
		{Key: "key", UploadID: "id", PartNumber: 0, Size: 10},
		{Key: "key", UploadID: "id", PartNumber: 1, Size: 0},
	} {
		_, err := presigner.PresignUploadPart(context.Background(), input)
		if !errors.Is(err, ErrInvalidPresignerConfig) {
			t.Fatalf("input %+v: error = %v", input, err)
		}
	}
}

func TestCompleteMultipartUpload(t *testing.T) {
	presigner := &Presigner{
		bucket:      "file-upload",
		objectStore: stubObjectStore{},
	}

	err := presigner.CompleteMultipartUpload(context.Background(), CompleteMultipartUploadInput{
		Key:      "tenants/tenant/objects/object-id",
		UploadID: "mpu-id",
		Parts: []CompletePart{
			{PartNumber: 1, ETag: `"etag1"`},
			{PartNumber: 2, ETag: `"etag2"`},
		},
	})
	if err != nil {
		t.Fatalf("complete multipart upload: %v", err)
	}
}

func TestCompleteMultipartUploadMapsNotFound(t *testing.T) {
	presigner := &Presigner{
		bucket:      "file-upload",
		objectStore: stubObjectStore{completeErr: &types.NoSuchUpload{}},
	}

	err := presigner.CompleteMultipartUpload(context.Background(), CompleteMultipartUploadInput{
		Key:      "tenants/tenant/objects/object-id",
		UploadID: "mpu-id",
		Parts:    []CompletePart{{PartNumber: 1, ETag: `"etag1"`}},
	})
	if !errors.Is(err, ErrMultipartUploadNotFound) {
		t.Fatalf("error = %v", err)
	}
}

func TestAbortMultipartUpload(t *testing.T) {
	presigner := &Presigner{
		bucket:      "file-upload",
		objectStore: stubObjectStore{},
	}

	err := presigner.AbortMultipartUpload(context.Background(), AbortMultipartUploadInput{
		Key:      "tenants/tenant/objects/object-id",
		UploadID: "mpu-id",
	})
	if err != nil {
		t.Fatalf("abort multipart upload: %v", err)
	}
}

func TestAbortMultipartUploadMapsNotFound(t *testing.T) {
	presigner := &Presigner{
		bucket:      "file-upload",
		objectStore: stubObjectStore{abortErr: &types.NoSuchUpload{}},
	}

	err := presigner.AbortMultipartUpload(context.Background(), AbortMultipartUploadInput{
		Key:      "tenants/tenant/objects/object-id",
		UploadID: "mpu-id",
	})
	if !errors.Is(err, ErrMultipartUploadNotFound) {
		t.Fatalf("error = %v", err)
	}
}

type stubPutObjectPresigner struct {
	response     *v4.PresignedHTTPRequest
	getResponse  *v4.PresignedHTTPRequest
	partResponse *v4.PresignedHTTPRequest
	err          error
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

func (stub stubPutObjectPresigner) PresignUploadPart(
	context.Context,
	*s3.UploadPartInput,
	...func(*s3.PresignOptions),
) (*v4.PresignedHTTPRequest, error) {
	return stub.partResponse, stub.err
}

type stubObjectStore struct {
	headOutput   *s3.HeadObjectOutput
	headErr      error
	createOutput *s3.CreateMultipartUploadOutput
	createErr    error
	completeErr  error
	abortErr     error
}

func (stub stubObjectStore) HeadObject(
	context.Context,
	*s3.HeadObjectInput,
	...func(*s3.Options),
) (*s3.HeadObjectOutput, error) {
	return stub.headOutput, stub.headErr
}

func (stub stubObjectStore) CreateMultipartUpload(
	context.Context,
	*s3.CreateMultipartUploadInput,
	...func(*s3.Options),
) (*s3.CreateMultipartUploadOutput, error) {
	return stub.createOutput, stub.createErr
}

func (stub stubObjectStore) CompleteMultipartUpload(
	context.Context,
	*s3.CompleteMultipartUploadInput,
	...func(*s3.Options),
) (*s3.CompleteMultipartUploadOutput, error) {
	return &s3.CompleteMultipartUploadOutput{}, stub.completeErr
}

func (stub stubObjectStore) AbortMultipartUpload(
	context.Context,
	*s3.AbortMultipartUploadInput,
	...func(*s3.Options),
) (*s3.AbortMultipartUploadOutput, error) {
	return &s3.AbortMultipartUploadOutput{}, stub.abortErr
}
