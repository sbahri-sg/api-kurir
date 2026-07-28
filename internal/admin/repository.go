package admin

import (
	"context"
	"errors"
)

var (
	ErrNotFound = errors.New("admin resource not found")
	ErrConflict = errors.New("admin resource conflict")
)

type Repository interface {
	Overview(ctx context.Context) (Overview, error)
	ListRateSnapshots(ctx context.Context, search string, limit, offset int) ([]RateSnapshot, error)
	ListRateCards(ctx context.Context, search string, limit, offset int) ([]RateCard, error)
	CreateRateCard(ctx context.Context, input RateCardInput, actorAlias, requestID string) (RateCard, error)
	DeprecateRateCard(ctx context.Context, id, actorAlias, requestID string) error
	ListLocationMappings(ctx context.Context, search, providerCode string, limit, offset int) ([]LocationMapping, error)
	UpsertLocationMapping(
		ctx context.Context,
		input LocationMappingInput,
		actorAlias string,
		requestID string,
	) (LocationMapping, error)
	ListProviderQuotas(ctx context.Context, limit int) ([]ProviderQuota, error)
	Catalog(ctx context.Context) (Catalog, error)
}
