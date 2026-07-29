package locations

import "context"

type Repository interface {
	Search(ctx context.Context, search string, limit, offset int) ([]Location, error)
	ListHierarchy(
		ctx context.Context,
		level string,
		parentPublicID string,
	) ([]HierarchyLocation, error)
	ResolvePublicID(
		ctx context.Context,
		identifier string,
		level string,
	) (string, error)
}
