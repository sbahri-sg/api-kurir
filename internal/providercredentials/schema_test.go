package providercredentials

import "testing"

func TestOAuthCredentialFields(t *testing.T) {
	t.Parallel()

	fields := FieldsForCredentialType(CredentialTypeOAuth2ClientCredentials)
	if len(fields) != 2 || fields[0].Code != "client_id" || fields[0].Secret ||
		fields[1].Code != "client_secret" || !fields[1].Secret {
		t.Fatalf("unexpected OAuth fields: %#v", fields)
	}
}

func TestNormalizeCredentialValuesRejectsUnexpectedField(t *testing.T) {
	t.Parallel()

	_, err := NormalizeCredentialValues(CredentialTypeOAuth2ClientCredentials, map[string]string{
		"client_id": "merchant-client", "client_secret": "oauth-secret",
		"password": "not-allowed",
	})
	if err == nil {
		t.Fatal("expected unsupported field error")
	}
}

func TestCapabilityAPIKeyFieldsAndLegacyAlias(t *testing.T) {
	t.Parallel()

	fields := FieldsForCredentialType(CredentialTypeCapabilityAPIKeys)
	if len(fields) != 2 || fields[0].Code != "shipping_api_key" ||
		!fields[0].Required || fields[1].Code != "delivery_api_key" ||
		fields[1].Required {
		t.Fatalf("unexpected capability fields: %#v", fields)
	}
	if len(fields[0].Capabilities) != 2 || fields[0].Capabilities[0] != "rates:read" {
		t.Fatalf("unexpected shipping capabilities: %#v", fields[0].Capabilities)
	}

	values, err := NormalizeCredentialValues(
		CredentialTypeCapabilityAPIKeys,
		map[string]string{"api_key": "legacy-shipping-key"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if values["shipping_api_key"] != "legacy-shipping-key" || values["delivery_api_key"] != "" {
		t.Fatalf("legacy alias was not normalized: %#v", values)
	}
}

func TestCapabilityAPIKeysRejectDuplicateShippingAlias(t *testing.T) {
	t.Parallel()

	_, err := NormalizeCredentialValues(
		CredentialTypeCapabilityAPIKeys,
		map[string]string{
			"api_key": "legacy-shipping-key", "shipping_api_key": "new-shipping-key",
		},
	)
	if err == nil {
		t.Fatal("expected duplicate shipping key alias error")
	}
}
