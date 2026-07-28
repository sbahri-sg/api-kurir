package apikeys

import "time"

type APIKey struct {
	ID          string     `json:"id"`
	KeyPrefix   string     `json:"key_prefix"`
	KeyLastFour string     `json:"key_last_four"`
	DisplayKey  string     `json:"display_key"`
	Scopes      []string   `json:"scopes"`
	Active      bool       `json:"active"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	RevokedBy   *string    `json:"revoked_by"`
	RevokedAt   *time.Time `json:"revoked_at"`
}

type GeneratedAPIKey struct {
	APIKey APIKey `json:"api_key"`
	Secret string `json:"secret"`
}

type CreateInput struct {
	KeyPrefix string
	KeyLast4  string
	KeyHash   []byte
	Scopes    []string
	CreatedBy string
	RequestID string
}
