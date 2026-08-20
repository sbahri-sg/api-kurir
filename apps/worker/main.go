package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/emisell/api-kurir/internal/config"
	"github.com/emisell/api-kurir/internal/database"
	"github.com/emisell/api-kurir/internal/providercredentials"
	"github.com/emisell/api-kurir/internal/providers/biteship"
	"github.com/emisell/api-kurir/internal/providers/rajaongkir"
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
		providerRepository := rates.NewPostgresRepository(pool)
		rajaOngkirTrackingAdapter := rajaongkir.NewDynamicTrackingAdapter(
			providerResolver,
			cfg.RajaOngkir.BaseURL,
			cfg.RajaOngkir.Timeout,
			cfg.RajaOngkir.MinRequestInterval,
			providerRepository,
			providerRepository,
			cfg.RajaOngkir.TrackingCouriers,
		)
		biteshipTrackingAdapter := biteship.NewDynamicTrackingAdapter(
			providerResolver,
			cfg.Biteship.BaseURL,
			cfg.Biteship.Timeout,
			providerRepository,
			providerRepository,
			cfg.Biteship.TrackingCouriers,
		)
		fallbackTrackingAdapter := tracking.NewFallbackAdapter(
			rajaOngkirTrackingAdapter,
			biteshipTrackingAdapter,
		)
		adapters := []tracking.Adapter{fallbackTrackingAdapter}
		logger.Info(
			"dynamic tracking credential resolver enabled",
			"rajaongkir_couriers", len(cfg.RajaOngkir.TrackingCouriers),
			"biteship_fallback_couriers", len(cfg.Biteship.TrackingCouriers),
		)
		trackingRepository := tracking.NewPostgresRepository(pool)
		runner := tracking.NewRunner(
			trackingRepository,
			trackingCipher,
			adapters,
			cfg.Tracking.WorkerID,
			cfg.Tracking.Concurrency,
			cfg.Tracking.PollInterval,
			logger,
		)
		logger.Info("durable tracking worker ready")
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
		logger.Info("tracking webhook dispatcher ready; database settings override environment fallback")
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
