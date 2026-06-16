package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aykutssert/file-upload-service/internal/auth"
	"github.com/aykutssert/file-upload-service/internal/config"
	"github.com/aykutssert/file-upload-service/internal/database"
	"github.com/aykutssert/file-upload-service/internal/files"
	"github.com/aykutssert/file-upload-service/internal/httpapi"
	"github.com/aykutssert/file-upload-service/internal/httpserver"
	"github.com/aykutssert/file-upload-service/internal/readiness"
	"github.com/aykutssert/file-upload-service/internal/storage"
)

const shutdownTimeout = 10 * time.Second

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}

	pool, err := database.Open(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Error("create database pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	checker := readiness.New(
		pool,
		readiness.NewHTTPChecker(http.DefaultClient, cfg.SeaweedFSHealthURL),
		readiness.NewHTTPChecker(http.DefaultClient, cfg.NATSHealthURL),
	)
	presigner, err := storage.NewPresigner(storage.Config{
		AccessKey: cfg.SeaweedFSAccessKey,
		Bucket:    cfg.SeaweedFSBucket,
		Endpoint:  cfg.SeaweedFSS3URL,
		ExpiresIn: time.Duration(cfg.PresignTTLSeconds) * time.Second,
		PublicURL: cfg.SeaweedFSPublicURL,
		Region:    cfg.SeaweedFSRegion,
		SecretKey: cfg.SeaweedFSSecretKey,
	})
	if err != nil {
		logger.Error("create storage presigner", "error", err)
		os.Exit(1)
	}

	server := &http.Server{
		Handler: httpapi.NewRouter(
			checker,
			auth.NewPostgreSQLResolver(pool),
			files.NewRepository(pool),
			presigner,
			auth.NewKeyCreator(pool),
			auth.NewKeyRevoker(pool),
			files.NewMultipartRepository(pool),
			presigner,
		),
		ReadHeaderTimeout: 5 * time.Second,
	}

	listener, err := net.Listen("tcp", cfg.Address())
	if err != nil {
		logger.Error("listen", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	if err := httpserver.Run(ctx, server, listener, shutdownTimeout, logger); err != nil {
		logger.Error("run server", "error", err)
		os.Exit(1)
	}
}
