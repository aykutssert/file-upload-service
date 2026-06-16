package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
)

const (
	defaultDatabaseURL        = "postgres://file_upload:local-development-only@127.0.0.1:5432/file_upload?sslmode=disable"
	defaultHost               = "127.0.0.1"
	defaultNATSHealthURL      = "http://127.0.0.1:8222/healthz"
	defaultPort               = 8080
	defaultPresignTTLSeconds  = 900
	defaultSeaweedFSAccessKey = "local-access-key"
	defaultSeaweedFSBucket    = "file-upload"
	defaultSeaweedFSHealthURL = "http://127.0.0.1:9333/cluster/status"
	defaultSeaweedFSRegion    = "local"
	defaultSeaweedFSSecretKey = "local-secret-key"
	defaultSeaweedFSS3URL     = "http://127.0.0.1:8333"

	DefaultMaxSinglePartBytes int64 = 100 * 1024 * 1024      // 100 MB
	DefaultMaxMultipartBytes  int64 = 5 * 1024 * 1024 * 1024 // 5 GB
)

type Config struct {
	DatabaseURL        string
	Host               string
	NATSHealthURL      string
	Port               int
	PresignTTLSeconds  int
	SeaweedFSAccessKey string
	SeaweedFSBucket    string
	SeaweedFSHealthURL string
	SeaweedFSPublicURL string
	SeaweedFSRegion    string
	SeaweedFSS3URL     string
	SeaweedFSSecretKey string
	MaxSinglePartBytes int64
	MaxMultipartBytes  int64
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL: valueOrDefault(
			"UPLOAD_API_DATABASE_URL",
			defaultDatabaseURL,
		),
		Host: valueOrDefault("UPLOAD_API_HOST", defaultHost),
		NATSHealthURL: valueOrDefault(
			"UPLOAD_API_NATS_HEALTH_URL",
			defaultNATSHealthURL,
		),
		Port:              defaultPort,
		PresignTTLSeconds: defaultPresignTTLSeconds,
		SeaweedFSAccessKey: valueOrDefault(
			"UPLOAD_API_SEAWEEDFS_ACCESS_KEY",
			defaultSeaweedFSAccessKey,
		),
		SeaweedFSBucket: valueOrDefault(
			"UPLOAD_API_SEAWEEDFS_BUCKET",
			defaultSeaweedFSBucket,
		),
		SeaweedFSHealthURL: valueOrDefault(
			"UPLOAD_API_SEAWEEDFS_HEALTH_URL",
			defaultSeaweedFSHealthURL,
		),
		SeaweedFSPublicURL: valueOrDefault(
			"UPLOAD_API_SEAWEEDFS_PUBLIC_URL",
			valueOrDefault("UPLOAD_API_SEAWEEDFS_S3_URL", defaultSeaweedFSS3URL),
		),
		SeaweedFSRegion: valueOrDefault(
			"UPLOAD_API_SEAWEEDFS_REGION",
			defaultSeaweedFSRegion,
		),
		SeaweedFSS3URL: valueOrDefault(
			"UPLOAD_API_SEAWEEDFS_S3_URL",
			defaultSeaweedFSS3URL,
		),
		SeaweedFSSecretKey: valueOrDefault(
			"UPLOAD_API_SEAWEEDFS_SECRET_KEY",
			defaultSeaweedFSSecretKey,
		),
		MaxSinglePartBytes: DefaultMaxSinglePartBytes,
		MaxMultipartBytes:  DefaultMaxMultipartBytes,
	}

	if value := os.Getenv("UPLOAD_API_PORT"); value != "" {
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return Config{}, fmt.Errorf(
				"UPLOAD_API_PORT must be an integer between 1 and 65535",
			)
		}
		cfg.Port = port
	}
	if value := os.Getenv("UPLOAD_API_PRESIGN_TTL_SECONDS"); value != "" {
		ttlSeconds, err := strconv.Atoi(value)
		if err != nil || ttlSeconds < 1 || ttlSeconds > 3600 {
			return Config{}, fmt.Errorf(
				"UPLOAD_API_PRESIGN_TTL_SECONDS must be an integer between 1 and 3600",
			)
		}
		cfg.PresignTTLSeconds = ttlSeconds
	}
	if value := os.Getenv("UPLOAD_API_MAX_SINGLE_PART_BYTES"); value != "" {
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil || n < 1 {
			return Config{}, fmt.Errorf("UPLOAD_API_MAX_SINGLE_PART_BYTES must be a positive integer")
		}
		cfg.MaxSinglePartBytes = n
	}
	if value := os.Getenv("UPLOAD_API_MAX_MULTIPART_BYTES"); value != "" {
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil || n < 1 {
			return Config{}, fmt.Errorf("UPLOAD_API_MAX_MULTIPART_BYTES must be a positive integer")
		}
		cfg.MaxMultipartBytes = n
	}

	return cfg, nil
}

func (c Config) Address() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

func valueOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
