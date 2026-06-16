package storage

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

var (
	ErrInvalidPresignerConfig = errors.New("invalid storage presigner config")
	ErrObjectNotFound         = errors.New("object not found")
)

type Presigner struct {
	bucket      string
	expiresIn   time.Duration
	objectStore objectStore
	s3Presigner putObjectPresigner
}

type Config struct {
	AccessKey string
	Bucket    string
	Endpoint  string
	ExpiresIn time.Duration
	PublicURL string
	Region    string
	SecretKey string
}

type PutObjectInput struct {
	Key           string
	ContentType   string
	ContentLength int64
}

type GetObjectInput struct {
	Key string
}

type PresignedRequest struct {
	Method    string
	URL       string
	ExpiresIn time.Duration
	Headers   map[string]string
}

type ObjectMetadata struct {
	ContentLength int64
	ContentType   string
	ETag          string
}

type putObjectPresigner interface {
	PresignPutObject(
		context.Context,
		*s3.PutObjectInput,
		...func(*s3.PresignOptions),
	) (*v4.PresignedHTTPRequest, error)
	PresignGetObject(
		context.Context,
		*s3.GetObjectInput,
		...func(*s3.PresignOptions),
	) (*v4.PresignedHTTPRequest, error)
}

type objectStore interface {
	HeadObject(
		context.Context,
		*s3.HeadObjectInput,
		...func(*s3.Options),
	) (*s3.HeadObjectOutput, error)
}

func NewPresigner(cfg Config) (*Presigner, error) {
	if strings.TrimSpace(cfg.AccessKey) == "" ||
		strings.TrimSpace(cfg.Bucket) == "" ||
		strings.TrimSpace(cfg.Endpoint) == "" ||
		strings.TrimSpace(cfg.PublicURL) == "" ||
		strings.TrimSpace(cfg.Region) == "" ||
		strings.TrimSpace(cfg.SecretKey) == "" ||
		cfg.ExpiresIn <= 0 {
		return nil, ErrInvalidPresignerConfig
	}

	if _, err := url.ParseRequestURI(cfg.Endpoint); err != nil {
		return nil, ErrInvalidPresignerConfig
	}
	if _, err := url.ParseRequestURI(cfg.PublicURL); err != nil {
		return nil, ErrInvalidPresignerConfig
	}

	signingClient := newS3Client(cfg, cfg.PublicURL)
	objectClient := newS3Client(cfg, cfg.Endpoint)

	return &Presigner{
		bucket:      cfg.Bucket,
		expiresIn:   cfg.ExpiresIn,
		objectStore: objectClient,
		s3Presigner: s3.NewPresignClient(signingClient),
	}, nil
}

func newS3Client(cfg Config, endpoint string) *s3.Client {
	return s3.New(s3.Options{
		BaseEndpoint: aws.String(strings.TrimRight(endpoint, "/")),
		Credentials: credentials.NewStaticCredentialsProvider(
			cfg.AccessKey,
			cfg.SecretKey,
			"",
		),
		Region:       cfg.Region,
		UsePathStyle: true,
	})
}

func (presigner *Presigner) PresignPutObject(
	ctx context.Context,
	input PutObjectInput,
) (PresignedRequest, error) {
	if strings.TrimSpace(input.Key) == "" ||
		strings.TrimSpace(input.ContentType) == "" ||
		input.ContentLength < 0 {
		return PresignedRequest{}, ErrInvalidPresignerConfig
	}

	result, err := presigner.s3Presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(presigner.bucket),
		Key:           aws.String(input.Key),
		ContentLength: aws.Int64(input.ContentLength),
		ContentType:   aws.String(input.ContentType),
	}, s3.WithPresignExpires(presigner.expiresIn))
	if err != nil {
		return PresignedRequest{}, err
	}

	headers := make(map[string]string, len(result.SignedHeader))
	for key, values := range result.SignedHeader {
		if len(values) > 0 && !strings.EqualFold(key, "Host") {
			headers[key] = values[0]
		}
	}

	return PresignedRequest{
		Method:    result.Method,
		URL:       result.URL,
		ExpiresIn: presigner.expiresIn,
		Headers:   headers,
	}, nil
}

func (presigner *Presigner) PresignGetObject(
	ctx context.Context,
	input GetObjectInput,
) (PresignedRequest, error) {
	if strings.TrimSpace(input.Key) == "" {
		return PresignedRequest{}, ErrInvalidPresignerConfig
	}

	result, err := presigner.s3Presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(presigner.bucket),
		Key:    aws.String(input.Key),
	}, s3.WithPresignExpires(presigner.expiresIn))
	if err != nil {
		return PresignedRequest{}, err
	}

	headers := make(map[string]string, len(result.SignedHeader))
	for key, values := range result.SignedHeader {
		if len(values) > 0 && !strings.EqualFold(key, "Host") {
			headers[key] = values[0]
		}
	}

	return PresignedRequest{
		Method:    result.Method,
		URL:       result.URL,
		ExpiresIn: presigner.expiresIn,
		Headers:   headers,
	}, nil
}

func (presigner *Presigner) HeadObject(
	ctx context.Context,
	key string,
) (ObjectMetadata, error) {
	if strings.TrimSpace(key) == "" {
		return ObjectMetadata{}, ErrInvalidPresignerConfig
	}

	result, err := presigner.objectStore.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(presigner.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return ObjectMetadata{}, ErrObjectNotFound
		}
		return ObjectMetadata{}, err
	}

	return ObjectMetadata{
		ContentLength: aws.ToInt64(result.ContentLength),
		ContentType:   aws.ToString(result.ContentType),
		ETag:          aws.ToString(result.ETag),
	}, nil
}
