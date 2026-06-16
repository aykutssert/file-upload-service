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
)

var ErrInvalidPresignerConfig = errors.New("invalid storage presigner config")

type Presigner struct {
	bucket      string
	expiresIn   time.Duration
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

type PresignedRequest struct {
	Method    string
	URL       string
	ExpiresIn time.Duration
	Headers   map[string]string
}

type putObjectPresigner interface {
	PresignPutObject(
		context.Context,
		*s3.PutObjectInput,
		...func(*s3.PresignOptions),
	) (*v4.PresignedHTTPRequest, error)
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

	client := s3.New(s3.Options{
		BaseEndpoint: aws.String(strings.TrimRight(cfg.PublicURL, "/")),
		Credentials: credentials.NewStaticCredentialsProvider(
			cfg.AccessKey,
			cfg.SecretKey,
			"",
		),
		Region:       cfg.Region,
		UsePathStyle: true,
	})

	return &Presigner{
		bucket:      cfg.Bucket,
		expiresIn:   cfg.ExpiresIn,
		s3Presigner: s3.NewPresignClient(client),
	}, nil
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
