package httpapi

import (
	"encoding/json"
	"testing"

	"github.com/emisell/api-kurir/internal/merchantproviders"
	"github.com/emisell/api-kurir/internal/providercredentials"
)

func TestTenantShippingProviderCatalogOmitsInstallationDetails(t *testing.T) {
	t.Parallel()
	response := tenantShippingProviderCatalog(merchantproviders.Catalog{
		Version: 3,
		Providers: []merchantproviders.Provider{{
			Code: "rajaongkir", Name: "RajaOngkir", Available: true,
			CredentialFields: []providercredentials.FieldDefinition{{
				Code: "shipping_api_key", Secret: true,
			}},
			RequiredScopes: []string{"rates:read", "pickup:write"},
		}},
	})
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	providers, ok := payload["providers"].([]any)
	if !ok || len(providers) != 1 {
		t.Fatalf("providers=%#v", payload["providers"])
	}
	provider, ok := providers[0].(map[string]any)
	if !ok {
		t.Fatalf("provider=%#v", providers[0])
	}
	for _, hidden := range []string{
		"credential_fields", "environments", "capability_policies",
		"required_scopes", "granted_scopes", "available_credentials",
	} {
		if _, exists := provider[hidden]; exists {
			t.Fatalf("listing leaked detail field %q: %s", hidden, encoded)
		}
	}
}
