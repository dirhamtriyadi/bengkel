package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bengkel/internal/config"
	"bengkel/internal/database"
	httpapi "bengkel/internal/http"

	"go.uber.org/zap"
)

// @title BengkelOS API
// @version 1.0
// @description API operasional bengkel, kasir, stok, pembayaran, akuntansi, audit, dan CMS.
// @BasePath /api/v1
// @schemes http https
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Masukkan "Bearer {access_token}".
func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	logger, err := zap.NewProduction()
	if cfg.Environment == "development" {
		logger, err = zap.NewDevelopment()
	}
	if err != nil {
		panic(err)
	}
	defer logger.Sync() //nolint:errcheck

	db, err := database.Open(cfg.DatabaseURL, cfg.Environment == "development")
	if err != nil {
		logger.Fatal("database connection failed", zap.Error(err))
	}
	router := httpapi.NewRouter(cfg, db, logger)
	server := &http.Server{
		Addr: ":" + cfg.HTTPPort, Handler: router,
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 20 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 90 * time.Second,
	}
	go func() {
		logger.Info("api listening", zap.String("port", cfg.HTTPPort))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("http server failed", zap.Error(err))
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", zap.Error(err))
	}
}
