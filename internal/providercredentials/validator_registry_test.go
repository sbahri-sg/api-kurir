package providercredentials

import (
	"context"
	"errors"
	"testing"
)

type registryValidatorStub struct {
	calls int
}

func (v *registryValidatorStub) Validate(context.Context, string, string) error {
	v.calls++
	return nil
}

type registryBundleValidatorStub struct {
	registryValidatorStub
	credentialType string
	values         map[string]string
}

func (v *registryBundleValidatorStub) ValidateCredentials(
	_ context.Context,
	_ string,
	credentialType string,
	values map[string]string,
) error {
	v.calls++
	v.credentialType = credentialType
	v.values = values
	return nil
}

func TestValidatorRegistryRoutesByProviderCode(t *testing.T) {
	t.Parallel()
	raja := &registryValidatorStub{}
	biteship := &registryValidatorStub{}
	registry := NewValidatorRegistry(map[string]Validator{
		"rajaongkir": raja,
		"biteship":   biteship,
	})
	if err := registry.Validate(context.Background(), "biteship", "token"); err != nil {
		t.Fatal(err)
	}
	if raja.calls != 0 || biteship.calls != 1 {
		t.Fatalf("unexpected calls: raja=%d biteship=%d", raja.calls, biteship.calls)
	}
	if err := registry.Validate(context.Background(), "unknown", "token"); !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("expected unsupported provider, got %v", err)
	}
}

func TestValidatorRegistryRoutesCredentialBundles(t *testing.T) {
	t.Parallel()
	oauth := &registryBundleValidatorStub{}
	registry := NewValidatorRegistry(map[string]Validator{"oauth-provider": oauth})
	values := map[string]string{
		"client_id":     "client-production",
		"client_secret": "secret-production",
	}
	if err := registry.ValidateCredentials(
		context.Background(),
		"oauth-provider",
		CredentialTypeOAuth2ClientCredentials,
		values,
	); err != nil {
		t.Fatal(err)
	}
	if oauth.calls != 1 || oauth.credentialType != CredentialTypeOAuth2ClientCredentials {
		t.Fatalf("unexpected bundle validation: %#v", oauth)
	}
	if oauth.values["client_id"] != values["client_id"] {
		t.Fatalf("unexpected OAuth values: %#v", oauth.values)
	}
}

func TestValidatorRegistryFallsBackForSingleSecret(t *testing.T) {
	t.Parallel()
	apiKey := &registryValidatorStub{}
	registry := NewValidatorRegistry(map[string]Validator{"api-provider": apiKey})
	if err := registry.ValidateCredentials(
		context.Background(),
		"api-provider",
		CredentialTypeAPIKey,
		map[string]string{"api_key": "api-key-production"},
	); err != nil {
		t.Fatal(err)
	}
	if apiKey.calls != 1 {
		t.Fatalf("expected one API key validation, got %d", apiKey.calls)
	}
}

func TestValidatorRegistryValidatesCapabilityShippingKey(t *testing.T) {
	t.Parallel()
	shipping := &registryValidatorStub{}
	registry := NewValidatorRegistry(map[string]Validator{"rajaongkir": shipping})
	if err := registry.ValidateCredentials(
		context.Background(),
		"rajaongkir",
		CredentialTypeCapabilityAPIKeys,
		map[string]string{
			"shipping_api_key": "shipping-production-key",
			"delivery_api_key": "delivery-production-key",
		},
	); err != nil {
		t.Fatal(err)
	}
	if shipping.calls != 1 {
		t.Fatalf("expected one shipping key validation, got %d", shipping.calls)
	}
}

func TestValidatorRegistryDefersDeliveryOnlyCredentialValidation(t *testing.T) {
	t.Parallel()
	delivery := &registryValidatorStub{}
	registry := NewValidatorRegistry(map[string]Validator{"rajaongkir": delivery})
	if err := registry.ValidateCredentials(
		context.Background(),
		"rajaongkir",
		CredentialTypeCapabilityAPIKeys,
		map[string]string{"delivery_api_key": "sandbox-delivery-key"},
	); err != nil {
		t.Fatal(err)
	}
	if delivery.calls != 0 {
		t.Fatalf("delivery-only credential used Shipping Cost validator %d times", delivery.calls)
	}
}
