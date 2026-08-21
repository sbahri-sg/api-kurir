package merchantproviders

import (
	"context"
	"errors"
	"testing"
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

func TestActivateNormalizesProviderAndCredential(t *testing.T) {
	t.Parallel()
	repository := &memoryRepository{catalog: Catalog{ActiveProviderCode: "rajaongkir"}}
	service := NewService(repository)
	version := int64(2)
	credentialID := "11111111-2222-4333-8444-555555555555"

	result, err := service.Activate(context.Background(), "merchant_123", " RajaOngkir ", ChangeInput{
		CredentialID:    " " + credentialID + " ",
		ExpectedVersion: &version,
		UpdatedBy:       " tenant:merchant_123 ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ActiveProviderCode != "rajaongkir" ||
		repository.activatedProvider != "rajaongkir" ||
		repository.input.CredentialID != credentialID ||
		repository.input.UpdatedBy != "tenant:merchant_123" {
		t.Fatalf("unexpected activation: result=%#v input=%#v", result, repository.input)
	}
}

func TestDeactivateRejectsDefaultProvider(t *testing.T) {
	t.Parallel()
	service := NewService(&memoryRepository{})
	_, err := service.Deactivate(context.Background(), "merchant_123", "emisell", ChangeInput{})
	if !errors.Is(err, ErrDefaultProvider) {
		t.Fatalf("error=%v want ErrDefaultProvider", err)
	}
}

func TestChangeRejectsInvalidTenantProviderAndCredential(t *testing.T) {
	t.Parallel()
	service := NewService(&memoryRepository{})
	tests := []struct {
		name       string
		tenant     string
		provider   string
		credential string
		want       error
	}{
		{name: "tenant", tenant: "merchant with spaces", provider: "emisell", want: ErrInvalidTenant},
		{name: "provider", tenant: "merchant_123", provider: "Raja Ongkir", want: ErrInvalidProvider},
		{name: "credential", tenant: "merchant_123", provider: "rajaongkir", credential: "not-a-uuid", want: ErrInvalidCredential},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.Activate(context.Background(), test.tenant, test.provider, ChangeInput{
				CredentialID: test.credential,
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("error=%v want=%v", err, test.want)
			}
		})
	}
}
