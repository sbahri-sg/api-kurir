package merchantshipping

import "context"

type Repository interface {
	Get(ctx context.Context, tenantID string) (Preference, error)
	SelectedCourierCodes(ctx context.Context, tenantID string) ([]string, error)
	Replace(ctx context.Context, tenantID string, input UpdateInput) (Preference, error)
}

// ProviderCatalogRepository is implemented by provider-aware persistence.
// Keeping this capability separate preserves compatibility with lightweight
// repositories while production PostgreSQL always enforces provider scope.
type ProviderCatalogRepository interface {
	ActiveCatalog(ctx context.Context, tenantID string) (ProviderCatalog, error)
}
