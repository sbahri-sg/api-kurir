package biteship

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/tracking"
)

type credentialResolverStub struct {
	secret string
	alias  string
	limit  int64
	err    error
}

func (r credentialResolverStub) ResolveProviderCredential(
	context.Context,
	string,
) (string, string, int64, error) {
	return r.secret, r.alias, r.limit, r.err
}

type quotaStub struct {
	calls int
	err   error
}

func (q *quotaStub) ConsumeProviderHit(
	context.Context,
	string,
	string,
	int64,
) error {
	q.calls++
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

	quota := &quotaStub{}
	adapter := NewDynamicTrackingAdapter(
		credentialResolverStub{
			secret: "biteship_test.valid", alias: "biteship-test", limit: 100,
		},
		server.URL,
		time.Second,
		quota,
		nil,
		[]string{"sicepat"},
	)
	result, err := adapter.Track(context.Background(), tracking.Request{
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
