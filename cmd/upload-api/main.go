package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aykutssert/file-upload-service/internal/config"
	"github.com/aykutssert/file-upload-service/internal/database"
	"github.com/aykutssert/file-upload-service/internal/httpapi"
	"github.com/aykutssert/file-upload-service/internal/readiness"
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

	server := &http.Server{
		Addr:              cfg.Address(),
		Handler:           httpapi.NewRouter(checker),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server started", "address", server.Addr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		logger.Info("shutdown requested")
	}

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		shutdownTimeout,
	)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}

	logger.Info("server stopped")
}
