package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/emisell/api-kurir/internal/config"
	"github.com/emisell/api-kurir/internal/database"
	"github.com/emisell/api-kurir/internal/fulfillment"
	"github.com/emisell/api-kurir/internal/merchantproviders"
	"github.com/emisell/api-kurir/internal/providercredentials"
	"github.com/emisell/api-kurir/internal/providers/biteship"
	"github.com/emisell/api-kurir/internal/providers/hosted"
	"github.com/emisell/api-kurir/internal/rates"
	"github.com/emisell/api-kurir/internal/tracking"
	"github.com/emisell/api-kurir/internal/webhooksettings"
	"golang.org/x/sync/errgroup"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.Open(ctx, cfg.DatabaseURL, cfg.DatabaseMinConn, cfg.DatabaseMaxConn)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if cfg.Tracking.Enabled {
		trackingCipher, err := tracking.NewCipher(cfg.Tracking.EncryptionKey)
		if err != nil {
			logger.Error("initialize tracking cipher", "error", err)
			os.Exit(1)
		}
		providerCredentialCipher, err := providercredentials.NewCipher(
			cfg.ProviderCredentials.EncryptionKey,
		)
		if err != nil {
			logger.Error("initialize provider credential cipher", "error", err)
			os.Exit(1)
		}
		providerCredentialService := providercredentials.NewService(
			providercredentials.NewPostgresRepository(pool),
			providerCredentialCipher,
			nil,
		)
		fallbacks := make(map[string]providercredentials.StaticCredential)
		if cfg.RajaOngkir.APIKey != "" || cfg.RajaOngkir.DeliveryAPIKey != "" {
			fallbacks["rajaongkir"] = providercredentials.StaticCredential{
				Secret: cfg.RajaOngkir.APIKey,
				CapabilitySecrets: map[string]string{
					"rates:read":       cfg.RajaOngkir.APIKey,
					"tracking:read":    cfg.RajaOngkir.APIKey,
					"shipments:write":  cfg.RajaOngkir.DeliveryAPIKey,
					"shipments:read":   cfg.RajaOngkir.DeliveryAPIKey,
					"pickup:write":     cfg.RajaOngkir.DeliveryAPIKey,
					"labels:read":      cfg.RajaOngkir.DeliveryAPIKey,
					"shipments:cancel": cfg.RajaOngkir.DeliveryAPIKey,
				},
				CredentialAlias: cfg.RajaOngkir.CredentialAlias,
				DailyLimit:      cfg.RajaOngkir.DailyLimit,
			}
		}
		merchantProviderService := merchantproviders.NewService(
			merchantproviders.NewPostgresRepository(pool),
		)
		providerResolver := providercredentials.NewStaticFallbackResolver(
			providerCredentialService,
			fallbacks,
			merchantProviderService,
		)
		rajaOngkirHostedClient, err := hosted.NewClient(
			cfg.RajaOngkirHosted.BaseURL,
			cfg.RajaOngkirHosted.Timeout,
		)
		if err != nil {
			logger.Error("initialize RajaOngkir hosted connector", "error", err)
			os.Exit(1)
		}
		providerRepository := rates.NewPostgresRepository(pool)
		rajaOngkirTrackingAdapter := hosted.NewTrackingAdapter(
			"rajaongkir",
			rajaOngkirHostedClient,
			providerResolver,
			providerRepository,
			cfg.RajaOngkir.TrackingCouriers,
		)
		biteshipTrackingAdapter := biteship.NewDynamicTrackingAdapter(
			providerResolver,
			cfg.Biteship.BaseURL,
			cfg.Biteship.Timeout,
			providerRepository,
			providerRepository,
			biteship.EmisellTrackingFallbackCouriers(
				cfg.RajaOngkir.TrackingCouriers,
				cfg.Biteship.TrackingCouriers,
			),
		)
		fallbackTrackingAdapter := tracking.NewPolicyFallbackAdapter(
			merchantProviderService,
			rajaOngkirTrackingAdapter,
			biteshipTrackingAdapter,
		)
		adapters := []tracking.Adapter{fallbackTrackingAdapter}
		logger.Info(
			"hosted RajaOngkir tracking resolver enabled",
			"rajaongkir_couriers", len(cfg.RajaOngkir.TrackingCouriers),
			"biteship_fallback_couriers", len(biteshipTrackingAdapter.CourierCodes()),
		)
		trackingRepository := tracking.NewPostgresRepository(pool)
		migratedWaybills, err := trackingRepository.MigrateLegacyWaybills(
			ctx,
			trackingCipher,
		)
		if err != nil {
			logger.Error("migrate legacy tracking waybills", "error", err)
			os.Exit(1)
		}
		runner := tracking.NewRunner(
			trackingRepository,
			trackingCipher,
			adapters,
			cfg.Tracking.WorkerID,
			cfg.Tracking.Concurrency,
			cfg.Tracking.PollInterval,
			logger,
		)
		logger.Info(
			"durable tracking worker ready",
			"migrated_legacy_waybills", migratedWaybills,
		)
		group, workerCtx := errgroup.WithContext(ctx)
		group.Go(func() error { return runner.Run(workerCtx) })
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
		dispatcher := tracking.NewResolvingWebhookDispatcher(
			trackingRepository,
			webhookSettingsService,
			cfg.Tracking.WorkerID,
			cfg.Tracking.WebhookConcurrency,
			cfg.Tracking.WebhookPollInterval,
			cfg.Tracking.WebhookTimeout,
			logger,
		)
		group.Go(func() error { return dispatcher.Run(workerCtx) })
		fulfillmentRepository := fulfillment.NewPostgresRepository(
			pool, providerCredentialCipher,
		)
		fulfillmentService := fulfillment.NewService(
			fulfillmentRepository,
			merchantProviderService,
			providerResolver,
			hosted.NewFulfillmentAdapter("rajaongkir", rajaOngkirHostedClient),
		)
		fulfillmentRunner := fulfillment.NewRunner(
			fulfillmentRepository,
			fulfillmentService,
			fulfillment.NewTrackingServiceRegistrar(
				tracking.NewService(
					trackingRepository,
					trackingCipher,
					fallbackTrackingAdapter.CourierCodes()...,
				),
			),
			cfg.Tracking.WorkerID,
			2,
			cfg.Tracking.PollInterval,
			logger,
		)
		group.Go(func() error { return fulfillmentRunner.Run(workerCtx) })
		fulfillmentWebhookDispatcher := fulfillment.NewWebhookDispatcher(
			fulfillmentRepository,
			webhookSettingsService,
			cfg.Tracking.WorkerID,
			cfg.Tracking.WebhookConcurrency,
			cfg.Tracking.WebhookPollInterval,
			cfg.Tracking.WebhookTimeout,
			logger,
		)
		group.Go(func() error { return fulfillmentWebhookDispatcher.Run(workerCtx) })
		logger.Info("tracking webhook dispatcher ready; database settings override environment fallback")
		logger.Info("fulfillment lifecycle and webhook workers ready")
		if err := group.Wait(); err != nil {
			logger.Error("tracking worker stopped", "error", err)
			os.Exit(1)
		}
	} else {
		logger.Info("tracking worker disabled; set TRACKING_ENABLED after an adapter is configured")
		<-ctx.Done()
	}
	logger.Info("worker stopped")
}
