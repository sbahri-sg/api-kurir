package config

import (
	"testing"
	"time"
)

func TestLoadEnablesBiteshipRateFallbackByDefault(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Biteship.RateFallbackEnabled ||
		cfg.Biteship.RateSnapshotTTL != 14*24*time.Hour {
		t.Fatalf("unexpected Biteship fallback config: %#v", cfg.Biteship)
	}
}

func TestLoadAllowsTrackingProviderOverlapForOrderedFallback(t *testing.T) {
	t.Setenv("RAJAONGKIR_TRACKING_COURIERS", "jne,jnt")
	t.Setenv("BITESHIP_TRACKING_COURIERS", "jne,sicepat")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Biteship.TrackingCouriers) != 2 {
		t.Fatalf("unexpected Biteship couriers: %v", cfg.Biteship.TrackingCouriers)
	}
}

func TestLoadSeparatesRajaOngkirShippingAndDeliveryCredentials(t *testing.T) {
	t.Setenv("RAJAONGKIR_API_KEY", "shipping-key")
	t.Setenv("RAJAONGKIR_DELIVERY_API_KEY", "delivery-key")
	t.Setenv("RAJAONGKIR_DELIVERY_BASE_URL", "https://api-sandbox.collaborator.komerce.id")
	t.Setenv("RAJAONGKIR_DELIVERY_TIMEOUT", "9s")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RajaOngkir.APIKey != "shipping-key" ||
		cfg.RajaOngkir.DeliveryAPIKey != "delivery-key" ||
		cfg.RajaOngkir.DeliveryBaseURL != "https://api-sandbox.collaborator.komerce.id" ||
		cfg.RajaOngkir.DeliveryTimeout != 9*time.Second {
		t.Fatalf("unexpected RajaOngkir delivery config: %#v", cfg.RajaOngkir)
	}
}

func TestLoadConfiguresHostedConnectorPublicURL(t *testing.T) {
	t.Setenv("RAJAONGKIR_HOSTED_PUBLIC_BASE_URL", "http://127.0.0.1:5174/connectors/rajaongkir/v1")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RajaOngkirHosted.PublicBaseURL != "http://127.0.0.1:5174/connectors/rajaongkir/v1" {
		t.Fatalf("unexpected hosted public URL: %#v", cfg.RajaOngkirHosted)
	}
}

func TestLoadRequiresHTTPSHostedPublicURLInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("API_KEYS", "public")
	t.Setenv("ADMIN_API_KEYS", "admin")
	t.Setenv("PROVIDER_CREDENTIAL_ENCRYPTION_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	t.Setenv("RAJAONGKIR_HOSTED_PUBLIC_BASE_URL", "http://api-kurir.emisell.com/connectors/rajaongkir/v1")
	if _, err := Load(); err == nil {
		t.Fatal("expected production hosted public URL validation error")
	}
}

func TestLoadMerchantShippingLimits(t *testing.T) {
	t.Setenv("MERCHANT_SHIPPING_MAX_COURIERS", "7")
	t.Setenv("MERCHANT_SHIPPING_MAX_SERVICES", "30")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MerchantShipping.MaxSelectedCouriers != 7 ||
		cfg.MerchantShipping.MaxSelectedServices != 30 {
		t.Fatalf("unexpected merchant shipping limits: %#v", cfg.MerchantShipping)
	}
}

func TestLoadRejectsInvalidMerchantShippingLimits(t *testing.T) {
	t.Setenv("MERCHANT_SHIPPING_MAX_COURIERS", "0")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid courier limit error")
	}
}
