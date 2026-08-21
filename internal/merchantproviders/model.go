package merchantproviders

const EmisellProviderCode = "emisell"

type Provider struct {
	Code               string `json:"code"`
	Name               string `json:"name"`
	BuiltIn            bool   `json:"built_in"`
	RequiresCredential bool   `json:"requires_credential"`
	Available          bool   `json:"available"`
	Installed          bool   `json:"installed"`
	Active             bool   `json:"active"`
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
