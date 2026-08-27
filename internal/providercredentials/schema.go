package providercredentials

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
)

const (
	CredentialTypeNone                    = "none"
	CredentialTypeAPIKey                  = "api_key"
	CredentialTypeCapabilityAPIKeys       = "capability_api_keys"
	CredentialTypeBearerToken             = "bearer_token"
	CredentialTypeAPIKeySecret            = "api_key_secret"
	CredentialTypeOAuth2ClientCredentials = "oauth2_client_credentials"
	CredentialTypeProviderDeclared        = "provider_declared"

	EnvironmentLive    = "live"
	EnvironmentSandbox = "sandbox"
)

var validCredentialFieldCode = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)

type environmentContextKey struct{}

type FieldOption struct {
	Value string `json:"value" yaml:"value"`
	Label string `json:"label" yaml:"label"`
}

type FieldDefinition struct {
	Code         string        `json:"code" yaml:"code"`
	Label        string        `json:"label" yaml:"label"`
	InputType    string        `json:"input_type" yaml:"input_type"`
	Secret       bool          `json:"secret" yaml:"secret"`
	Required     bool          `json:"required" yaml:"required"`
	Placeholder  string        `json:"placeholder" yaml:"placeholder"`
	Help         string        `json:"help" yaml:"help"`
	Capabilities []string      `json:"capabilities" yaml:"capabilities"`
	Environments []string      `json:"environments" yaml:"environments"`
	Options      []FieldOption `json:"options,omitempty" yaml:"options,omitempty"`
}

type EnvironmentDefinition struct {
	Code        string `json:"code" yaml:"code"`
	Label       string `json:"label" yaml:"label"`
	Description string `json:"description" yaml:"description"`
}

type CapabilityEnvironmentPolicy struct {
	Capability            string `json:"capability" yaml:"capability"`
	Environment           string `json:"environment" yaml:"environment"`
	Behavior              string `json:"behavior" yaml:"behavior"`
	CredentialEnvironment string `json:"credential_environment" yaml:"credential_environment"`
	Billing               string `json:"billing" yaml:"billing"`
}

func WithExecutionEnvironment(ctx context.Context, environment string) context.Context {
	environment = NormalizeEnvironment(environment)
	return context.WithValue(ctx, environmentContextKey{}, environment)
}

func ExecutionEnvironment(ctx context.Context) string {
	if ctx == nil {
		return EnvironmentLive
	}
	if value, ok := ctx.Value(environmentContextKey{}).(string); ok {
		return NormalizeEnvironment(value)
	}
	return EnvironmentLive
}

func NormalizeEnvironment(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case EnvironmentSandbox:
		return EnvironmentSandbox
	default:
		return EnvironmentLive
	}
}

func NormalizeEnvironmentStrict(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return EnvironmentLive, true
	}
	if value != EnvironmentLive && value != EnvironmentSandbox {
		return "", false
	}
	return value, true
}

func NormalizeCredentialType(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case CredentialTypeNone,
		CredentialTypeAPIKey,
		CredentialTypeCapabilityAPIKeys,
		CredentialTypeBearerToken,
		CredentialTypeAPIKeySecret,
		CredentialTypeOAuth2ClientCredentials,
		CredentialTypeProviderDeclared:
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
			Environments: []string{EnvironmentLive},
		}}
	case CredentialTypeCapabilityAPIKeys:
		return []FieldDefinition{
			{
				Code: "shipping_api_key", Label: "Shipping API key", InputType: "password",
				Secret: true, Required: true, Placeholder: "Masukkan API key tarif dan tracking",
				Help:         "Dipakai untuk cek ongkir dan tracking.",
				Capabilities: []string{"rates:read", "tracking:read"},
				Environments: []string{EnvironmentLive},
			},
			{
				Code: "delivery_api_key", Label: "Delivery API key", InputType: "password",
				Secret: true, Required: false, Placeholder: "Masukkan API key delivery (opsional)",
				Help:         "Dipakai untuk membuat pengiriman, pickup, pembatalan, dan label.",
				Capabilities: []string{"shipments:write", "shipments:read", "pickup:write", "labels:read", "shipments:cancel"},
				Environments: []string{EnvironmentLive, EnvironmentSandbox},
			},
		}
	case CredentialTypeBearerToken:
		return []FieldDefinition{{
			Code: "token", Label: "Access token", InputType: "password",
			Secret: true, Required: true, Placeholder: "Masukkan access token",
			Help:         "Token akan dikirim sebagai Bearer credential oleh adapter provider.",
			Capabilities: []string{},
			Environments: []string{EnvironmentLive},
		}}
	case CredentialTypeAPIKeySecret:
		return []FieldDefinition{
			{
				Code: "api_key", Label: "API key", InputType: "text",
				Required: true, Placeholder: "Masukkan API key",
				Help:         "Identifier API yang diterbitkan provider.",
				Capabilities: []string{},
				Environments: []string{EnvironmentLive},
			},
			{
				Code: "api_secret", Label: "API secret", InputType: "password",
				Secret: true, Required: true, Placeholder: "Masukkan API secret",
				Help:         "Secret tidak pernah dikembalikan setelah disimpan.",
				Capabilities: []string{},
				Environments: []string{EnvironmentLive},
			},
		}
	case CredentialTypeOAuth2ClientCredentials:
		return []FieldDefinition{
			{
				Code: "client_id", Label: "Client ID", InputType: "text",
				Required: true, Placeholder: "Masukkan OAuth client ID",
				Help:         "Client ID dari aplikasi yang didaftarkan pada provider.",
				Capabilities: []string{},
				Environments: []string{EnvironmentLive},
			},
			{
				Code: "client_secret", Label: "Client secret", InputType: "password",
				Secret: true, Required: true, Placeholder: "Masukkan OAuth client secret",
				Help:         "Client secret disimpan terenkripsi dan tidak ditampilkan kembali.",
				Capabilities: []string{},
				Environments: []string{EnvironmentLive},
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
	return NormalizeCredentialValuesForFields(
		FieldsForCredentialType(credentialType),
		EnvironmentLive,
		values,
	)
}

func NormalizeCredentialValuesForFields(
	fields []FieldDefinition,
	environment string,
	values map[string]string,
) (map[string]string, error) {
	environment = NormalizeEnvironment(environment)
	fields = FieldsForEnvironment(fields, environment)
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
		if field.InputType == "select" && value != "" && !containsOption(field.Options, value) {
			return nil, errors.New("credential field contains an unsupported option")
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
	if len(normalized) == 0 {
		return nil, errors.New("credential must contain at least one field")
	}
	return normalized, nil
}

func FieldsForEnvironment(fields []FieldDefinition, environment string) []FieldDefinition {
	environment = NormalizeEnvironment(environment)
	result := make([]FieldDefinition, 0, len(fields))
	for _, field := range fields {
		if len(field.Environments) == 0 || containsString(field.Environments, environment) {
			result = append(result, field)
		}
	}
	return result
}

func ValidateFieldDefinitions(fields []FieldDefinition, environments []EnvironmentDefinition) error {
	knownEnvironments := make(map[string]struct{}, len(environments))
	for _, environment := range environments {
		code := strings.ToLower(strings.TrimSpace(environment.Code))
		if code != EnvironmentLive && code != EnvironmentSandbox {
			return errors.New("credential environment is unsupported")
		}
		knownEnvironments[code] = struct{}{}
	}
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if !validCredentialFieldCode.MatchString(field.Code) || strings.TrimSpace(field.Label) == "" {
			return errors.New("credential field code or label is invalid")
		}
		if _, exists := seen[field.Code]; exists {
			return errors.New("credential field code is duplicated")
		}
		seen[field.Code] = struct{}{}
		switch field.InputType {
		case "text", "password", "select", "checkbox":
		default:
			return errors.New("credential input type is unsupported")
		}
		if field.Secret && field.InputType != "password" {
			return errors.New("secret credential field must use password input")
		}
		if field.InputType == "select" && len(field.Options) == 0 {
			return errors.New("select credential field must declare options")
		}
		for _, environment := range field.Environments {
			environment = strings.ToLower(strings.TrimSpace(environment))
			if _, exists := knownEnvironments[environment]; !exists {
				return errors.New("credential field references an undeclared environment")
			}
		}
	}
	return nil
}

func capabilityName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if separator := strings.IndexByte(value, ':'); separator >= 0 {
		value = value[:separator]
	}
	switch value {
	case "labels":
		return "shipments"
	case "rates", "tracking", "shipments", "pickup", "balance":
		return value
	default:
		return value
	}
}

func NormalizeFieldDefinitions(fields []FieldDefinition) []FieldDefinition {
	result := append([]FieldDefinition(nil), fields...)
	for index := range result {
		result[index].Code = strings.ToLower(strings.TrimSpace(result[index].Code))
		result[index].Label = strings.TrimSpace(result[index].Label)
		result[index].InputType = strings.ToLower(strings.TrimSpace(result[index].InputType))
		result[index].Placeholder = strings.TrimSpace(result[index].Placeholder)
		result[index].Help = strings.TrimSpace(result[index].Help)
		result[index].Capabilities = normalizedStrings(result[index].Capabilities)
		result[index].Environments = normalizedStrings(result[index].Environments)
	}
	return result
}

func containsOption(options []FieldOption, value string) bool {
	for _, option := range options {
		if option.Value == value {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}

func normalizedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
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
