package hosted

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/fulfillment"
	"github.com/emisell/api-kurir/internal/providercredentials"
	"github.com/emisell/api-kurir/internal/rates"
	"github.com/emisell/api-kurir/internal/tracking"
)

type credentialStub struct{}

func (credentialStub) ResolveProviderCredential(
	context.Context,
	string,
) (string, string, int64, error) {
	return "seller-key", "seller", 50_000, nil
}

func (credentialStub) ResolveProviderCredentialForCapability(
	context.Context,
	string,
	string,
) (string, string, int64, error) {
	return "seller-key", "seller", 50_000, nil
}

type mappingStub struct{}

func (mappingStub) ResolveProviderLocation(
	_ context.Context,
	locationPublicID, _ string,
) (string, error) {
	if locationPublicID == "origin" {
		return "1391", nil
	}
	return "1376", nil
}

type quotaStub struct{ hits int }

func (q *quotaStub) ConsumeProviderHit(
	context.Context,
	string,
	string,
	int64,
) error {
	q.hits++
	return nil
}

func TestHostedRateAndTrackingAdapters(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("key") != "seller-key" {
			http.Error(response, "unauthorized", http.StatusUnauthorized)
			return
		}
		if request.Header.Get("X-Emisell-Execution-Mode") != "live" {
			http.Error(response, "missing execution mode", http.StatusBadRequest)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/partner/v1/rates":
			_ = json.NewEncoder(response).Encode(map[string]any{
				"data": map[string]any{"quotes": []map[string]any{{
					"provider_code": "rajaongkir", "courier_code": "J&T Express",
					"courier_name": "J&T", "service_code": "EZ",
					"service_name": "Regular", "service_group": "regular",
					"price": 15_000, "etd": "2-3 day",
				}}},
			})
		case "/partner/v1/tracking/waybills":
			_ = json.NewEncoder(response).Encode(map[string]any{
				"data": map[string]any{
					"waybill_number": "JY1234567890", "courier_code": "j&t",
					"courier_name": "J&T", "status": "delivered", "delivered": true,
					"history": []map[string]any{{
						"code": "DELIVERED", "description": "Paket diterima",
						"event_at": "2026-08-26 10:00:00", "location": "Sleman",
					}},
				},
			})
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/partner/v1", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	quota := &quotaStub{}
	rateAdapter := NewRateProvider(
		"rajaongkir", client, credentialStub{}, mappingStub{}, quota, 24*time.Hour,
	)
	quotes, err := rateAdapter.Quote(context.Background(), rates.Request{
		Origin: "origin", Destination: "destination", ActualWeightGrams: 1_000,
		Couriers: []string{"jnt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(quotes) != 1 || quotes[0].Cost != 15_000 || quotes[0].ServiceGroup != "regular" ||
		quotes[0].CourierCode != "jnt" {
		t.Fatalf("unexpected quotes: %#v", quotes)
	}

	trackingAdapter := NewTrackingAdapter(
		"rajaongkir", client, credentialStub{}, quota, []string{"jnt"},
	)
	tracked, err := trackingAdapter.Track(context.Background(), tracking.Request{
		CourierCode: "jnt", Waybill: "JY1234567890",
	})
	if err != nil {
		t.Fatal(err)
	}
	if tracked.NormalizedStatus != "delivered" || !tracked.IsFinal || len(tracked.Events) != 1 {
		t.Fatalf("unexpected tracking result: %#v", tracked)
	}
	if tracked.Summary["courier_code"] != "jnt" {
		t.Fatalf("tracking courier was not canonicalized: %#v", tracked.Summary)
	}
	if quota.hits != 2 {
		t.Fatalf("quota hits=%d want 2", quota.hits)
	}
}

func TestHostedFulfillmentAdapterUsesDeliveryCredential(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/partner/v1/shipments" || request.Header.Get("x-api-key") != "delivery-key" {
			http.Error(response, "unexpected request", http.StatusBadRequest)
			return
		}
		var input struct {
			MerchantReference string `json:"merchant_reference"`
			Payment           struct {
				ServiceFee int64 `json:"service_fee"`
			} `json:"payment"`
		}
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil ||
			input.MerchantReference != "order-1" || input.Payment.ServiceFee != 2500 {
			http.Error(response, "invalid payload", http.StatusUnprocessableEntity)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(response).Encode(map[string]any{
			"data": map[string]any{
				"partner_shipment_id": "RO-1", "waybill_number": "AWB-1", "status": "created",
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/partner/v1", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	adapter := NewFulfillmentAdapter("rajaongkir", client)
	result, err := adapter.Create(context.Background(), "delivery-key", fulfillment.CreateRequest{
		MerchantReference: "order-1",
		Payment:           fulfillment.Payment{ServiceFee: 2500},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProviderShipmentID != "RO-1" || result.AWB != "AWB-1" || result.Status != fulfillment.StatusBooked {
		t.Fatalf("unexpected fulfillment result: %#v", result)
	}
}

func TestHostedFulfillmentQuoteUsesDeliveryCredential(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/partner/v1/fulfillment/quotes" || request.Header.Get("x-api-key") != "delivery-key" {
			http.Error(response, "unexpected request", http.StatusBadRequest)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(map[string]any{
			"data": map[string]any{"quotes": []map[string]any{{
				"courier_code": "jne", "courier_name": "JNE",
				"service_code": "JNEFlat", "service_name": "Regular",
				"service_group": "regular", "delivery_mode": "regular",
				"shipping_cost": 10500, "grand_total": 110500,
				"currency": "IDR", "expires_at": "2026-08-27T09:00:00Z",
			}}},
		})
	}))
	defer server.Close()
	client, err := NewClient(server.URL+"/partner/v1", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	quotes, err := NewFulfillmentAdapter("rajaongkir", client).Quote(
		context.Background(), "delivery-key", fulfillment.QuoteRequest{},
	)
	if err != nil || len(quotes) != 1 || quotes[0].ServiceCode != "JNEFlat" || quotes[0].ShippingCost != 10500 {
		t.Fatalf("unexpected fulfillment quote: %#v err=%v", quotes, err)
	}
}

func TestHostedPickupForwardsSandboxModeAndMapsWaybill(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/partner/v1/pickups" || request.Header.Get("x-api-key") != "delivery-key" {
			http.Error(response, "unexpected request", http.StatusBadRequest)
			return
		}
		if request.Header.Get("X-Emisell-Execution-Mode") != providercredentials.EnvironmentSandbox {
			http.Error(response, "sandbox mode was not forwarded", http.StatusBadRequest)
			return
		}
		var input map[string]any
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil || input["provider_shipment_id"] != "KOM-1" {
			http.Error(response, "invalid payload", http.StatusUnprocessableEntity)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(response).Encode(map[string]any{
			"data": map[string]any{
				"pickup_id": "PICKUP-1", "partner_shipment_id": "KOM-1",
				"waybill_number": "AWB-1", "status": "requested",
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/partner/v1", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	adapter := NewFulfillmentAdapter("rajaongkir", client)
	ctx := providercredentials.WithExecutionEnvironment(
		context.Background(), providercredentials.EnvironmentSandbox,
	)
	result, err := adapter.Pickup(ctx, "delivery-key", fulfillment.Shipment{
		ProviderShipmentID: "KOM-1",
	}, fulfillment.PickupRequest{
		ScheduledAt: time.Now().Add(time.Hour), Vehicle: "motor",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProviderOperationID != "PICKUP-1" || result.AWB != "AWB-1" ||
		result.Status != fulfillment.StatusPickupRequested {
		t.Fatalf("unexpected pickup result: %#v", result)
	}
}
