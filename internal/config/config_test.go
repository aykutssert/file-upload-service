package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("UPLOAD_API_DATABASE_URL", "")
	t.Setenv("UPLOAD_API_HOST", "")
	t.Setenv("UPLOAD_API_NATS_HEALTH_URL", "")
	t.Setenv("UPLOAD_API_PRESIGN_TTL_SECONDS", "")
	t.Setenv("UPLOAD_API_PORT", "")
	t.Setenv("UPLOAD_API_SEAWEEDFS_ACCESS_KEY", "")
	t.Setenv("UPLOAD_API_SEAWEEDFS_BUCKET", "")
	t.Setenv("UPLOAD_API_SEAWEEDFS_HEALTH_URL", "")
	t.Setenv("UPLOAD_API_SEAWEEDFS_PUBLIC_URL", "")
	t.Setenv("UPLOAD_API_SEAWEEDFS_REGION", "")
	t.Setenv("UPLOAD_API_SEAWEEDFS_S3_URL", "")
	t.Setenv("UPLOAD_API_SEAWEEDFS_SECRET_KEY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Address() != "127.0.0.1:8080" {
		t.Fatalf("Address() = %q", cfg.Address())
	}
	if cfg.DatabaseURL != defaultDatabaseURL {
		t.Fatalf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.NATSHealthURL != defaultNATSHealthURL {
		t.Fatalf("NATSHealthURL = %q", cfg.NATSHealthURL)
	}
	if cfg.PresignTTLSeconds != defaultPresignTTLSeconds {
		t.Fatalf("PresignTTLSeconds = %d", cfg.PresignTTLSeconds)
	}
	if cfg.SeaweedFSAccessKey != defaultSeaweedFSAccessKey {
		t.Fatalf("SeaweedFSAccessKey = %q", cfg.SeaweedFSAccessKey)
	}
	if cfg.SeaweedFSBucket != defaultSeaweedFSBucket {
		t.Fatalf("SeaweedFSBucket = %q", cfg.SeaweedFSBucket)
	}
	if cfg.SeaweedFSHealthURL != defaultSeaweedFSHealthURL {
		t.Fatalf("SeaweedFSHealthURL = %q", cfg.SeaweedFSHealthURL)
	}
	if cfg.SeaweedFSPublicURL != defaultSeaweedFSS3URL {
		t.Fatalf("SeaweedFSPublicURL = %q", cfg.SeaweedFSPublicURL)
	}
	if cfg.SeaweedFSRegion != defaultSeaweedFSRegion {
		t.Fatalf("SeaweedFSRegion = %q", cfg.SeaweedFSRegion)
	}
	if cfg.SeaweedFSS3URL != defaultSeaweedFSS3URL {
		t.Fatalf("SeaweedFSS3URL = %q", cfg.SeaweedFSS3URL)
	}
	if cfg.SeaweedFSSecretKey != defaultSeaweedFSSecretKey {
		t.Fatalf("SeaweedFSSecretKey = %q", cfg.SeaweedFSSecretKey)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("UPLOAD_API_DATABASE_URL", "postgres://example")
	t.Setenv("UPLOAD_API_HOST", "0.0.0.0")
	t.Setenv("UPLOAD_API_NATS_HEALTH_URL", "http://nats/health")
	t.Setenv("UPLOAD_API_PRESIGN_TTL_SECONDS", "60")
	t.Setenv("UPLOAD_API_PORT", "9000")
	t.Setenv("UPLOAD_API_SEAWEEDFS_ACCESS_KEY", "access")
	t.Setenv("UPLOAD_API_SEAWEEDFS_BUCKET", "bucket")
	t.Setenv("UPLOAD_API_SEAWEEDFS_HEALTH_URL", "http://storage/health")
	t.Setenv("UPLOAD_API_SEAWEEDFS_PUBLIC_URL", "http://localhost:8333")
	t.Setenv("UPLOAD_API_SEAWEEDFS_REGION", "region")
	t.Setenv("UPLOAD_API_SEAWEEDFS_S3_URL", "http://seaweedfs:8333")
	t.Setenv("UPLOAD_API_SEAWEEDFS_SECRET_KEY", "secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Address() != "0.0.0.0:9000" {
		t.Fatalf("Address() = %q", cfg.Address())
	}
	if cfg.DatabaseURL != "postgres://example" {
		t.Fatalf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.NATSHealthURL != "http://nats/health" {
		t.Fatalf("NATSHealthURL = %q", cfg.NATSHealthURL)
	}
	if cfg.PresignTTLSeconds != 60 {
		t.Fatalf("PresignTTLSeconds = %d", cfg.PresignTTLSeconds)
	}
	if cfg.SeaweedFSAccessKey != "access" {
		t.Fatalf("SeaweedFSAccessKey = %q", cfg.SeaweedFSAccessKey)
	}
	if cfg.SeaweedFSBucket != "bucket" {
		t.Fatalf("SeaweedFSBucket = %q", cfg.SeaweedFSBucket)
	}
	if cfg.SeaweedFSHealthURL != "http://storage/health" {
		t.Fatalf("SeaweedFSHealthURL = %q", cfg.SeaweedFSHealthURL)
	}
	if cfg.SeaweedFSPublicURL != "http://localhost:8333" {
		t.Fatalf("SeaweedFSPublicURL = %q", cfg.SeaweedFSPublicURL)
	}
	if cfg.SeaweedFSRegion != "region" {
		t.Fatalf("SeaweedFSRegion = %q", cfg.SeaweedFSRegion)
	}
	if cfg.SeaweedFSS3URL != "http://seaweedfs:8333" {
		t.Fatalf("SeaweedFSS3URL = %q", cfg.SeaweedFSS3URL)
	}
	if cfg.SeaweedFSSecretKey != "secret" {
		t.Fatalf("SeaweedFSSecretKey = %q", cfg.SeaweedFSSecretKey)
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	for _, value := range []string{"invalid", "0", "65536"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("UPLOAD_API_PORT", value)

			_, err := Load()
			if err == nil {
				t.Fatal("Load() error = nil")
			}
		})
	}
}

func TestLoadRejectsInvalidPresignTTL(t *testing.T) {
	for _, value := range []string{"invalid", "0", "3601"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("UPLOAD_API_PRESIGN_TTL_SECONDS", value)

			_, err := Load()
			if err == nil {
				t.Fatal("Load() error = nil")
			}
		})
	}
}
