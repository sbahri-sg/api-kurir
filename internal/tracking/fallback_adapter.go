package tracking

import (
	"context"
	"errors"
	"strings"
)

// FallbackAdapter routes a courier to the first supporting adapter. A second
// provider is contacted only when the primary cannot answer the tracking
// request, never when its credential or quota is invalid.
type FallbackAdapter struct {
	adapters     []Adapter
	courierCodes []string
}

func NewFallbackAdapter(adapters ...Adapter) *FallbackAdapter {
	filtered := make([]Adapter, 0, len(adapters))
	seen := make(map[string]struct{})
	courierCodes := make([]string, 0)
	for _, adapter := range adapters {
		if adapter == nil {
			continue
		}
		filtered = append(filtered, adapter)
		for _, code := range adapter.CourierCodes() {
			code = strings.ToLower(strings.TrimSpace(code))
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
	return &FallbackAdapter{adapters: filtered, courierCodes: courierCodes}
}

func (a *FallbackAdapter) Code() string { return "fallback" }

func (a *FallbackAdapter) CourierCodes() []string {
	return append([]string(nil), a.courierCodes...)
}

func (a *FallbackAdapter) Track(ctx context.Context, request Request) (Result, error) {
	courierCode := strings.ToLower(strings.TrimSpace(request.CourierCode))
	matched := false
	var lastErr error
	for _, adapter := range a.adapters {
		if !adapterSupportsCourier(adapter, courierCode) {
			continue
		}
		matched = true
		result, err := adapter.Track(ctx, request)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !allowsProviderFallback(err) {
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

func adapterSupportsCourier(adapter Adapter, courierCode string) bool {
	for _, supported := range adapter.CourierCodes() {
		if strings.EqualFold(strings.TrimSpace(supported), courierCode) {
			return true
		}
	}
	return false
}

func allowsProviderFallback(err error) bool {
	return errors.Is(err, ErrWaybillNotFound) ||
		errors.Is(err, ErrProviderUnavailable) ||
		errors.Is(err, ErrProviderTimeout) ||
		errors.Is(err, ErrAdapterUnavailable)
}
