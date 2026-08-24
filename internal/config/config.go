package config

import (
	"errors"
	"fmt"
	"net/url"
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
	Biteship            BiteshipConfig
	ProviderCredentials ProviderCredentialConfig
	MerchantShipping    MerchantShippingConfig
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

type BiteshipConfig struct {
	BaseURL             string
	Timeout             time.Duration
	TrackingCouriers    []string
	RateFallbackEnabled bool
	RateSnapshotTTL     time.Duration
}

type TrackingConfig struct {
	Enabled             bool
	EncryptionKey       string
	WorkerID            string
	Concurrency         int
	PollInterval        time.Duration
	WebhookEnabled      bool
	WebhookURL          string
	WebhookSecret       string
	WebhookTimeout      time.Duration
	WebhookConcurrency  int
	WebhookPollInterval time.Duration
}

type ProviderCredentialConfig struct {
	EncryptionKey string
}

type MerchantShippingConfig struct {
	MaxSelectedCouriers int
	MaxSelectedServices int
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
	biteshipTimeout, err := durationEnv("BITESHIP_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	biteshipRateFallbackEnabled, err := boolEnv(
		"BITESHIP_RATE_FALLBACK_ENABLED",
		true,
	)
	if err != nil {
		return Config{}, err
	}
	biteshipRateSnapshotTTL, err := durationEnv(
		"BITESHIP_RATE_SNAPSHOT_TTL",
		14*24*time.Hour,
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
	trackingWebhookEnabled, err := boolEnv("TRACKING_WEBHOOK_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	trackingWebhookTimeout, err := durationEnv("TRACKING_WEBHOOK_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	trackingWebhookConcurrency, err := intEnv("TRACKING_WEBHOOK_CONCURRENCY", 4)
	if err != nil {
		return Config{}, err
	}
	trackingWebhookPollInterval, err := durationEnv("TRACKING_WEBHOOK_POLL_INTERVAL", time.Second)
	if err != nil {
		return Config{}, err
	}
	maxSelectedCouriers, err := intEnv("MERCHANT_SHIPPING_MAX_COURIERS", 5)
	if err != nil {
		return Config{}, err
	}
	maxSelectedServices, err := intEnv("MERCHANT_SHIPPING_MAX_SERVICES", 20)
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
				"jne,sap,ninja,jnt,tiki,wahana,pos,lion,anteraja",
			)),
			APIKey:             strings.TrimSpace(os.Getenv("RAJAONGKIR_API_KEY")),
			BaseURL:            envOr("RAJAONGKIR_BASE_URL", "https://rajaongkir.komerce.id/api/v1/"),
			Timeout:            rajaOngkirTimeout,
			DailyLimit:         rajaOngkirDailyLimit,
			CredentialAlias:    envOr("RAJAONGKIR_CREDENTIAL_ALIAS", "primary"),
			SnapshotTTL:        rajaOngkirSnapshotTTL,
			MinRequestInterval: rajaOngkirMinRequestInterval,
		},
		Biteship: BiteshipConfig{
			BaseURL:             envOr("BITESHIP_BASE_URL", "https://api.biteship.com/"),
			Timeout:             biteshipTimeout,
			RateFallbackEnabled: biteshipRateFallbackEnabled,
			RateSnapshotTTL:     biteshipRateSnapshotTTL,
			TrackingCouriers: splitCSV(envOr(
				"BITESHIP_TRACKING_COURIERS",
				"ide,rpx,sentral,sicepat",
			)),
		},
		ProviderCredentials: ProviderCredentialConfig{
			EncryptionKey: strings.TrimSpace(os.Getenv("PROVIDER_CREDENTIAL_ENCRYPTION_KEY")),
		},
		MerchantShipping: MerchantShippingConfig{
			MaxSelectedCouriers: maxSelectedCouriers,
			MaxSelectedServices: maxSelectedServices,
		},
		Tracking: TrackingConfig{
			Enabled:             trackingEnabled,
			EncryptionKey:       strings.TrimSpace(os.Getenv("TRACKING_ENCRYPTION_KEY")),
			WorkerID:            envOr("TRACKING_WORKER_ID", "worker-local"),
			Concurrency:         trackingConcurrency,
			PollInterval:        trackingPollInterval,
			WebhookEnabled:      trackingWebhookEnabled,
			WebhookURL:          strings.TrimSpace(os.Getenv("EMISELL_TRACKING_WEBHOOK_URL")),
			WebhookSecret:       strings.TrimSpace(os.Getenv("EMISELL_TRACKING_WEBHOOK_SECRET")),
			WebhookTimeout:      trackingWebhookTimeout,
			WebhookConcurrency:  trackingWebhookConcurrency,
			WebhookPollInterval: trackingWebhookPollInterval,
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
	if cfg.Biteship.Timeout <= 0 || cfg.Biteship.RateSnapshotTTL <= 0 {
		return Config{}, errors.New("Biteship timeout and rate snapshot TTL must be positive")
	}
	if cfg.MerchantShipping.MaxSelectedCouriers < 1 ||
		cfg.MerchantShipping.MaxSelectedCouriers > 50 {
		return Config{}, errors.New("MERCHANT_SHIPPING_MAX_COURIERS must be between 1 and 50")
	}
	if cfg.MerchantShipping.MaxSelectedServices < 1 ||
		cfg.MerchantShipping.MaxSelectedServices > 200 {
		return Config{}, errors.New("MERCHANT_SHIPPING_MAX_SERVICES must be between 1 and 200")
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
	if cfg.Tracking.WebhookEnabled {
		if cfg.Tracking.WebhookURL == "" || cfg.Tracking.WebhookSecret == "" {
			return Config{}, errors.New(
				"EMISELL_TRACKING_WEBHOOK_URL and EMISELL_TRACKING_WEBHOOK_SECRET are required when TRACKING_WEBHOOK_ENABLED=true",
			)
		}
		if cfg.AppEnv == "production" && !strings.HasPrefix(cfg.Tracking.WebhookURL, "https://") {
			return Config{}, errors.New("EMISELL_TRACKING_WEBHOOK_URL must use https in production")
		}
		webhookURL, parseErr := url.ParseRequestURI(cfg.Tracking.WebhookURL)
		if parseErr != nil || webhookURL.Host == "" ||
			(webhookURL.Scheme != "http" && webhookURL.Scheme != "https") {
			return Config{}, errors.New("EMISELL_TRACKING_WEBHOOK_URL must be a valid absolute HTTP URL")
		}
		if len(cfg.Tracking.WebhookSecret) < 32 {
			return Config{}, errors.New("EMISELL_TRACKING_WEBHOOK_SECRET must be at least 32 characters")
		}
	}
	if cfg.Tracking.WebhookTimeout <= 0 || cfg.Tracking.WebhookPollInterval <= 0 {
		return Config{}, errors.New("tracking webhook timeout and poll interval must be positive")
	}
	if cfg.Tracking.WebhookConcurrency < 1 || cfg.Tracking.WebhookConcurrency > 32 {
		return Config{}, errors.New("TRACKING_WEBHOOK_CONCURRENCY must be between 1 and 32")
	}
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
