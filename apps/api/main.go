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
		merchantshipping.WithSelectionLimits(
			cfg.MerchantShipping.MaxSelectedCouriers,
			cfg.MerchantShipping.MaxSelectedServices,
		),
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
		merchantProviderService,
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
	rateLockTTL := cfg.RajaOngkir.Timeout + cfg.Biteship.Timeout + 2*time.Second
	var rateProviderOption rates.Option
	if cfg.Biteship.RateFallbackEnabled {
		biteshipRateProvider := biteship.NewDynamicRateProvider(
			providerResolver,
			cfg.Biteship.BaseURL,
			cfg.Biteship.Timeout,
			locationRepository,
			rateRepository,
			rateRepository,
			cfg.Biteship.RateSnapshotTTL,
		)
		rateProviderOption = rates.WithProviderFallbackChain(
			rajaOngkirProvider,
			rateRepository,
			runtimeLocker,
			rateLockTTL,
			merchantProviderService,
			biteshipRateProvider,
		)
	} else {
		rateProviderOption = rates.WithProviderFallback(
			rajaOngkirProvider,
			rateRepository,
			runtimeLocker,
			cfg.RajaOngkir.Timeout+2*time.Second,
		)
	}
	rateOptions := []rates.Option{rateProviderOption, rates.WithCredentialSelector(providerCredentialService), rates.WithShippingProviderGate(merchantProviderService), rates.WithResultPolicy(merchantShippingService), rates.WithServicePolicyCache(
		runtimeCache,
		5*time.Minute,
	)}
	operationTimeout := cfg.RajaOngkir.Timeout + 2*time.Second
	if cfg.Biteship.RateFallbackEnabled {
		operationTimeout += 2*cfg.Biteship.Timeout + 2*time.Second
	}
	logger.Info(
		"runtime shipping provider resolver enabled",
		"primary", "rajaongkir",
		"rate_fallback", cfg.Biteship.RateFallbackEnabled,
	)
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
		trackingRepository := tracking.NewPostgresRepository(pool)
		migratedWaybills, err := trackingRepository.MigrateLegacyWaybills(
			rootCtx,
			trackingCipher,
		)
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
			biteship.EmisellTrackingFallbackCouriers(
				cfg.RajaOngkir.TrackingCouriers,
				cfg.Biteship.TrackingCouriers,
			),
		)
		immediateTrackingAdapter = tracking.NewPolicyFallbackAdapter(
			merchantProviderService,
			rajaOngkirTrackingAdapter,
			biteshipTrackingAdapter,
		)
		trackingService = tracking.NewService(
			trackingRepository,
			trackingCipher,
			immediateTrackingAdapter.CourierCodes()...,
		)
		logger.Info(
			"tracking registration enabled; waybills stored as plaintext for Emisell",
			"rajaongkir_couriers", len(cfg.RajaOngkir.TrackingCouriers),
			"biteship_fallback_couriers", len(biteshipTrackingAdapter.CourierCodes()),
			"migrated_legacy_waybills", migratedWaybills,
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
