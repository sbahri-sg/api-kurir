package tracking

import (
	"context"
	"errors"
	"testing"
)

type fallbackAdapterStub struct {
	code     string
	couriers []string
	result   Result
	err      error
	calls    int
}

type fallbackPolicyStub struct {
	allowed bool
}

func (p fallbackPolicyStub) AllowsProviderFallback(context.Context) (bool, error) {
	return p.allowed, nil
}

func (a *fallbackAdapterStub) Code() string           { return a.code }
func (a *fallbackAdapterStub) CourierCodes() []string { return a.couriers }
func (a *fallbackAdapterStub) Track(context.Context, Request) (Result, error) {
	a.calls++
	return a.result, a.err
}

func TestFallbackAdapterRoutesUnsupportedPrimaryCourierDirectly(t *testing.T) {
	t.Parallel()
	primary := &fallbackAdapterStub{code: "rajaongkir", couriers: []string{"jne"}}
	secondary := &fallbackAdapterStub{
		code: "biteship", couriers: []string{"sicepat"},
		result: Result{ProviderCode: "biteship", NormalizedStatus: "in_transit"},
	}
	adapter := NewFallbackAdapter(primary, secondary)
	result, err := adapter.Track(context.Background(), Request{CourierCode: "sicepat"})
	if err != nil || result.ProviderCode != "biteship" {
		t.Fatalf("unexpected route result: result=%#v err=%v", result, err)
	}
	if primary.calls != 0 || secondary.calls != 1 {
		t.Fatalf("unexpected calls: primary=%d secondary=%d", primary.calls, secondary.calls)
	}
}

func TestFallbackAdapterUsesSecondProviderForAvailabilityError(t *testing.T) {
	t.Parallel()
	primary := &fallbackAdapterStub{
		code: "rajaongkir", couriers: []string{"sicepat"}, err: ErrWaybillNotFound,
	}
	secondary := &fallbackAdapterStub{
		code: "biteship", couriers: []string{"sicepat"},
		result: Result{ProviderCode: "biteship", NormalizedStatus: "in_transit"},
	}
	adapter := NewFallbackAdapter(primary, secondary)
	result, err := adapter.Track(context.Background(), Request{CourierCode: "sicepat"})
	if err != nil || result.ProviderCode != "biteship" {
		t.Fatalf("unexpected fallback result: result=%#v err=%v", result, err)
	}
}

func TestFallbackAdapterDoesNotBypassPrimaryQuotaOrCredential(t *testing.T) {
	t.Parallel()
	for _, primaryErr := range []error{ErrProviderQuota, ErrProviderUnauthorized, ErrProviderRateLimited} {
		primary := &fallbackAdapterStub{couriers: []string{"jne"}, err: primaryErr}
		secondary := &fallbackAdapterStub{couriers: []string{"jne"}}
		_, err := NewFallbackAdapter(primary, secondary).Track(
			context.Background(), Request{CourierCode: "jne"},
		)
		if !errors.Is(err, primaryErr) || secondary.calls != 0 {
			t.Fatalf("error %v must not call paid fallback; got %v calls=%d", primaryErr, err, secondary.calls)
		}
	}
}

func TestPolicyFallbackAdapterAllowsOperationalFallbackForEmisell(t *testing.T) {
	t.Parallel()
	primary := &fallbackAdapterStub{
		code: "rajaongkir", couriers: []string{"jnt"}, err: ErrProviderQuota,
	}
	secondary := &fallbackAdapterStub{
		code: "biteship", couriers: []string{"jnt"},
		result: Result{ProviderCode: "biteship", NormalizedStatus: "delivered"},
	}
	result, err := NewPolicyFallbackAdapter(
		fallbackPolicyStub{allowed: true}, primary, secondary,
	).Track(context.Background(), Request{CourierCode: "jnt"})
	if err != nil || result.ProviderCode != "biteship" || secondary.calls != 1 {
		t.Fatalf("result=%#v err=%v calls=%d", result, err, secondary.calls)
	}
}

func TestPolicyFallbackAdapterBlocksPlatformFallbackForBYOK(t *testing.T) {
	t.Parallel()
	primary := &fallbackAdapterStub{
		code: "rajaongkir", couriers: []string{"jnt"}, err: ErrProviderUnauthorized,
	}
	secondary := &fallbackAdapterStub{code: "biteship", couriers: []string{"jnt"}}
	_, err := NewPolicyFallbackAdapter(
		fallbackPolicyStub{allowed: false}, primary, secondary,
	).Track(context.Background(), Request{CourierCode: "jnt"})
	if !errors.Is(err, ErrProviderUnauthorized) || secondary.calls != 0 {
		t.Fatalf("err=%v secondary calls=%d", err, secondary.calls)
	}
}
