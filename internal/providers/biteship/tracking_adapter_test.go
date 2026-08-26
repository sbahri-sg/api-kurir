package biteship

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/tenancy"
	"github.com/emisell/api-kurir/internal/tracking"
)

type credentialResolverStub struct {
	secret     string
	alias      string
	limit      int64
	err        error
	seenTenant *string
}

func (r credentialResolverStub) ResolveProviderCredential(
	ctx context.Context,
	_ string,
) (string, string, int64, error) {
	if r.seenTenant != nil {
		*r.seenTenant = tenancy.TenantID(ctx)
	}
	return r.secret, r.alias, r.limit, r.err
}

type quotaStub struct {
	calls      int
	err        error
	seenTenant *string
}

func (q *quotaStub) ConsumeProviderHit(
	ctx context.Context,
	_ string,
	_ string,
	_ int64,
) error {
	q.calls++
	if q.seenTenant != nil {
		*q.seenTenant = tenancy.TenantID(ctx)
	}
	return q.err
}

func TestTrackingAdapterNormalizesPublicTracking(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/trackings/AWB123456/couriers/sicepat" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "biteship_test.valid" {
			t.Fatal("missing Biteship authorization token")
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
          "success": true,
          "id": "tracking-1",
          "waybill_id": "AWB123456",
          "courier": {"company": "SiCepat"},
          "origin": {"contact_name": "Seller", "address": "Jakarta"},
          "destination": {"contact_name": "Buyer", "address": "Bandung"},
          "history": [{
            "note": "Package is on the way",
            "updated_at": "2026-08-20T08:00:00+07:00",
            "status": "inTransit"
          }],
          "status": "inTransit"
        }`))
	}))
	defer server.Close()

	resolverTenant := "not-called"
	quotaTenant := "not-called"
	quota := &quotaStub{seenTenant: &quotaTenant}
	adapter := NewDynamicTrackingAdapter(
		credentialResolverStub{
			secret: "biteship_test.valid", alias: "biteship-test", limit: 100,
			seenTenant: &resolverTenant,
		},
		server.URL,
		time.Second,
		quota,
		nil,
		[]string{"sicepat"},
	)
	ctx := tenancy.WithIdentity(
		context.Background(),
		tenancy.Identity{TenantID: "merchant_123"},
	)
	result, err := adapter.Track(ctx, tracking.Request{
		CourierCode: "sicepat", Waybill: "AWB123456",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProviderCode != "biteship" ||
		result.NormalizedStatus != "in_transit" ||
		result.IsFinal ||
		len(result.Events) != 1 ||
		quota.calls != 1 {
		t.Fatalf("unexpected tracking result: %#v quota=%d", result, quota.calls)
	}
	if result.NextRefreshAt == nil || result.NextRefreshAt.Sub(result.FetchedAt) != 12*time.Hour {
		t.Fatalf("expected economical twelve-hour checkpoint, got %#v", result.NextRefreshAt)
	}
	wantShipped := time.Date(2026, 8, 20, 1, 0, 0, 0, time.UTC)
	if result.ShippedAt == nil || !result.ShippedAt.Equal(wantShipped) ||
		result.DeliveredAt != nil {
		t.Fatalf("unexpected tracking milestones: %#v", result)
	}
	if resolverTenant != "" || quotaTenant != "" {
		t.Fatalf(
			"platform fallback must use global credential/quota: resolver=%q quota=%q",
			resolverTenant,
			quotaTenant,
		)
	}
}

func TestNormalizeBiteshipTrackingUsesDeliveredHistoryTime(t *testing.T) {
	t.Parallel()

	fetchedAt := time.Date(2026, 8, 22, 15, 0, 0, 0, time.UTC)
	result := normalizeBiteshipTracking(PublicTracking{
		Status: "delivered",
		History: []TrackingHistory{
			{Status: "picked", UpdatedAt: "2026-08-20T09:00:00+07:00"},
			{Status: "delivered", UpdatedAt: "2026-08-22T14:25:00+07:00"},
		},
	}, "sicepat", fetchedAt)
	wantShipped := time.Date(2026, 8, 20, 2, 0, 0, 0, time.UTC)
	wantDelivered := time.Date(2026, 8, 22, 7, 25, 0, 0, time.UTC)
	if result.ShippedAt == nil || !result.ShippedAt.Equal(wantShipped) ||
		result.DeliveredAt == nil || !result.DeliveredAt.Equal(wantDelivered) ||
		!result.IsFinal {
		t.Fatalf("unexpected delivered tracking result: %#v", result)
	}
}

func TestTrackingAdapterMapsPublicTrackingErrors(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"success":false,"code":40003003,"message":"Waybill not found"}`))
	}))
	defer server.Close()

	adapter := NewDynamicTrackingAdapter(
		credentialResolverStub{secret: "biteship_test.valid", alias: "test", limit: 100},
		server.URL, time.Second, &quotaStub{}, nil, []string{"sicepat"},
	)
	_, err := adapter.Track(context.Background(), tracking.Request{
		CourierCode: "sicepat", Waybill: "INVALID123",
	})
	if !errors.Is(err, tracking.ErrWaybillNotFound) {
		t.Fatalf("expected waybill not found, got %v", err)
	}
}

func TestDefaultTrackingCouriersExcludeInstantCouriers(t *testing.T) {
	t.Parallel()
	adapter := NewDynamicTrackingAdapter(nil, "", time.Second, nil, nil, nil)
	if !supportsTrackingCourier(adapter.CourierCodes(), "sicepat") {
		t.Fatal("SiCepat must be enabled as a default fallback")
	}
	if supportsTrackingCourier(adapter.CourierCodes(), "anteraja") {
		t.Fatal("AnterAja must use RajaOngkir tracking, not Biteship")
	}
	if supportsTrackingCourier(adapter.CourierCodes(), "paxel") {
		t.Fatal("Paxel must remain disabled while it is not used by Emisell")
	}
	if supportsTrackingCourier(adapter.CourierCodes(), "grab") ||
		supportsTrackingCourier(adapter.CourierCodes(), "gojek") {
		t.Fatal("instant couriers must not be enabled")
	}
}
