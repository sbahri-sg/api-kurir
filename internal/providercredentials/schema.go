package providercredentials

import (
	"errors"
	"strings"
)

const (
	CredentialTypeNone                    = "none"
	CredentialTypeAPIKey                  = "api_key"
	CredentialTypeCapabilityAPIKeys       = "capability_api_keys"
	CredentialTypeBearerToken             = "bearer_token"
	CredentialTypeAPIKeySecret            = "api_key_secret"
	CredentialTypeOAuth2ClientCredentials = "oauth2_client_credentials"
)

type FieldDefinition struct {
	Code         string   `json:"code"`
	Label        string   `json:"label"`
	InputType    string   `json:"input_type"`
	Secret       bool     `json:"secret"`
	Required     bool     `json:"required"`
	Placeholder  string   `json:"placeholder"`
	Help         string   `json:"help"`
	Capabilities []string `json:"capabilities"`
}

func NormalizeCredentialType(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case CredentialTypeNone,
		CredentialTypeAPIKey,
		CredentialTypeCapabilityAPIKeys,
		CredentialTypeBearerToken,
		CredentialTypeAPIKeySecret,
		CredentialTypeOAuth2ClientCredentials:
		return value, true
	default:
		return "", false
	}
}

func FieldsForCredentialType(value string) []FieldDefinition {
	switch value {
	case CredentialTypeAPIKey:
		return []FieldDefinition{{
			Code: "api_key", Label: "API key", InputType: "password",
			Secret: true, Required: true, Placeholder: "Masukkan API key provider",
			Help:         "API key diterbitkan oleh provider dan disimpan terenkripsi.",
			Capabilities: []string{},
		}}
	case CredentialTypeCapabilityAPIKeys:
		return []FieldDefinition{
			{
				Code: "shipping_api_key", Label: "Shipping API key", InputType: "password",
				Secret: true, Required: true, Placeholder: "Masukkan API key tarif dan tracking",
				Help:         "Dipakai untuk cek ongkir dan tracking.",
				Capabilities: []string{"rates:read", "tracking:read"},
			},
			{
				Code: "delivery_api_key", Label: "Delivery API key", InputType: "password",
				Secret: true, Required: false, Placeholder: "Masukkan API key delivery (opsional)",
				Help:         "Dipakai untuk membuat pengiriman, pickup, pembatalan, dan label.",
				Capabilities: []string{"shipments:write", "shipments:read", "pickup:write", "labels:read", "shipments:cancel"},
			},
		}
	case CredentialTypeBearerToken:
		return []FieldDefinition{{
			Code: "token", Label: "Access token", InputType: "password",
			Secret: true, Required: true, Placeholder: "Masukkan access token",
			Help:         "Token akan dikirim sebagai Bearer credential oleh adapter provider.",
			Capabilities: []string{},
		}}
	case CredentialTypeAPIKeySecret:
		return []FieldDefinition{
			{
				Code: "api_key", Label: "API key", InputType: "text",
				Required: true, Placeholder: "Masukkan API key",
				Help:         "Identifier API yang diterbitkan provider.",
				Capabilities: []string{},
			},
			{
				Code: "api_secret", Label: "API secret", InputType: "password",
				Secret: true, Required: true, Placeholder: "Masukkan API secret",
				Help:         "Secret tidak pernah dikembalikan setelah disimpan.",
				Capabilities: []string{},
			},
		}
	case CredentialTypeOAuth2ClientCredentials:
		return []FieldDefinition{
			{
				Code: "client_id", Label: "Client ID", InputType: "text",
				Required: true, Placeholder: "Masukkan OAuth client ID",
				Help:         "Client ID dari aplikasi yang didaftarkan pada provider.",
				Capabilities: []string{},
			},
			{
				Code: "client_secret", Label: "Client secret", InputType: "password",
				Secret: true, Required: true, Placeholder: "Masukkan OAuth client secret",
				Help:         "Client secret disimpan terenkripsi dan tidak ditampilkan kembali.",
				Capabilities: []string{},
			},
		}
	default:
		return []FieldDefinition{}
	}
}

func NormalizeCredentialValues(
	credentialType string,
	values map[string]string,
) (map[string]string, error) {
	if credentialType == CredentialTypeCapabilityAPIKeys {
		values = normalizeLegacyShippingAPIKey(values)
		if values == nil {
			return nil, errors.New("api_key and shipping_api_key cannot be combined")
		}
	}
	fields := FieldsForCredentialType(credentialType)
	if len(fields) == 0 {
		return nil, errors.New("provider does not accept merchant credentials")
	}
	allowed := make(map[string]FieldDefinition, len(fields))
	for _, field := range fields {
		allowed[field.Code] = field
	}
	normalized := make(map[string]string, len(fields))
	for code, raw := range values {
		field, exists := allowed[code]
		if !exists {
			return nil, errors.New("credential contains an unsupported field")
		}
		value := strings.TrimSpace(raw)
		if len(value) > 1024 || strings.ContainsAny(value, "\r\n\x00") {
			return nil, errors.New("credential field is invalid")
		}
		if field.Required && value == "" {
			return nil, errors.New("required credential field is empty")
		}
		if value != "" {
			normalized[code] = value
		}
	}
	for _, field := range fields {
		if field.Required && normalized[field.Code] == "" {
			return nil, errors.New("required credential field is missing")
		}
		if field.Secret && normalized[field.Code] != "" && len(normalized[field.Code]) < 8 {
			return nil, errors.New("secret credential field is too short")
		}
	}
	return normalized, nil
}

func normalizeLegacyShippingAPIKey(values map[string]string) map[string]string {
	legacy, hasLegacy := values["api_key"]
	_, hasShipping := values["shipping_api_key"]
	if hasLegacy && hasShipping {
		return nil
	}
	if !hasLegacy {
		return values
	}
	normalized := make(map[string]string, len(values))
	for code, value := range values {
		if code != "api_key" {
			normalized[code] = value
		}
	}
	normalized["shipping_api_key"] = legacy
	return normalized
}
