package apikeys

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("api key not found")

type Repository interface {
	List(ctx context.Context, limit, offset int) ([]APIKey, error)
	Create(ctx context.Context, input CreateInput) (APIKey, error)
	Revoke(ctx context.Context, id, actor, requestID string) error
	Authenticate(ctx context.Context, keyHash []byte) (bool, error)
	AuthenticateScope(ctx context.Context, keyHash []byte, scope string) (bool, error)
}
