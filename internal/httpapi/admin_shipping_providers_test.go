package httpapi

import "testing"

func TestNormalizeShippingProviderCreateInput(t *testing.T) {
	t.Parallel()

	input, err := normalizeShippingProviderCreateInput(createShippingProviderRequest{
		Code:         " Partner-Express ",
		Name:         " Partner Express ",
		Logo:         "https://cdn.example.com/partner.svg",
		Description:  "Provider partner untuk pengiriman merchant Emisell.",
		DisplayOrder: 40,
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.Code != "partner-express" || input.Name != "Partner Express" || input.DisplayOrder != 40 {
		t.Fatalf("unexpected normalized provider: %#v", input)
	}
	if input.CredentialType != "none" {
		t.Fatalf("partner-hosted credential type=%q want none", input.CredentialType)
	}
	if input.DistributionType != "merchant" {
		t.Fatalf("distribution type=%q want merchant", input.DistributionType)
	}
}

func TestNormalizeShippingProviderOAuthCredential(t *testing.T) {
	t.Parallel()

	input, err := normalizeShippingProviderCreateInput(createShippingProviderRequest{
		Code: "oauth-courier", Name: "OAuth Courier",
		Logo:            "https://cdn.example.com/oauth.svg",
		Description:     "Provider OAuth untuk pengiriman merchant Emisell.",
		IntegrationType: "managed_upstream",
		CredentialType:  "oauth2_client_credentials", DisplayOrder: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.CredentialType != "oauth2_client_credentials" {
		t.Fatalf("credential type=%q", input.CredentialType)
	}
}

func TestNormalizePartnerHostedCapabilityCredential(t *testing.T) {
	t.Parallel()

	input, err := normalizeShippingProviderCreateInput(createShippingProviderRequest{
		Code: "hosted-byok", Name: "Hosted BYOK",
		Logo:            "https://cdn.example.com/hosted.svg",
		Description:     "Connector hosted dengan credential milik seller.",
		IntegrationType: "partner_hosted",
		CredentialType:  "capability_api_keys", DisplayOrder: 55,
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.CredentialType != "capability_api_keys" {
		t.Fatalf("credential type=%q", input.CredentialType)
	}
}

func TestNormalizeShippingProviderRejectsUnsafeMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request createShippingProviderRequest
	}{
		{
			name: "non https logo",
			request: createShippingProviderRequest{
				Code: "partner", Name: "Partner", Logo: "http://example.com/logo.svg",
				Description: "Deskripsi provider yang valid.", DisplayOrder: 10,
			},
		},
		{
			name: "html description",
			request: createShippingProviderRequest{
				Code: "partner", Name: "Partner", Logo: "https://example.com/logo.svg",
				Description: "<strong>Provider pengiriman</strong>", DisplayOrder: 10,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := normalizeShippingProviderCreateInput(test.request); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
