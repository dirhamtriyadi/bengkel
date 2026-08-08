package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppName, Environment, HTTPPort, DatabaseURL, AccessSecret, RefreshSecret string
	AccessTTL, RefreshTTL                                                    time.Duration
	CORSOrigins, TrustedProxies                                              []string
	FrontendURL                                                              string
	PublicInvoiceTTL                                                         time.Duration
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
	publicInvoiceTTL, err := time.ParseDuration(get("PUBLIC_INVOICE_TTL", "168h"))
	if err != nil {
		return Config{}, fmt.Errorf("PUBLIC_INVOICE_TTL: %w", err)
	}
	cfg := Config{
		AppName: get("APP_NAME", "BengkelOS"), Environment: get("APP_ENV", "development"),
		HTTPPort: get("HTTP_PORT", "8080"), DatabaseURL: os.Getenv("DATABASE_URL"),
		AccessSecret: os.Getenv("JWT_ACCESS_SECRET"), RefreshSecret: os.Getenv("JWT_REFRESH_SECRET"),
		AccessTTL: accessTTL, RefreshTTL: refreshTTL, CORSOrigins: split(get("CORS_ORIGINS", "http://localhost:3000")),
		TrustedProxies: split(os.Getenv("TRUSTED_PROXIES")), FrontendURL: get("FRONTEND_URL", "http://localhost:3000"),
		PublicInvoiceTTL: publicInvoiceTTL,
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if len(cfg.AccessSecret) < 32 || len(cfg.RefreshSecret) < 32 {
		return Config{}, fmt.Errorf("JWT secrets must contain at least 32 characters")
	}
	frontend, parseErr := url.Parse(cfg.FrontendURL)
	if parseErr != nil || (frontend.Scheme != "http" && frontend.Scheme != "https") || frontend.Host == "" {
		return Config{}, fmt.Errorf("FRONTEND_URL must be an absolute http(s) URL")
	}
	if cfg.PublicInvoiceTTL < 5*time.Minute || cfg.PublicInvoiceTTL > 30*24*time.Hour {
		return Config{}, fmt.Errorf("PUBLIC_INVOICE_TTL must be between 5m and 720h")
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
