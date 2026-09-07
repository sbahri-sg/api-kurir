package rates

import (
	"context"
	"errors"
)

// ActiveProviderResolver uses API-Kurir's current merchant provider selection.
type ActiveProviderResolver interface {
	ActiveProviderCode(context.Context, string) (string, error)
}

type ProviderRequestAuthorizer interface {
	Authorize(context.Context, string, string, string) error
}

// WithInstalledProviderAuthorization composes with the existing authorization
// hook and runs before cache/singleflight. The application composition must
// provision a real binding resolver/verifier before enabling this option.
func WithInstalledProviderAuthorization(providers ActiveProviderResolver, grants ProviderRequestAuthorizer) Option {
	return func(s *Service) {
		previous := s.requestAuthorization
		s.requestAuthorization = func(ctx context.Context, request Request) error {
			if previous != nil {
				if err := previous(ctx, request); err != nil {
					return err
				}
			}
			if providers == nil || grants == nil {
				return errors.New("installed provider authorization unavailable")
			}
			provider, err := providers.ActiveProviderCode(ctx, request.TenantID)
			if err != nil {
				return err
			}
			return grants.Authorize(ctx, request.TenantID, provider, "rates.read")
		}
	}
}
