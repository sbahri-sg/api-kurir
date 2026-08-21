package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/tracking"
	"github.com/labstack/echo/v5"
)

type trackingHTTPAdapterStub struct {
	result      tracking.Result
	err         error
	lastRequest tracking.Request
}

func (a *trackingHTTPAdapterStub) Code() string { return "rajaongkir" }
func (a *trackingHTTPAdapterStub) CourierCodes() []string {
	return []string{"jne"}
}
func (a *trackingHTTPAdapterStub) Track(
	_ context.Context,
	request tracking.Request,
) (tracking.Result, error) {
	a.lastRequest = request
	return a.result, a.err
}

type trackingHTTPRepositoryStub struct{}

func (trackingHTTPRepositoryStub) Register(
	context.Context,
	string,
	string,
	string,
	[]byte,
	[]byte,
) (tracking.Shipment, error) {
	return tracking.Shipment{
		CourierCode:      "jne",
		WaybillMasked:    "********6789",
		NormalizedStatus: "unknown",
		RefreshQueued:    true,
	}, nil
}

func (trackingHTTPRepositoryStub) RegisterImmediate(
	ctx context.Context,
	courierCode string,
	waybillHash string,
	waybillMasked string,
	waybillCiphertext []byte,
	providerContextCiphertext []byte,
) (tracking.Shipment, error) {
	return trackingHTTPRepositoryStub{}.Register(
		ctx,
		courierCode,
		waybillHash,
		waybillMasked,
		waybillCiphertext,
		providerContextCiphertext,
	)
}

func (trackingHTTPRepositoryStub) Claim(
	context.Context,
	string,
	[]string,
) (tracking.Job, error) {
	return tracking.Job{}, tracking.ErrNoJobAvailable
}

func (trackingHTTPRepositoryStub) Complete(
	context.Context,
	tracking.Job,
	tracking.Result,
) error {
	return nil
}

func (trackingHTTPRepositoryStub) CompleteImmediate(
	context.Context,
	string,
	tracking.Result,
) error {
	return nil
}

func (trackingHTTPRepositoryStub) RecordNotFound(
	context.Context,
	tracking.Job,
	time.Time,
	*time.Time,
	bool,
) error {
	return nil
}

func (trackingHTTPRepositoryStub) RecordNotFoundImmediate(
	context.Context,
	string,
	time.Time,
	*time.Time,
	bool,
) error {
	return nil
}

func (trackingHTTPRepositoryStub) RecordImmediateFailure(
	context.Context,
	string,
	string,
	time.Time,
	bool,
) error {
	return nil
}

func (trackingHTTPRepositoryStub) UpsertSubscription(
	context.Context,
	string,
	string,
	string,
	int,
) (tracking.Subscription, error) {
	return tracking.Subscription{}, nil
}

func (trackingHTTPRepositoryStub) GetSubscription(
	context.Context,
	string,
) (tracking.Subscription, error) {
	return tracking.Subscription{}, tracking.ErrNotFound
}

func (trackingHTTPRepositoryStub) Fail(
	context.Context,
	tracking.Job,
	string,
	string,
	time.Time,
) error {
	return nil
}

func trackingTestService(t *testing.T) *tracking.Service {
	t.Helper()
	key := bytes.Repeat([]byte{11}, 32)
	cipher, err := tracking.NewCipher(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	return tracking.NewService(trackingHTTPRepositoryStub{}, cipher, "jne")
}

func TestTrackingHandlerAcceptsJNEWithoutPhoneSuffix(t *testing.T) {
	t.Parallel()

	e := echo.New()
	e.POST("/v1/track/waybill", trackingHandler(trackingTestService(t)))
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/track/waybill",
		bytes.NewBufferString(`{
			"waybill":"TEST123456789",
			"courier":"jne",
			"refresh":"if_stale"
		}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	e.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf(
			"status: got %d want %d, body=%s",
			response.Code,
			http.StatusAccepted,
			response.Body.String(),
		)
	}
	if !bytes.Contains(response.Body.Bytes(), []byte(`"courier":"jne"`)) {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
}

func TestTrackingHandlerRejectsUnsupportedCourier(t *testing.T) {
	t.Parallel()

	e := echo.New()
	e.POST("/v1/track/waybill", trackingHandler(trackingTestService(t)))
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/track/waybill",
		bytes.NewBufferString(`{
			"waybill":"TEST123456789",
			"courier":"sicepat",
			"refresh":"if_stale"
		}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	e.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf(
			"status: got %d want %d, body=%s",
			response.Code,
			http.StatusUnprocessableEntity,
			response.Body.String(),
		)
	}
}

func TestRajaOngkirV2TrackingReturnsSynchronousContract(t *testing.T) {
	t.Parallel()

	occurredAt := time.Date(2026, 7, 29, 3, 15, 0, 0, time.UTC)
	adapter := &trackingHTTPAdapterStub{result: tracking.Result{
		NormalizedStatus: "delivered",
		StatusLabel:      "Terkirim",
		Summary: map[string]any{
			"courier_code":    "jne",
			"courier_name":    "Jalur Nugraha Ekakurir (JNE)",
			"service_code":    "REG",
			"waybill_date":    "2026-07-28",
			"shipper_name":    "TOKO EMISell",
			"receiver_name":   "BUDI",
			"origin":          "JAKARTA",
			"destination":     "BANDUNG",
			"status":          "DELIVERED",
			"weight":          "1",
			"delivery_status": "DELIVERED",
			"pod_receiver":    "BUDI",
			"pod_date":        "2026-07-29",
			"pod_time":        "10:15",
			"delivered":       true,
		},
		Events: []tracking.Event{{
			Code:        "DELIVERED",
			Description: "Package delivered",
			Location:    "Bandung",
			OccurredAt:  occurredAt,
		}},
		ProviderCode: "rajaongkir",
		FetchedAt:    occurredAt,
		IsFinal:      true,
	}}
	e := echo.New()
	e.Use(rajaOngkirV2CompatibilityMiddleware())
	e.Use(customerAPIKeyMiddleware([]string{"sdk-key"}, nil))
	e.POST(
		"/api/v1/track/waybill",
		trackingPublicHandler(trackingTestService(t), adapter),
	)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/track/waybill?awb=TEST123456789&courier=jne",
		nil,
	)
	request.Header.Set("Authorization", "Bearer sdk-key")
	response := httptest.NewRecorder()

	e.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf(
			"status: got %d want %d, body=%s",
			response.Code,
			http.StatusOK,
			response.Body.String(),
		)
	}
	if adapter.lastRequest.LastPhoneDigits != "" {
		t.Fatalf("phone suffix must remain optional: %#v", adapter.lastRequest)
	}

	var payload struct {
		Meta map[string]any `json:"meta"`
		Data struct {
			Delivered bool `json:"delivered"`
			Summary   struct {
				CourierCode   string `json:"courier_code"`
				WaybillNumber string `json:"waybill_number"`
				Status        string `json:"status"`
			} `json:"summary"`
			DeliveryStatus struct {
				PODReceiver string `json:"pod_receiver"`
			} `json:"delivery_status"`
			Manifest []struct {
				Code string `json:"manifest_code"`
			} `json:"manifest"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Data.Delivered ||
		payload.Data.Summary.CourierCode != "jne" ||
		payload.Data.Summary.WaybillNumber != "TEST123456789" ||
		payload.Data.Summary.Status != "DELIVERED" ||
		payload.Data.DeliveryStatus.PODReceiver != "BUDI" ||
		len(payload.Data.Manifest) != 1 ||
		payload.Data.Manifest[0].Code != "DELIVERED" {
		t.Fatalf("unexpected response: %#v", payload)
	}
	if _, exists := payload.Meta["request_id"]; exists {
		t.Fatalf("compatibility meta contains internal extension: %#v", payload.Meta)
	}
}

func TestRajaOngkirV2TrackingNotFoundUses404Envelope(t *testing.T) {
	t.Parallel()

	adapter := &trackingHTTPAdapterStub{err: tracking.ErrWaybillNotFound}
	e := echo.New()
	e.Use(rajaOngkirV2CompatibilityMiddleware())
	e.Use(customerAPIKeyMiddleware([]string{"sdk-key"}, nil))
	e.POST(
		"/api/v1/track/waybill",
		trackingPublicHandler(trackingTestService(t), adapter),
	)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/track/waybill?awb=TEST123456789&courier=jne",
		nil,
	)
	request.Header.Set("key", "sdk-key")
	response := httptest.NewRecorder()

	e.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf(
			"status: got %d want %d, body=%s",
			response.Code,
			http.StatusNotFound,
			response.Body.String(),
		)
	}
	var payload struct {
		Meta struct {
			Code   int    `json:"code"`
			Status string `json:"status"`
		} `json:"meta"`
		Data any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Meta.Code != http.StatusNotFound ||
		payload.Meta.Status != "error" ||
		payload.Data != nil {
		t.Fatalf("unexpected response: %#v", payload)
	}
}

func TestRajaOngkirV2TrackingContractDoesNotDependOnAuthenticationHeader(t *testing.T) {
	t.Parallel()

	fetchedAt := time.Date(2026, 7, 29, 3, 15, 0, 0, time.UTC)
	adapter := &trackingHTTPAdapterStub{result: tracking.Result{
		NormalizedStatus: "in_transit",
		StatusLabel:      "Dalam perjalanan",
		Summary: map[string]any{
			"courier_code":    "jne",
			"courier_name":    "Jalur Nugraha Ekakurir (JNE)",
			"waybill_number":  "TEST123456789",
			"status":          "IN TRANSIT",
			"delivery_status": "IN TRANSIT",
		},
		ProviderCode: "rajaongkir",
		FetchedAt:    fetchedAt,
	}}
	e := echo.New()
	e.Use(rajaOngkirV2CompatibilityMiddleware())
	e.Use(customerAPIKeyMiddleware([]string{"sdk-key"}, nil))
	e.POST(
		"/api/v1/track/waybill",
		trackingPublicHandler(trackingTestService(t), adapter),
	)

	responses := make([][]byte, 0, 2)
	for _, headers := range []map[string]string{
		{"key": "sdk-key"},
		{"Authorization": "Bearer sdk-key"},
	} {
		request := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/track/waybill?awb=TEST123456789&courier=jne",
			nil,
		)
		for name, value := range headers {
			request.Header.Set(name, value)
		}
		response := httptest.NewRecorder()
		e.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("headers=%v status=%d body=%s", headers, response.Code, response.Body)
		}
		responses = append(responses, append([]byte(nil), response.Body.Bytes()...))
	}
	if !bytes.Equal(responses[0], responses[1]) {
		t.Fatalf(
			"key and bearer contracts differ:\nkey=%s\nbearer=%s",
			responses[0],
			responses[1],
		)
	}
}
