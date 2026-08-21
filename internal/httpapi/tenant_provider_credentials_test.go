package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/providercredentials"
)

func TestTenantProviderCredentialResponseHidesInternalIDs(t *testing.T) {
	t.Parallel()
	response := tenantProviderCredential(providercredentials.Credential{
		ID:               "11111111-2222-4333-8444-555555555555",
		TenantID:         "merchant_123",
		ProviderCode:     "rajaongkir",
		CredentialAlias:  "rajaongkir-demo",
		DisplayKey:       "demo••••1234",
		DailyLimit:       50_000,
		Active:           true,
		ValidationStatus: "valid",
		LastValidatedAt:  time.Now().UTC(),
		CreatedAt:        time.Now().UTC(),
	})
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(payload)
	if strings.Contains(encoded, `"id"`) || strings.Contains(encoded, "tenant_id") ||
		strings.Contains(encoded, "11111111-2222-4333-8444-555555555555") {
		t.Fatalf("tenant response leaked internal ownership fields: %s", encoded)
	}
}
