package providercredentials

import (
	"context"
	"strings"
)

type ValidatorRegistry struct {
	validators map[string]Validator
}

func NewValidatorRegistry(validators map[string]Validator) *ValidatorRegistry {
	copyOfValidators := make(map[string]Validator, len(validators))
	for code, validator := range validators {
		code = strings.ToLower(strings.TrimSpace(code))
		if code != "" && validator != nil {
			copyOfValidators[code] = validator
		}
	}
	return &ValidatorRegistry{validators: copyOfValidators}
}

func (r *ValidatorRegistry) Validate(
	ctx context.Context,
	providerCode, secret string,
) error {
	validator, exists := r.validators[strings.ToLower(strings.TrimSpace(providerCode))]
	if !exists {
		return ErrUnsupportedProvider
	}
	return validator.Validate(ctx, providerCode, secret)
}

func (r *ValidatorRegistry) ValidateCredentials(
	ctx context.Context,
	providerCode string,
	credentialType string,
	values map[string]string,
) error {
	validator, exists := r.validators[strings.ToLower(strings.TrimSpace(providerCode))]
	if !exists {
		return ErrUnsupportedProvider
	}
	if bundleValidator, ok := validator.(BundleValidator); ok {
		return bundleValidator.ValidateCredentials(
			ctx,
			providerCode,
			credentialType,
			values,
		)
	}

	var secret string
	switch credentialType {
	case CredentialTypeAPIKey:
		secret = values["api_key"]
	case CredentialTypeCapabilityAPIKeys:
		secret = values["shipping_api_key"]
	case CredentialTypeBearerToken:
		secret = values["token"]
	default:
		return ErrUnsupportedProvider
	}
	return validator.Validate(ctx, providerCode, secret)
}
