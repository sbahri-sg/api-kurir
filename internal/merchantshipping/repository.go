package merchantshipping

import "context"

type Repository interface {
	Get(ctx context.Context, tenantID string) (Preference, error)
	SelectedCourierCodes(ctx context.Context, tenantID string) ([]string, error)
	Replace(ctx context.Context, tenantID string, input UpdateInput) (Preference, error)
}
