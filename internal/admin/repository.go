package admin

import (
	"context"
	"errors"
)

var (
	ErrNotFound      = errors.New("admin resource not found")
	ErrConflict      = errors.New("admin resource conflict")
	ErrResourceInUse = errors.New("admin resource is still in use")
)

type Repository interface {
	Overview(ctx context.Context) (Overview, error)
	ListTrackingOperations(ctx context.Context, filter TrackingOperationFilter) (TrackingOperationPage, error)
	DeleteTrackingOperation(ctx context.Context, id, actorAlias, requestID string) error
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
	ListShippingProviders(ctx context.Context) ([]ShippingProvider, error)
	CreateShippingProvider(
		ctx context.Context,
		input ShippingProviderCreateInput,
		actorAlias string,
		requestID string,
	) (ShippingProvider, error)
	UpdateShippingProvider(
		ctx context.Context,
		code string,
		input ShippingProviderUpdateInput,
		actorAlias string,
		requestID string,
	) (ShippingProvider, error)
	Catalog(ctx context.Context) (Catalog, error)
}
