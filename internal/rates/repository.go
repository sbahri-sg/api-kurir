package rates

import "context"

type Repository interface {
	FindActiveRateCards(ctx context.Context, request Request) ([]RateCard, error)
}

// CourierLogoRepository is an optional repository capability used to enrich
// provider quotes with presentation metadata from the local courier master.
type CourierLogoRepository interface {
	FindActiveCourierLogos(
		ctx context.Context,
		courierCodes []string,
	) (map[string]string, error)
}
