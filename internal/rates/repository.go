package rates

import "context"

type Repository interface {
	FindActiveRateCards(ctx context.Context, request Request) ([]RateCard, error)
}
