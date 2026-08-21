package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/emisell/api-kurir/internal/admin"
	"github.com/emisell/api-kurir/internal/apikeys"
	"github.com/emisell/api-kurir/internal/config"
	"github.com/emisell/api-kurir/internal/couriers"
	"github.com/emisell/api-kurir/internal/database"
	"github.com/emisell/api-kurir/internal/httpapi"
	"github.com/emisell/api-kurir/internal/locations"
	"github.com/emisell/api-kurir/internal/merchantproviders"
	"github.com/emisell/api-kurir/internal/merchantshipping"
	"github.com/emisell/api-kurir/internal/platform/cache"
	"github.com/emisell/api-kurir/internal/providercredentials"
	"github.com/emisell/api-kurir/internal/providers/biteship"
	"github.com/emisell/api-kurir/internal/providers/rajaongkir"
	"github.com/emisell/api-kurir/internal/rates"
	"github.com/emisell/api-kurir/internal/tenancy"
	"github.com/emisell/api-kurir/internal/tracking"
	"github.com/emisell/api-kurir/internal/webhooksettings"
	"github.com/labstack/echo/v5"
	"github.com/redis/go-redis/v9"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("API stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.Open(
		rootCtx,
		cfg.DatabaseURL,
		cfg.DatabaseMinConn,
		cfg.DatabaseMaxConn,
	)
	if err != nil {
		return err
	}
	defer pool.Close()

	var redisClient *redis.Client
	var runtimeCache cache.Cache = cache.NewMemory(10_000)
	var runtimeLocker cache.Locker = cache.NewPostgresLocker(pool)
	if cfg.Redis.Enabled {
		redisClient = redis.NewClient(&redis.Options{
			Addr:     cfg.Redis.Addr,
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		})
		pingCtx, cancel := context.WithTimeout(rootCtx, 2*time.Second)
		defer cancel()
		if err := redisClient.Ping(pingCtx).Err(); err != nil {
			return err
		}
		redisAdapter := cache.NewRedis(redisClient, cfg.Redis.Prefix)
		runtimeCache = redisAdapter
		runtimeLocker = redisAdapter
		defer redisClient.Close()
		logger.Info("Redis coordination enabled")
	} else {
		logger.Info("Redis disabled; using memory cache and PostgreSQL locks")
	}
	locationRepository := locations.NewPostgresRepository(pool)
	rateRepository := rates.NewPostgresRepository(pool)
	courierRepository := couriers.NewPostgresRepository(pool)
	merchantShippingService := merchantshipping.NewService(
		merchantshipping.NewPostgresRepository(pool),
		courierRepository,
	)
	merchantProviderService := merchantproviders.NewService(
		merchantproviders.NewPostgresRepository(pool),
	)
	providerCredentialCipher, err := providercredentials.NewCipher(
		cfg.ProviderCredentials.EncryptionKey,
	)
	if err != nil {
		return err
	}
	providerCredentialService := providercredentials.NewService(
		providercredentials.NewPostgresRepository(pool),
		providerCredentialCipher,
		providercredentials.NewValidatorRegistry(map[string]providercredentials.Validator{
			"rajaongkir": rajaongkir.NewCredentialValidator(
				cfg.RajaOngkir.BaseURL,
				cfg.RajaOngkir.Timeout,
				cfg.RajaOngkir.MinRequestInterval,
			),
			"biteship": biteship.NewCredentialValidator(
				cfg.Biteship.BaseURL,
				cfg.Biteship.Timeout,
			),
		}),
	)
	webhookSettingsService := webhooksettings.NewService(
		webhooksettings.NewPostgresRepository(pool),
		providerCredentialCipher,
		cfg.AppEnv,
		webhooksettings.Fallback{
			Enabled:     cfg.Tracking.WebhookEnabled,
			CallbackURL: cfg.Tracking.WebhookURL,
			Secret:      cfg.Tracking.WebhookSecret,
		},
	)
	tenantVerifier, err := tenancy.NewVerifier(
		cfg.TenantContext.PublicKey,
		cfg.TenantContext.Issuer,
		cfg.TenantContext.Audience,
		cfg.TenantContext.MaxTTL,
	)
	if err != nil {
		return err
	}
	if tenantVerifier == nil {
		logger.Warn("tenant context verification disabled; merchant credential routes are unavailable")
	} else {
		logger.Info("tenant context verification enabled", "issuer", cfg.TenantContext.Issuer)
	}
	fallbacks := make(map[string]providercredentials.StaticCredential)
	if cfg.RajaOngkir.APIKey != "" {
		fallbacks["rajaongkir"] = providercredentials.StaticCredential{
			Secret:          cfg.RajaOngkir.APIKey,
			CredentialAlias: cfg.RajaOngkir.CredentialAlias,
			DailyLimit:      cfg.RajaOngkir.DailyLimit,
		}
	}
	providerResolver := providercredentials.NewStaticFallbackResolver(
		providerCredentialService,
		fallbacks,
	)
	rajaOngkirProvider := rajaongkir.NewDynamicProvider(
		providerResolver,
		cfg.RajaOngkir.BaseURL,
		cfg.RajaOngkir.Timeout,
		cfg.RajaOngkir.MinRequestInterval,
		locationRepository,
		rateRepository,
		cfg.RajaOngkir.SnapshotTTL,
	)
	rateOptions := []rates.Option{rates.WithProviderFallback(
		rajaOngkirProvider,
		rateRepository,
		runtimeLocker,
		cfg.RajaOngkir.Timeout+2*time.Second,
	), rates.WithResultPolicy(merchantShippingService), rates.WithServicePolicyCache(
		runtimeCache,
		5*time.Minute,
	)}
	operationTimeout := cfg.RajaOngkir.Timeout + 2*time.Second
	logger.Info("runtime RajaOngkir credential resolver enabled")
	rateService := rates.NewService(rateRepository, operationTimeout, rateOptions...)
	adminRepository := admin.NewPostgresRepository(pool)
	customerAPIKeyService := apikeys.NewService(apikeys.NewPostgresRepository(pool))
	var trackingService *tracking.Service
	var immediateTrackingAdapter tracking.Adapter
	if cfg.Tracking.Enabled {
		trackingCipher, err := tracking.NewCipher(cfg.Tracking.EncryptionKey)
		if err != nil {
			return err
		}
		rajaOngkirTrackingAdapter := rajaongkir.NewDynamicTrackingAdapter(
			providerResolver,
			cfg.RajaOngkir.BaseURL,
			cfg.RajaOngkir.Timeout,
			cfg.RajaOngkir.MinRequestInterval,
			rateRepository,
			rateRepository,
			cfg.RajaOngkir.TrackingCouriers,
		)
		biteshipTrackingAdapter := biteship.NewDynamicTrackingAdapter(
			providerResolver,
			cfg.Biteship.BaseURL,
			cfg.Biteship.Timeout,
			rateRepository,
			rateRepository,
			cfg.Biteship.TrackingCouriers,
		)
		immediateTrackingAdapter = tracking.NewFallbackAdapter(
			rajaOngkirTrackingAdapter,
			biteshipTrackingAdapter,
		)
		trackingService = tracking.NewService(
			tracking.NewPostgresRepository(pool),
			trackingCipher,
			immediateTrackingAdapter.CourierCodes()...,
		)
		logger.Info(
			"tracking registration enabled; waybills encrypted at application layer",
			"rajaongkir_couriers", len(cfg.RajaOngkir.TrackingCouriers),
			"biteship_fallback_couriers", len(cfg.Biteship.TrackingCouriers),
		)
	}

	server := httpapi.New(
		pool,
		rateService,
		locationRepository,
		courierRepository,
		adminRepository,
		trackingService,
		immediateTrackingAdapter,
		customerAPIKeyService,
		providerCredentialService,
		webhookSettingsService,
		merchantProviderService,
		merchantShippingService,
		tenantVerifier,
		cfg.APIKeys,
		cfg.AdminAPIKeys,
		logger,
		cfg.AppEnv,
	)

	logger.Info("API listening", "address", cfg.HTTPAddr)
	startConfig := echo.StartConfig{
		Address:         cfg.HTTPAddr,
		HideBanner:      true,
		HidePort:        true,
		GracefulTimeout: cfg.ShutdownTimeout,
	}
	return startConfig.Start(rootCtx, server.Echo)
}
