package merchantproviders

import "context"

type Repository interface {
	Catalog(ctx context.Context, tenantID string) (Catalog, error)
	Activate(
		ctx context.Context,
		tenantID string,
		providerCode string,
		input ChangeInput,
	) (Catalog, error)
	Deactivate(
		ctx context.Context,
		tenantID string,
		providerCode string,
		input ChangeInput,
	) (Catalog, error)
}
