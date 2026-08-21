package config

import (
	"reflect"
	"testing"
)

func TestOverlappingValuesNormalizesAndDeduplicates(t *testing.T) {
	got := overlappingValues(
		[]string{"jne", "JNT", " tiki "},
		[]string{"sicepat", "JNE", "jne", "jnt"},
	)
	want := []string{"jne", "jnt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("overlap = %v, want %v", got, want)
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
