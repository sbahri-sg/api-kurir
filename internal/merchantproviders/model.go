package merchantproviders

const DefaultProviderCode = "emisell"

type Provider struct {
	Code               string  `json:"code"`
	Name               string  `json:"name"`
	BuiltIn            bool    `json:"built_in"`
	RequiresCredential bool    `json:"requires_credential"`
	Available          bool    `json:"available"`
	Installed          bool    `json:"installed"`
	Active             bool    `json:"active"`
	CredentialID       *string `json:"credential_id,omitempty"`
}

type Catalog struct {
	ActiveProviderCode string     `json:"active_provider_code"`
	ActiveCredentialID *string    `json:"active_credential_id,omitempty"`
	Version            int64      `json:"version"`
	Providers          []Provider `json:"providers"`
}

type ChangeInput struct {
	CredentialID    string
	ExpectedVersion *int64
	UpdatedBy       string
}
