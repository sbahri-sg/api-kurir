package tracking

import (
	"context"
	"errors"
)

// FallbackAdapter routes a courier to the first supporting adapter. A second
// provider is contacted only when the primary cannot answer the request. Paid
// platform failover for credential/quota errors requires an explicit policy.
type FallbackAdapter struct {
	adapters     []Adapter
	courierCodes []string
	policy       ProviderFallbackPolicy
}

type ProviderFallbackPolicy interface {
	AllowsProviderFallback(ctx context.Context) (bool, error)
}

func NewFallbackAdapter(adapters ...Adapter) *FallbackAdapter {
	return newFallbackAdapter(nil, adapters...)
}

func NewPolicyFallbackAdapter(
	policy ProviderFallbackPolicy,
	adapters ...Adapter,
) *FallbackAdapter {
	return newFallbackAdapter(policy, adapters...)
}

func newFallbackAdapter(
	policy ProviderFallbackPolicy,
	adapters ...Adapter,
) *FallbackAdapter {
	filtered := make([]Adapter, 0, len(adapters))
	seen := make(map[string]struct{})
	courierCodes := make([]string, 0)
	for _, adapter := range adapters {
		if adapter == nil {
			continue
		}
		filtered = append(filtered, adapter)
		for _, code := range adapter.CourierCodes() {
			code = normalizeCourierCode(code)
			if code == "" {
				continue
			}
			if _, exists := seen[code]; exists {
				continue
			}
			seen[code] = struct{}{}
			courierCodes = append(courierCodes, code)
		}
	}
	return &FallbackAdapter{
		adapters: filtered, courierCodes: courierCodes, policy: policy,
	}
}

func (a *FallbackAdapter) Code() string { return "fallback" }

func (a *FallbackAdapter) CourierCodes() []string {
	return append([]string(nil), a.courierCodes...)
}

func (a *FallbackAdapter) Track(ctx context.Context, request Request) (Result, error) {
	courierCode := normalizeCourierCode(request.CourierCode)
	request.CourierCode = courierCode
	matched := false
	var lastErr error
	for index, adapter := range a.adapters {
		if !adapterSupportsCourier(adapter, courierCode) {
			continue
		}
		if index > 0 {
			allowed, err := a.fallbackAllowed(ctx)
			if err != nil {
				return Result{}, err
			}
			if !allowed {
				if lastErr != nil {
					return Result{}, lastErr
				}
				return Result{}, ErrUnsupportedCourier
			}
		}
		matched = true
		result, err := adapter.Track(ctx, request)
		if err == nil {
			return normalizeResultCourier(result, courierCode), nil
		}
		lastErr = err
		if !allowsProviderFallback(err) {
			return Result{}, err
		}
		if a.policy == nil && requiresPlatformFallbackAuthorization(err) {
			return Result{}, err
		}
	}
	if !matched {
		return Result{}, ErrUnsupportedCourier
	}
	if lastErr == nil {
		lastErr = ErrProviderUnavailable
	}
	return Result{}, lastErr
}

func requiresPlatformFallbackAuthorization(err error) bool {
	return errors.Is(err, ErrProviderQuota) ||
		errors.Is(err, ErrProviderRateLimited) ||
		errors.Is(err, ErrProviderUnauthorized) ||
		errors.Is(err, ErrPhoneSuffixRequired)
}

func (a *FallbackAdapter) fallbackAllowed(ctx context.Context) (bool, error) {
	if a.policy == nil {
		return true, nil
	}
	return a.policy.AllowsProviderFallback(ctx)
}

func adapterSupportsCourier(adapter Adapter, courierCode string) bool {
	for _, supported := range adapter.CourierCodes() {
		if normalizeCourierCode(supported) == courierCode {
			return true
		}
	}
	return false
}

func allowsProviderFallback(err error) bool {
	return errors.Is(err, ErrWaybillNotFound) ||
		errors.Is(err, ErrProviderUnavailable) ||
		errors.Is(err, ErrProviderTimeout) ||
		errors.Is(err, ErrAdapterUnavailable) ||
		errors.Is(err, ErrProviderQuota) ||
		errors.Is(err, ErrProviderRateLimited) ||
		errors.Is(err, ErrProviderUnauthorized) ||
		errors.Is(err, ErrPhoneSuffixRequired)
}
