package enginegrant

import (
	"context"
	"errors"
	"regexp"
	"slices"
)

// ProviderBinding is provisioned by a trusted installation synchronizer, never
// from checkout input. It contains references only, not provider credentials.
type ProviderBinding struct {
	MerchantID, ProviderCode, AppID, InstallationID, CredentialID string
	Active                                                        bool
}

var ErrBindingNotFound = errors.New("provider installation binding not found")
var providerCode = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

type BindingResolver interface {
	Resolve(context.Context, string, string) (ProviderBinding, error)
}

// ProviderGrantVerifier must verify the current installation, permission and
// exact binding against Apps Platform. A saved Active flag is not authorization.
type ProviderGrantVerifier interface {
	Verify(context.Context, ProviderBinding, string) error
}

// CredentialVerifier checks ownership and provider association without exposing
// a credential to the app/browser. Existing API-Kurir credential storage remains
// the source of truth; an installation does not make a credential valid.
type CredentialVerifier interface {
	VerifyCredential(context.Context, string, string, string) error
}

type ProviderPolicy struct {
	RequiresCredential bool
	// MerchantIDs is an explicit operator rollout list, never checkout input.
	MerchantIDs []string
}

type ProviderGate struct {
	policies    map[string]ProviderPolicy
	bindings    BindingResolver
	grants      ProviderGrantVerifier
	credentials CredentialVerifier
}

// NewProviderGate enrolls providers explicitly. Providers outside this map keep
// their existing authorization path, allowing built-in and legacy integrations
// to coexist during migration. An enrolled provider NEVER falls back to legacy.
func NewProviderGate(policies map[string]ProviderPolicy, bindings BindingResolver, grants ProviderGrantVerifier, credentials CredentialVerifier) (*ProviderGate, error) {
	if len(policies) == 0 || bindings == nil || grants == nil {
		return nil, ErrUnavailable
	}
	g := &ProviderGate{policies: map[string]ProviderPolicy{}, bindings: bindings, grants: grants, credentials: credentials}
	for code, policy := range policies {
		if code == "emisell" || !providerCode.MatchString(code) || len(policy.MerchantIDs) == 0 || (policy.RequiresCredential && credentials == nil) {
			return nil, ErrUnavailable
		}
		for _, merchant := range policy.MerchantIDs {
			if !identifier.MatchString(merchant) {
				return nil, ErrUnavailable
			}
		}
		policy.MerchantIDs = slices.Clone(policy.MerchantIDs)
		g.policies[code] = policy
	}
	return g, nil
}

// Authorize receives merchant identity resolved by the authenticated gateway and
// provider selected by API-Kurir, not a provider/app/credential supplied by UI.
func (g *ProviderGate) Authorize(ctx context.Context, merchant, provider, operation string) error {
	if g == nil {
		return ErrUnavailable
	}
	if !identifier.MatchString(merchant) || !providerCode.MatchString(provider) {
		return ErrDenied
	}
	switch operation {
	case "rates.read", "settings.read", "settings.write", "shipments.create", "tracking.read":
	default:
		return ErrDenied
	}
	policy, enrolled := g.policies[provider]
	if !enrolled || !slices.Contains(policy.MerchantIDs, merchant) {
		return nil
	}
	binding, err := g.bindings.Resolve(ctx, merchant, provider)
	if errors.Is(err, ErrBindingNotFound) {
		return ErrDenied
	}
	if err != nil {
		return ErrUnavailable
	}
	if binding.MerchantID != merchant || binding.ProviderCode != provider || !binding.Active ||
		!identifier.MatchString(binding.AppID) || !identifier.MatchString(binding.InstallationID) {
		return ErrDenied
	}
	if err := g.grants.Verify(ctx, binding, operation); err != nil {
		if errors.Is(err, ErrDenied) {
			return ErrDenied
		}
		return ErrUnavailable
	}
	if policy.RequiresCredential {
		if !identifier.MatchString(binding.CredentialID) {
			return ErrDenied
		}
		if err := g.credentials.VerifyCredential(ctx, merchant, provider, binding.CredentialID); err != nil {
			if errors.Is(err, ErrDenied) {
				return ErrDenied
			}
			return ErrUnavailable
		}
	}
	return nil
}
