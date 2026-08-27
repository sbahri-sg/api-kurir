package providercredentials

import "time"

const DefaultDailyLimit int64 = 50_000

type Credential struct {
	ID               string     `json:"id"`
	TenantID         string     `json:"tenant_id,omitempty"`
	ProviderCode     string     `json:"provider_code"`
	Environment      string     `json:"environment"`
	CredentialAlias  string     `json:"credential_alias"`
	DisplayKey       string     `json:"display_key"`
	DailyLimit       int64      `json:"daily_limit"`
	Active           bool       `json:"active"`
	ValidationStatus string     `json:"validation_status"`
	LastValidatedAt  time.Time  `json:"last_validated_at"`
	LastSelectedAt   *time.Time `json:"last_selected_at"`
	CreatedAt        time.Time  `json:"created_at"`
	DisabledAt       *time.Time `json:"disabled_at"`
}

type StoredCredential struct {
	Credential
	SecretCiphertext []byte
	Fingerprint      []byte
}

type CreateInput struct {
	ID                  string
	TenantID            string
	ProviderCode        string
	Environment         string
	CredentialAlias     string
	KeyPrefix           string
	KeyLastFour         string
	SecretCiphertext    []byte
	SecretFingerprint   []byte
	DailyLimit          int64
	ValidationQuotaCost int64
	CreatedBy           string
	RequestID           string
}
