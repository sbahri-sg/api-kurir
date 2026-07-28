package couriers

import "context"

type Repository interface {
	List(ctx context.Context) ([]Courier, error)
}
