package providercredentials

import "time"

const DefaultDailyLimit int64 = 50_000

type Credential struct {
	ID               string     `json:"id"`
	ProviderCode     string     `json:"provider_code"`
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
	ID                string
	ProviderCode      string
	CredentialAlias   string
	KeyPrefix         string
	KeyLastFour       string
	SecretCiphertext  []byte
	SecretFingerprint []byte
	DailyLimit        int64
	CreatedBy         string
	RequestID         string
}
