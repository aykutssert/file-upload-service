package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestPresignerIntegration(t *testing.T) {
	endpoint := os.Getenv("UPLOAD_API_TEST_SEAWEEDFS_URL")
	if endpoint == "" {
		t.Skip("UPLOAD_API_TEST_SEAWEEDFS_URL is not set")
	}

	accessKey := os.Getenv("UPLOAD_API_TEST_SEAWEEDFS_ACCESS_KEY")
	if accessKey == "" {
		accessKey = "local-access-key"
	}
	secretKey := os.Getenv("UPLOAD_API_TEST_SEAWEEDFS_SECRET_KEY")
	if secretKey == "" {
		secretKey = "local-secret-key"
	}
	bucket := os.Getenv("UPLOAD_API_TEST_SEAWEEDFS_BUCKET")
	if bucket == "" {
		bucket = "file-upload"
	}
	region := os.Getenv("UPLOAD_API_TEST_SEAWEEDFS_REGION")
	if region == "" {
		region = "local"
	}

	presigner, err := NewPresigner(Config{
		AccessKey: accessKey,
		SecretKey: secretKey,
		Bucket:    bucket,
		Endpoint:  endpoint,
		PublicURL: endpoint,
		Region:    region,
		ExpiresIn: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("create presigner: %v", err)
	}

	ctx := context.Background()
	objectKey := fmt.Sprintf("integration-test/%d/sample.txt", time.Now().UnixNano())
	content := []byte("hello presigner integration test")
	contentType := "text/plain"

	// Presign PUT
	putReq, err := presigner.PresignPutObject(ctx, PutObjectInput{
		Key:           objectKey,
		ContentType:   contentType,
		ContentLength: int64(len(content)),
	})
	if err != nil {
		t.Fatalf("presign put: %v", err)
	}
	if putReq.Method != http.MethodPut {
		t.Fatalf("put method = %q", putReq.Method)
	}

	// Execute PUT to SeaweedFS
	putHTTPReq, err := http.NewRequestWithContext(ctx, putReq.Method, putReq.URL, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("create put request: %v", err)
	}
	putHTTPReq.ContentLength = int64(len(content))
	for k, v := range putReq.Headers {
		putHTTPReq.Header.Set(k, v)
	}
	putResp, err := http.DefaultClient.Do(putHTTPReq)
	if err != nil {
		t.Fatalf("execute put: %v", err)
	}
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK && putResp.StatusCode != http.StatusCreated {
		t.Fatalf("put status = %d", putResp.StatusCode)
	}

	// HeadObject
	metadata, err := presigner.HeadObject(ctx, objectKey)
	if err != nil {
		t.Fatalf("head object: %v", err)
	}
	if metadata.ContentLength != int64(len(content)) {
		t.Fatalf("ContentLength = %d, want %d", metadata.ContentLength, len(content))
	}

	// Presign GET
	getReq, err := presigner.PresignGetObject(ctx, GetObjectInput{Key: objectKey})
	if err != nil {
		t.Fatalf("presign get: %v", err)
	}
	if getReq.Method != http.MethodGet {
		t.Fatalf("get method = %q", getReq.Method)
	}

	// Execute GET and verify content
	getHTTPReq, err := http.NewRequestWithContext(ctx, getReq.Method, getReq.URL, nil)
	if err != nil {
		t.Fatalf("create get request: %v", err)
	}
	for k, v := range getReq.Headers {
		getHTTPReq.Header.Set(k, v)
	}
	getResp, err := http.DefaultClient.Do(getHTTPReq)
	if err != nil {
		t.Fatalf("execute get: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d", getResp.StatusCode)
	}
	body, err := io.ReadAll(getResp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != string(content) {
		t.Fatalf("body = %q, want %q", string(body), string(content))
	}
}
