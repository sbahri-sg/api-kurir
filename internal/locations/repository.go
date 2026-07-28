package locations

import "context"

type Repository interface {
	Search(ctx context.Context, search string, limit int) ([]Location, error)
}
