package merchantproviders

import "github.com/emisell/api-kurir/internal/providercredentials"

const EmisellProviderCode = "emisell"

type Provider struct {
	Code                 string                                `json:"code"`
	Name                 string                                `json:"name"`
	Logo                 string                                `json:"logo"`
	Description          string                                `json:"description"`
	BuiltIn              bool                                  `json:"built_in"`
	IntegrationType      string                                `json:"integration_type"`
	DistributionType     string                                `json:"distribution_type"`
	RequiresCredential   bool                                  `json:"requires_credential"`
	CredentialType       string                                `json:"credential_type"`
	CredentialFields     []providercredentials.FieldDefinition `json:"credential_fields"`
	Available            bool                                  `json:"available"`
	Installed            bool                                  `json:"installed"`
	Active               bool                                  `json:"active"`
	ActiveReleaseVersion string                                `json:"active_release_version,omitempty"`
	RequiredScopes       []string                              `json:"required_scopes"`
	GrantedScopes        []string                              `json:"granted_scopes"`
}

type Catalog struct {
	ActiveProviderCode *string    `json:"active_provider_code"`
	Version            int64      `json:"version"`
	Providers          []Provider `json:"providers"`
}

type ChangeInput struct {
	ExpectedVersion *int64
	UpdatedBy       string
}
