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
