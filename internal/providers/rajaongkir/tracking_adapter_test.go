package rajaongkir

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/tracking"
)

type trackingQuotaStub struct {
	calls int
	err   error
}

func (q *trackingQuotaStub) ConsumeProviderHit(
	context.Context,
	string,
	string,
	int64,
) error {
	q.calls++
	return q.err
}

type providerCallRecorderStub struct {
	calls     int
	outcome   string
	quotaCost int
}

func (r *providerCallRecorderStub) RecordProviderAPICall(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ string,
	_ int,
	outcome string,
	_ time.Duration,
	quotaCost int,
	_ string,
) error {
	r.calls++
	r.outcome = outcome
	r.quotaCost = quotaCost
	return nil
}

func TestTrackingAdapterNormalizesAndSchedulesRefresh(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Timeout: time.Second,
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{
				"meta":{"message":"success","code":200},
				"data":{
					"delivered":false,
					"summary":{
						"courier_code":"tiki",
						"courier_name":"TIKI",
						"service_code":"REG",
						"origin":"Jakarta",
						"destination":"Bandung",
						"status":"OUT FOR DELIVERY"
					},
					"details":{"weight":"1.0"},
					"delivery_status":{"status":""},
					"manifest":[{
						"manifest_code":"OFD",
						"manifest_description":"With delivery courier",
						"manifest_date":"2026-07-28",
						"manifest_time":"09:30",
						"city_name":"Bandung"
					}]
				}
			}`), nil
		}),
	}
	client, err := NewClient("https://provider.test/", "test-secret", httpClient)
	if err != nil {
		t.Fatal(err)
	}
	quota := &trackingQuotaStub{}
	recorder := &providerCallRecorderStub{}
	adapter := NewTrackingAdapter(
		client,
		quota,
		recorder,
		"test-alias",
		50_000,
		[]string{"tiki"},
	)
	now := time.Date(2026, 7, 28, 3, 0, 0, 0, time.UTC)
	adapter.now = func() time.Time { return now }

	result, err := adapter.Track(context.Background(), tracking.Request{
		CourierCode: "tiki",
		Waybill:     "TEST123456789",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.NormalizedStatus != "out_for_delivery" ||
		result.NextRefreshAt == nil ||
		!result.NextRefreshAt.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("unexpected normalized result: %#v", result)
	}
	if quota.calls != 1 || recorder.calls != 1 ||
		recorder.outcome != "success" || recorder.quotaCost != 1 {
		t.Fatalf(
			"unexpected accounting: quota=%d recorder=%#v",
			quota.calls,
			recorder,
		)
	}
}

func TestNormalizeTrackingResultMarksDeliveredFinal(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 28, 4, 0, 0, 0, time.UTC)
	result := normalizeTrackingResult(WaybillTracking{
		Delivered: true,
		Summary: WaybillSummary{
			CourierCode: "jne",
			Status:      "DELIVERED",
		},
	}, now)
	if result.NormalizedStatus != "delivered" ||
		!result.IsFinal ||
		result.NextRefreshAt != nil {
		t.Fatalf("unexpected delivered result: %#v", result)
	}
}

func TestDefaultTrackingCouriersIncludeAnterAja(t *testing.T) {
	t.Parallel()
	adapter := NewTrackingAdapter(nil, nil, nil, "test", 100, nil)
	if !adapter.supports("anteraja") {
		t.Fatal("AnterAja must be enabled in the default RajaOngkir tracking adapter")
	}
}
