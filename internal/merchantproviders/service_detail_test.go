package merchantproviders

import (
	"context"
	"testing"

	"github.com/emisell/api-kurir/internal/providercredentials"
)

type detailRepository struct {
	catalog Catalog
}

func (r detailRepository) Catalog(context.Context, string) (Catalog, error) {
	return r.catalog, nil
}

func (r detailRepository) Activate(
	context.Context, string, string, ChangeInput,
) (Catalog, error) {
	return r.catalog, nil
}

func (r detailRepository) Deactivate(
	context.Context, string, string, ChangeInput,
) (Catalog, error) {
	return r.catalog, nil
}

type detailAvailability struct {
	availability providercredentials.Availability
}

func (r detailAvailability) AvailableCredentialFieldsForTenant(
	context.Context, string, string,
) (providercredentials.Availability, error) {
	return r.availability, nil
}

func TestProviderDetailReportsConfiguredAutoPickup(t *testing.T) {
	t.Parallel()
	service := NewService(
		detailRepository{catalog: Catalog{Providers: []Provider{{
			Code: "rajaongkir", Name: "RajaOngkir", RequiresCredential: true,
			CredentialFields: []providercredentials.FieldDefinition{
				{
					Code: "shipping_api_key", Capabilities: []string{"rates:read"},
				},
				{
					Code: "delivery_api_key", Capabilities: []string{"pickup:write"},
				},
			},
			Environments: []providercredentials.EnvironmentDefinition{
				{Code: providercredentials.EnvironmentLive, Label: "Live"},
				{Code: providercredentials.EnvironmentSandbox, Label: "Sandbox"},
			},
		}}}},
		detailAvailability{availability: providercredentials.Availability{
			FieldsByEnvironment: map[string][]string{
				providercredentials.EnvironmentLive: {
					"delivery_api_key", "shipping_api_key",
				},
			},
			StatusByEnvironment: map[string]string{
				providercredentials.EnvironmentLive: "valid",
			},
		}},
	)
	detail, err := service.Provider(context.Background(), "merchant_123", "rajaongkir")
	if err != nil {
		t.Fatal(err)
	}
	if !detail.AutoPickup || detail.AutoPickupStatus != "configured" {
		t.Fatalf("unexpected auto-pickup detail: %#v", detail)
	}
	if got := detail.AvailableCredentials[providercredentials.EnvironmentSandbox]; len(got) != 0 {
		t.Fatalf("sandbox credentials=%#v want empty", got)
	}
}
