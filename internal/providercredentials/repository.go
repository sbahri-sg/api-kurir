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
	ActiveCredentialID(ctx context.Context, tenantID, providerCode string) (string, error)
	ResolveActive(ctx context.Context, providerCode string) (StoredCredential, error)
}
