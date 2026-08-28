package providercredentials

import (
	"context"
	"errors"
)

var (
	ErrNotFound                = errors.New("provider credential not found")
	ErrNoActiveCredential      = errors.New("no active provider credential")
	ErrAllCredentialsExhausted = errors.New("all provider credentials exhausted")
	ErrDuplicate               = errors.New("provider credential already exists")
	ErrEnvironmentUnavailable  = errors.New("provider environment is unavailable")
)

type Repository interface {
	CredentialType(ctx context.Context, providerCode string) (string, error)
	List(ctx context.Context) ([]Credential, error)
	ListForTenant(ctx context.Context, tenantID string) ([]Credential, error)
	Create(ctx context.Context, input CreateInput) (Credential, error)
	Disable(ctx context.Context, id, actor, requestID string) error
	DisableForTenantProvider(
		ctx context.Context,
		tenantID, providerCode, actor, requestID string,
	) error
	ActiveStoredForTenantProvider(
		ctx context.Context,
		tenantID, providerCode, environment string,
	) (StoredCredential, error)
	ActiveCredentialID(ctx context.Context, tenantID, providerCode string) (string, error)
	ResolveActive(ctx context.Context, providerCode string) (StoredCredential, error)
}

type SchemaRepository interface {
	CredentialDefinition(
		ctx context.Context,
		providerCode string,
	) (credentialType string, fields []FieldDefinition, err error)
	CredentialEnvironment(
		ctx context.Context,
		providerCode string,
		capability string,
		executionEnvironment string,
	) (string, error)
}
