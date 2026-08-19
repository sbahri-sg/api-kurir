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
	List(ctx context.Context) ([]Credential, error)
	ListForTenant(ctx context.Context, tenantID string) ([]Credential, error)
	Create(ctx context.Context, input CreateInput) (Credential, error)
	Disable(ctx context.Context, id, actor, requestID string) error
	DisableForTenant(ctx context.Context, tenantID, id, actor, requestID string) error
	ResolveActive(ctx context.Context, providerCode string) (StoredCredential, error)
}
