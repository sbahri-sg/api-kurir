package rates

import "context"

type Repository interface {
	FindActiveRateCards(ctx context.Context, request Request) ([]RateCard, error)
}

type CourierPresentation struct {
	Name         string
	Logo         string
	ServiceNames map[string]string
}

// CourierPresentationRepository is the preferred optional repository
// capability. Checkout presentation always comes from the local courier master,
// never from a provider's verbose or inconsistent display label.
type CourierPresentationRepository interface {
	FindActiveCourierPresentations(
		ctx context.Context,
		courierCodes []string,
	) (map[string]CourierPresentation, error)
}

// CourierLogoRepository remains as a compatibility capability for repository
// implementations that have not been upgraded to provide canonical names.
type CourierLogoRepository interface {
	FindActiveCourierLogos(
		ctx context.Context,
		courierCodes []string,
	) (map[string]string, error)
}
