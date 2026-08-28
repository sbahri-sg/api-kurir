package rajaongkir

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/fulfillment"
)

func TestFulfillmentAdapterCreateMapsCanonicalRequest(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/order/api/v1/orders/store" || request.Header.Get("x-api-key") != "delivery-key" {
			t.Fatalf("unexpected request %s key=%q", request.URL.Path, request.Header.Get("x-api-key"))
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["shipping"] != "JNE" || payload["shipping_type"] != "REG" || payload["shipper_destination_id"].(float64) != 5969 {
			t.Fatalf("unexpected payload: %#v", payload)
		}
		items := payload["order_details"].([]any)
		item := items[0].(map[string]any)
		if item["product_price"].(float64) != 100000 || item["subtotal"].(float64) != 100000 ||
			item["product_width"].(float64) != 10 {
			t.Fatalf("unexpected item payload: %#v", item)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"meta":{"message":"Success Create New Order","code":201,"status":"success"},"data":{"order_id":9999,"order_no":"KOM-100"}}`))
	}))
	defer server.Close()

	adapter := NewFulfillmentAdapter(server.URL, time.Second)
	adapter.now = func() time.Time { return time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC) }
	result, err := adapter.Create(context.Background(), "delivery-key", validFulfillmentRequest())
	if err != nil {
		t.Fatal(err)
	}
	if result.ProviderShipmentID != "KOM-100" || result.Status != fulfillment.StatusBooked {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestFulfillmentAdapterMapsUnauthorizedWithoutLeakingBody(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"secret":"must-not-surface"}`))
	}))
	defer server.Close()
	adapter := NewFulfillmentAdapter(server.URL, time.Second)
	_, err := adapter.Create(context.Background(), "bad-key", validFulfillmentRequest())
	if err != fulfillment.ErrProviderUnauthorized {
		t.Fatalf("expected unauthorized, got %v", err)
	}
}

func TestPickupVehicleIsDerivedFromShipmentWeight(t *testing.T) {
	t.Parallel()
	tests := []struct {
		weight int64
		want   string
	}{
		{weight: 500, want: "Motor"},
		{weight: 5_000, want: "Motor"},
		{weight: 5_001, want: "Mobil"},
		{weight: 9_999, want: "Mobil"},
		{weight: 10_000, want: "Truk"},
	}
	for _, item := range tests {
		if got := pickupVehicleForWeight(item.weight); got != item.want {
			t.Fatalf("weight %d: got %q want %q", item.weight, got, item.want)
		}
	}
}

func TestFulfillmentAdapterPickupMapsGatewayContext(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/order/api/v1/pickup/request" {
			t.Fatalf("unexpected pickup path: %s", request.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["pickup_date"] != "2026-08-28" || payload["pickup_time"] != "09:30:00" ||
			payload["pickup_vehicle"] != "Truk" {
			t.Fatalf("unexpected pickup payload: %#v", payload)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"meta":{"message":"success"},"data":[{"status":"success","order_no":"KOM-200","awb":"JNE200"}]}`))
	}))
	defer server.Close()

	adapter := NewFulfillmentAdapter(server.URL, time.Second)
	result, err := adapter.Pickup(context.Background(), "delivery-key", fulfillment.Shipment{
		ProviderShipmentID: "KOM-200",
	}, fulfillment.PickupRequest{
		Mode: "scheduled", ScheduledAt: time.Date(2026, 8, 28, 2, 30, 0, 0, time.UTC),
		PackageWeightGrams: 12_000,
	})
	if err != nil || result.AWB != "JNE200" {
		t.Fatalf("pickup result=%+v err=%v", result, err)
	}
}

func TestFulfillmentAdapterDetailMapsAWBAndDeliveryStatus(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/order/api/v1/orders/detail" ||
			request.URL.Query().Get("order_no") != "KOM-100" {
			t.Fatalf("unexpected detail request: %s %s", request.Method, request.URL.String())
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"meta":{"message":"Success get order detail","code":200,"status":"success"},
			"data":{"order_no":"KOM-100","awb":"JY100200300","order_status":"Delivered","live_tracking_url":"https://tracking.example/KOM-100"}
		}`))
	}))
	defer server.Close()

	adapter := NewFulfillmentAdapter(server.URL, time.Second)
	result, err := adapter.Detail(context.Background(), "delivery-key", fulfillment.Shipment{
		ProviderShipmentID: "KOM-100", Status: fulfillment.StatusPickupRequested,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.AWB != "JY100200300" || result.Status != fulfillment.StatusDelivered ||
		result.LiveTrackingURL == "" {
		t.Fatalf("unexpected detail result: %+v", result)
	}
}

func validFulfillmentRequest() fulfillment.CreateRequest {
	return fulfillment.CreateRequest{
		BrandName: "Emisell", MerchantReference: "ORDER-1", QuoteID: "quote-1",
		CourierCode: "jne", ServiceCode: "REG", DeliveryMode: "regular", Fulfillment: "pickup",
		Sender:    fulfillment.Address{Name: "Toko", Phone: "0812", Email: "admin@example.com", Address: "Jalan A", DestinationID: 5969},
		Recipient: fulfillment.Address{Name: "Budi", Phone: "0813", Address: "Jalan B", DestinationID: 4956},
		Package: fulfillment.Package{
			WeightGrams: 1000, LengthCM: 10, WidthCM: 10, HeightCM: 5,
			Items: []fulfillment.Item{{Name: "Kaos", Quantity: 1, UnitPrice: 100000, Subtotal: 100000, WeightGrams: 1000}},
		},
		Payment: fulfillment.Payment{
			Type: "non_cod", ItemsSubtotal: 100000, ShippingCost: 18000, GrandTotal: 118000,
		},
	}
}
