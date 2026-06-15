package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("UPLOAD_API_DATABASE_URL", "")
	t.Setenv("UPLOAD_API_HOST", "")
	t.Setenv("UPLOAD_API_PORT", "")

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
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("UPLOAD_API_DATABASE_URL", "postgres://example")
	t.Setenv("UPLOAD_API_HOST", "0.0.0.0")
	t.Setenv("UPLOAD_API_PORT", "9000")

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
