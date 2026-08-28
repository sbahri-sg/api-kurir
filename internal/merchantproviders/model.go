package merchantproviders

import "github.com/emisell/api-kurir/internal/providercredentials"

const EmisellProviderCode = "emisell"

const (
	AutoPickupStatusConfigured                 = "configured"
	AutoPickupStatusCredentialMissing          = "credential_missing"
	AutoPickupStatusCredentialInvalidOrExpired = "credential_invalid_or_expired"
	AutoPickupStatusUnsupported                = "unsupported"
)

type Provider struct {
	Code                 string                                            `json:"code"`
	Name                 string                                            `json:"name"`
	Logo                 string                                            `json:"logo"`
	Description          string                                            `json:"description"`
	BuiltIn              bool                                              `json:"built_in"`
	IntegrationType      string                                            `json:"integration_type"`
	DistributionType     string                                            `json:"distribution_type"`
	RequiresCredential   bool                                              `json:"requires_credential"`
	CredentialType       string                                            `json:"credential_type"`
	CredentialSource     string                                            `json:"credential_source"`
	CredentialFields     []providercredentials.FieldDefinition             `json:"credential_fields"`
	Environments         []providercredentials.EnvironmentDefinition       `json:"environments"`
	CapabilityPolicies   []providercredentials.CapabilityEnvironmentPolicy `json:"capability_policies"`
	Available            bool                                              `json:"available"`
	Installed            bool                                              `json:"installed"`
	Active               bool                                              `json:"active"`
	ActiveReleaseVersion string                                            `json:"active_release_version,omitempty"`
	RequiredScopes       []string                                          `json:"required_scopes"`
	GrantedScopes        []string                                          `json:"granted_scopes"`
}

type Catalog struct {
	ActiveProviderCode *string    `json:"active_provider_code"`
	Version            int64      `json:"version"`
	Providers          []Provider `json:"providers"`
}

type Detail struct {
	Provider
	AutoPickup             bool                                   `json:"auto_pickup"`
	AutoPickupStatus       string                                 `json:"auto_pickup_status"`
	AutoPickupEnvironments map[string]AutoPickupEnvironmentStatus `json:"auto_pickup_environments"`
	AvailableCredentials   map[string][]string                    `json:"available_credentials"`
}

type AutoPickupEnvironmentStatus struct {
	Enabled            bool     `json:"enabled"`
	Status             string   `json:"status"`
	MissingCredentials []string `json:"missing_credentials"`
}

type ChangeInput struct {
	ExpectedVersion *int64
	UpdatedBy       string
}
