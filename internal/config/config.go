package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppName, Environment, HTTPPort, DatabaseURL, AccessSecret, RefreshSecret string
	AccessTTL, RefreshTTL                                                    time.Duration
	CORSOrigins, TrustedProxies                                              []string
	FrontendURL, MidtransServerKey, MidtransClientKey                        string
	MidtransIsProduction                                                     bool
}

func Load() (Config, error) {
	_ = godotenv.Load()
	accessTTL, err := time.ParseDuration(get("JWT_ACCESS_TTL", "15m"))
	if err != nil {
		return Config{}, fmt.Errorf("JWT_ACCESS_TTL: %w", err)
	}
	refreshTTL, err := time.ParseDuration(get("JWT_REFRESH_TTL", "168h"))
	if err != nil {
		return Config{}, fmt.Errorf("JWT_REFRESH_TTL: %w", err)
	}
	cfg := Config{
		AppName: get("APP_NAME", "BengkelOS"), Environment: get("APP_ENV", "development"),
		HTTPPort: get("HTTP_PORT", "8080"), DatabaseURL: os.Getenv("DATABASE_URL"),
		AccessSecret: os.Getenv("JWT_ACCESS_SECRET"), RefreshSecret: os.Getenv("JWT_REFRESH_SECRET"),
		AccessTTL: accessTTL, RefreshTTL: refreshTTL, CORSOrigins: split(get("CORS_ORIGINS", "http://localhost:3000")),
		TrustedProxies: split(os.Getenv("TRUSTED_PROXIES")), FrontendURL: get("FRONTEND_URL", "http://localhost:3000"),
		MidtransServerKey: os.Getenv("MIDTRANS_SERVER_KEY"), MidtransClientKey: os.Getenv("MIDTRANS_CLIENT_KEY"),
		MidtransIsProduction: get("MIDTRANS_IS_PRODUCTION", "false") == "true",
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if len(cfg.AccessSecret) < 32 || len(cfg.RefreshSecret) < 32 {
		return Config{}, fmt.Errorf("JWT secrets must contain at least 32 characters")
	}
	return cfg, nil
}

func get(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func split(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	items := strings.Split(value, ",")
	for i := range items {
		items[i] = strings.TrimSpace(items[i])
	}
	return items
}
