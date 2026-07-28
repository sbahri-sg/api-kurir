package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/tracking"
	"github.com/labstack/echo/v5"
)

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
