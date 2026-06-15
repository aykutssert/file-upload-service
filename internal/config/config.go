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
	defaultSeaweedFSHealthURL = "http://127.0.0.1:9333/cluster/status"
)

type Config struct {
	DatabaseURL        string
	Host               string
	NATSHealthURL      string
	Port               int
	SeaweedFSHealthURL string
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
		Port: defaultPort,
		SeaweedFSHealthURL: valueOrDefault(
			"UPLOAD_API_SEAWEEDFS_HEALTH_URL",
			defaultSeaweedFSHealthURL,
		),
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
