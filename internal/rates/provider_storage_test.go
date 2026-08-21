package rates

import "testing"

func TestProviderRequestFingerprintIsStableForCourierOrder(t *testing.T) {
	t.Parallel()

	first := Request{
		Origin:            "origin",
		Destination:       "destination",
		ActualWeightGrams: 1200,
		Couriers:          []string{"tiki", "jne"},
	}
	second := first
	second.Couriers = []string{"jne", "tiki"}

	if providerRequestFingerprint(first) != providerRequestFingerprint(second) {
		t.Fatal("courier order changed the request fingerprint")
	}
}

func TestProviderRequestFingerprintIncludesDimensions(t *testing.T) {
	t.Parallel()

	withoutDimensions := Request{
		Origin:            "origin",
		Destination:       "destination",
		ActualWeightGrams: 1200,
		Couriers:          []string{"jne"},
	}
	withDimensions := withoutDimensions
	withDimensions.Dimensions = &Dimensions{LengthCM: 10, WidthCM: 10, HeightCM: 10}

	if providerRequestFingerprint(withoutDimensions) == providerRequestFingerprint(withDimensions) {
		t.Fatal("dimensions did not change the request fingerprint")
	}
}

func TestProviderRequestFingerprintIsolatedByTenantAndIntegration(t *testing.T) {
	t.Parallel()

	base := Request{
		Origin:            "origin",
		Destination:       "destination",
		ActualWeightGrams: 1200,
		Couriers:          []string{"jne"},
	}
	tenantA := base
	tenantA.TenantID = "merchant_a"
	tenantA.ProviderCredentialID = "credential_a"
	tenantB := tenantA
	tenantB.TenantID = "merchant_b"
	secondIntegration := tenantA
	secondIntegration.ProviderCredentialID = "credential_b"

	baseFingerprint := providerRequestFingerprint(base)
	if baseFingerprint == providerRequestFingerprint(tenantA) ||
		providerRequestFingerprint(tenantA) == providerRequestFingerprint(tenantB) ||
		providerRequestFingerprint(tenantA) == providerRequestFingerprint(secondIntegration) {
		t.Fatal("provider quote fingerprint was shared across tenant or credential")
	}
}
