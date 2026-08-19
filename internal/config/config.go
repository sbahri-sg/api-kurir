package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultDatabaseURL = "postgres://api_kurir:api_kurir@localhost:55432/api_kurir?sslmode=disable"

type Config struct {
	AppEnv              string
	HTTPAddr            string
	DatabaseURL         string
	DatabaseMaxConn     int32
	DatabaseMinConn     int32
	APIKeys             []string
	AdminAPIKeys        []string
	ShutdownTimeout     time.Duration
	Redis               RedisConfig
	RajaOngkir          RajaOngkirConfig
	ProviderCredentials ProviderCredentialConfig
	TenantContext       TenantContextConfig
	Tracking            TrackingConfig
}

type RedisConfig struct {
	Enabled  bool
	Addr     string
	Password string
	DB       int
	Prefix   string
}

type RajaOngkirConfig struct {
	TrackingCouriers   []string
	APIKey             string
	BaseURL            string
	Timeout            time.Duration
	DailyLimit         int64
	CredentialAlias    string
	SnapshotTTL        time.Duration
	MinRequestInterval time.Duration
}

type TrackingConfig struct {
	Enabled       bool
	EncryptionKey string
	WorkerID      string
	Concurrency   int
	PollInterval  time.Duration
}

type ProviderCredentialConfig struct {
	EncryptionKey string
}

type TenantContextConfig struct {
	PublicKey string
	Issuer    string
	Audience  string
	MaxTTL    time.Duration
}

func Load() (Config, error) {
	maxConns, err := int32Env("DATABASE_MAX_CONNS", 20)
	if err != nil {
		return Config{}, err
	}
	minConns, err := int32Env("DATABASE_MIN_CONNS", 2)
	if err != nil {
		return Config{}, err
	}
	if minConns > maxConns {
		return Config{}, errors.New("DATABASE_MIN_CONNS cannot exceed DATABASE_MAX_CONNS")
	}

	shutdownTimeout, err := durationEnv("SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	redisEnabled, err := boolEnv("REDIS_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	redisDB, err := intEnv("REDIS_DB", 0)
	if err != nil {
		return Config{}, err
	}
	rajaOngkirTimeout, err := durationEnv("RAJAONGKIR_TIMEOUT", 4*time.Second)
	if err != nil {
		return Config{}, err
	}
	rajaOngkirDailyLimit, err := int64Env("RAJAONGKIR_DAILY_LIMIT", 50_000)
	if err != nil {
		return Config{}, err
	}
	rajaOngkirSnapshotTTL, err := durationEnv("RAJAONGKIR_SNAPSHOT_TTL", 14*24*time.Hour)
	if err != nil {
		return Config{}, err
	}
	rajaOngkirMinRequestInterval, err := durationEnv(
		"RAJAONGKIR_MIN_REQUEST_INTERVAL",
		250*time.Millisecond,
	)
	if err != nil {
		return Config{}, err
	}
	trackingEnabled, err := boolEnv("TRACKING_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	trackingPollInterval, err := durationEnv("TRACKING_POLL_INTERVAL", time.Second)
	if err != nil {
		return Config{}, err
	}
	trackingConcurrency, err := intEnv("TRACKING_WORKER_CONCURRENCY", 8)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		AppEnv:          envOr("APP_ENV", "development"),
		HTTPAddr:        envOr("HTTP_ADDR", ":8080"),
		DatabaseURL:     envOr("DATABASE_URL", defaultDatabaseURL),
		DatabaseMaxConn: maxConns,
		DatabaseMinConn: minConns,
		APIKeys:         splitCSV(os.Getenv("API_KEYS")),
		AdminAPIKeys:    splitCSV(os.Getenv("ADMIN_API_KEYS")),
		ShutdownTimeout: shutdownTimeout,
		Redis: RedisConfig{
			Enabled:  redisEnabled,
			Addr:     envOr("REDIS_ADDR", "localhost:6379"),
			Password: os.Getenv("REDIS_PASSWORD"),
			DB:       redisDB,
			Prefix:   envOr("REDIS_PREFIX", "api-kurir"),
		},
		RajaOngkir: RajaOngkirConfig{
			TrackingCouriers: splitCSV(envOr(
				"RAJAONGKIR_TRACKING_COURIERS",
				"jne,sap,ninja,jnt,tiki,wahana,pos,lion",
			)),
			APIKey:             strings.TrimSpace(os.Getenv("RAJAONGKIR_API_KEY")),
			BaseURL:            envOr("RAJAONGKIR_BASE_URL", "https://rajaongkir.komerce.id/api/v1/"),
			Timeout:            rajaOngkirTimeout,
			DailyLimit:         rajaOngkirDailyLimit,
			CredentialAlias:    envOr("RAJAONGKIR_CREDENTIAL_ALIAS", "primary"),
			SnapshotTTL:        rajaOngkirSnapshotTTL,
			MinRequestInterval: rajaOngkirMinRequestInterval,
		},
		ProviderCredentials: ProviderCredentialConfig{
			EncryptionKey: strings.TrimSpace(os.Getenv("PROVIDER_CREDENTIAL_ENCRYPTION_KEY")),
		},
		TenantContext: TenantContextConfig{
			PublicKey: strings.TrimSpace(os.Getenv("TENANT_CONTEXT_PUBLIC_KEY")),
			Issuer:    envOr("TENANT_CONTEXT_ISSUER", "emisell-api"),
			Audience:  envOr("TENANT_CONTEXT_AUDIENCE", "api-kurir"),
			MaxTTL:    5 * time.Minute,
		},
		Tracking: TrackingConfig{
			Enabled:       trackingEnabled,
			EncryptionKey: strings.TrimSpace(os.Getenv("TRACKING_ENCRYPTION_KEY")),
			WorkerID:      envOr("TRACKING_WORKER_ID", "worker-local"),
			Concurrency:   trackingConcurrency,
			PollInterval:  trackingPollInterval,
		},
	}

	if cfg.AppEnv == "production" && len(cfg.APIKeys) == 0 {
		return Config{}, errors.New("API_KEYS is required in production")
	}
	if cfg.AppEnv == "production" && len(cfg.AdminAPIKeys) == 0 {
		return Config{}, errors.New("ADMIN_API_KEYS is required in production")
	}
	if cfg.AppEnv != "production" && len(cfg.APIKeys) == 0 {
		cfg.APIKeys = []string{"dev-api-key"}
	}
	if cfg.AppEnv != "production" && len(cfg.AdminAPIKeys) == 0 {
		cfg.AdminAPIKeys = []string{"dev-emisell"}
	}
	if cfg.AppEnv != "production" && cfg.ProviderCredentials.EncryptionKey == "" {
		cfg.ProviderCredentials.EncryptionKey =
			"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	}
	if cfg.AppEnv == "production" && cfg.ProviderCredentials.EncryptionKey == "" {
		return Config{}, errors.New(
			"PROVIDER_CREDENTIAL_ENCRYPTION_KEY is required in production",
		)
	}
	if cfg.Tracking.Enabled && len(cfg.RajaOngkir.TrackingCouriers) == 0 {
		return Config{}, errors.New("RAJAONGKIR_TRACKING_COURIERS cannot be empty")
	}
	if cfg.RajaOngkir.DailyLimit <= 0 {
		return Config{}, errors.New("RAJAONGKIR_DAILY_LIMIT must be positive")
	}
	if cfg.RajaOngkir.Timeout <= 0 ||
		cfg.RajaOngkir.SnapshotTTL <= 0 ||
		cfg.RajaOngkir.MinRequestInterval < 10*time.Millisecond {
		return Config{}, errors.New(
			"RajaOngkir timeout/snapshot TTL must be positive and request interval at least 10ms",
		)
	}
	if cfg.Tracking.Enabled && cfg.Tracking.EncryptionKey == "" {
		return Config{}, errors.New("TRACKING_ENCRYPTION_KEY is required when TRACKING_ENABLED=true")
	}
	if cfg.Tracking.PollInterval <= 0 {
		return Config{}, errors.New("TRACKING_POLL_INTERVAL must be positive")
	}
	if cfg.Tracking.Concurrency < 1 || cfg.Tracking.Concurrency > 64 {
		return Config{}, errors.New("TRACKING_WORKER_CONCURRENCY must be between 1 and 64")
	}
	tenantContextMaxTTL, err := durationEnv("TENANT_CONTEXT_MAX_TTL", cfg.TenantContext.MaxTTL)
	if err != nil {
		return Config{}, err
	}
	if tenantContextMaxTTL <= 0 || tenantContextMaxTTL > 15*time.Minute {
		return Config{}, errors.New("TENANT_CONTEXT_MAX_TTL must be between 1ns and 15m")
	}
	cfg.TenantContext.MaxTTL = tenantContextMaxTTL
	return cfg, nil
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func intEnv(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return value, nil
}

func int32Env(key string, fallback int32) (int32, error) {
	value, err := intEnv(key, int(fallback))
	if err != nil {
		return 0, err
	}
	if value < 0 {
		return 0, fmt.Errorf("%s cannot be negative", key)
	}
	return int32(value), nil
}

func int64Env(key string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return value, nil
}

func boolEnv(key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false: %w", key, err)
	}
	return value, nil
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", key, err)
	}
	return value, nil
}
