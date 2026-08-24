package merchantproviders

import (
	"context"
	"errors"
	"testing"

	"github.com/emisell/api-kurir/internal/tenancy"
)

type memoryRepository struct {
	catalog             Catalog
	activatedProvider   string
	deactivatedProvider string
	input               ChangeInput
	err                 error
}

func (repository *memoryRepository) Catalog(context.Context, string) (Catalog, error) {
	return repository.catalog, repository.err
}

func (repository *memoryRepository) Activate(
	_ context.Context,
	_ string,
	providerCode string,
	input ChangeInput,
) (Catalog, error) {
	repository.activatedProvider = providerCode
	repository.input = input
	return repository.catalog, repository.err
}

func (repository *memoryRepository) Deactivate(
	_ context.Context,
	_ string,
	providerCode string,
	input ChangeInput,
) (Catalog, error) {
	repository.deactivatedProvider = providerCode
	repository.input = input
	return repository.catalog, repository.err
}

func TestActivateNormalizesProvider(t *testing.T) {
	t.Parallel()
	repository := &memoryRepository{catalog: Catalog{ActiveProviderCode: stringPointer("rajaongkir")}}
	service := NewService(repository)
	version := int64(2)

	result, err := service.Activate(context.Background(), "merchant_123", " RajaOngkir ", ChangeInput{
		ExpectedVersion: &version,
		UpdatedBy:       " tenant:merchant_123 ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ActiveProviderCode == nil || *result.ActiveProviderCode != "rajaongkir" ||
		repository.activatedProvider != "rajaongkir" ||
		repository.input.UpdatedBy != "tenant:merchant_123" {
		t.Fatalf("unexpected activation: result=%#v input=%#v", result, repository.input)
	}
}

func TestDeactivateAllowsEmisellProvider(t *testing.T) {
	t.Parallel()
	repository := &memoryRepository{}
	service := NewService(repository)
	if _, err := service.Deactivate(context.Background(), "merchant_123", "emisell", ChangeInput{}); err != nil {
		t.Fatal(err)
	}
	if repository.deactivatedProvider != "emisell" {
		t.Fatalf("deactivated provider=%q want emisell", repository.deactivatedProvider)
	}
}

func TestHasActiveProvider(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		active  *string
		enabled bool
	}{
		{name: "inactive", active: nil, enabled: false},
		{name: "active", active: stringPointer("emisell"), enabled: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := NewService(&memoryRepository{catalog: Catalog{ActiveProviderCode: test.active}})
			enabled, err := service.HasActiveProvider(context.Background(), "merchant_123")
			if err != nil {
				t.Fatal(err)
			}
			if enabled != test.enabled {
				t.Fatalf("enabled=%v want=%v", enabled, test.enabled)
			}
		})
	}
}

func TestAllowsPlatformCredentialOnlyForEmisell(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		active  *string
		allowed bool
	}{
		{name: "inactive", active: nil, allowed: false},
		{name: "built in", active: stringPointer("emisell"), allowed: true},
		{name: "byok", active: stringPointer("rajaongkir"), allowed: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := NewService(&memoryRepository{catalog: Catalog{ActiveProviderCode: test.active}})
			allowed, err := service.AllowsPlatformCredential(context.Background(), "merchant_123")
			if err != nil {
				t.Fatal(err)
			}
			if allowed != test.allowed {
				t.Fatalf("allowed=%v want=%v", allowed, test.allowed)
			}
		})
	}
}

func TestAllowsProviderFallbackUsesTenantActiveProvider(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		active  *string
		allowed bool
	}{
		{name: "built in", active: stringPointer("emisell"), allowed: true},
		{name: "byok", active: stringPointer("rajaongkir"), allowed: false},
		{name: "inactive", active: nil, allowed: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := NewService(&memoryRepository{catalog: Catalog{ActiveProviderCode: test.active}})
			ctx := tenancy.WithIdentity(
				context.Background(),
				tenancy.Identity{TenantID: "merchant_123"},
			)
			allowed, err := service.AllowsProviderFallback(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if allowed != test.allowed {
				t.Fatalf("allowed=%v want=%v", allowed, test.allowed)
			}
		})
	}
}

func TestAllowsProviderFallbackAllowsPlatformOperation(t *testing.T) {
	t.Parallel()
	service := NewService(&memoryRepository{})
	allowed, err := service.AllowsProviderFallback(context.Background())
	if err != nil || !allowed {
		t.Fatalf("allowed=%v err=%v", allowed, err)
	}
}

func stringPointer(value string) *string { return &value }

func TestChangeRejectsInvalidTenantAndProvider(t *testing.T) {
	t.Parallel()
	service := NewService(&memoryRepository{})
	tests := []struct {
		name     string
		tenant   string
		provider string
		want     error
	}{
		{name: "tenant", tenant: "merchant with spaces", provider: "emisell", want: ErrInvalidTenant},
		{name: "provider", tenant: "merchant_123", provider: "Raja Ongkir", want: ErrInvalidProvider},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.Activate(context.Background(), test.tenant, test.provider, ChangeInput{})
			if !errors.Is(err, test.want) {
				t.Fatalf("error=%v want=%v", err, test.want)
			}
		})
	}
}
