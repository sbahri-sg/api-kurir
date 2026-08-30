package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/emisell/api-kurir/internal/fulfillment"
	"github.com/emisell/api-kurir/internal/providercredentials"
	"github.com/emisell/api-kurir/internal/tenancy"
	"github.com/labstack/echo/v5"
)

func TestPickupNotAllowedErrorResponse(t *testing.T) {
	t.Parallel()
	e := echo.New()
	e.GET("/", func(c *echo.Context) error {
		return writeFulfillmentError(c, fulfillment.ErrPickupNotAllowed)
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error.Code != "PICKUP_NOT_ALLOWED" {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
}

func TestLabelNotReadyErrorResponse(t *testing.T) {
	t.Parallel()
	e := echo.New()
	e.GET("/", func(c *echo.Context) error {
		return writeFulfillmentError(c, &fulfillment.LabelNotReadyError{
			Reason: "awb_pending", ShipmentStatus: fulfillment.StatusPickupRequested, Retryable: true,
		})
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Error struct {
			Code    string         `json:"code"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error.Code != "LABEL_NOT_READY" || payload.Error.Details["reason"] != "awb_pending" ||
		payload.Error.Details["shipment_status"] != fulfillment.StatusPickupRequested ||
		payload.Error.Details["retryable"] != true || payload.Error.Details["retry_after_seconds"] != float64(15) {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
}

func TestDeliveryCredentialRequiredReportsExecutionEnvironment(t *testing.T) {
	t.Parallel()
	e := echo.New()
	e.GET("/", func(c *echo.Context) error {
		request := c.Request().WithContext(providercredentials.WithExecutionEnvironment(
			c.Request().Context(), providercredentials.EnvironmentSandbox,
		))
		c.SetRequest(request)
		return writeFulfillmentError(c, fulfillment.ErrCredentialUnavailable)
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Error struct {
			Code    string         `json:"code"`
			Message string         `json:"message"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error.Code != "DELIVERY_CREDENTIAL_REQUIRED" ||
		payload.Error.Message != "Credential Shipping Delivery untuk mode sandbox belum tersedia." ||
		payload.Error.Details["environment"] != "sandbox" ||
		payload.Error.Details["required_credential"] != "delivery_api_key" {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
}

func TestDeliveryCredentialRequiredReportsAutomaticSelection(t *testing.T) {
	t.Parallel()
	e := echo.New()
	e.GET("/", func(c *echo.Context) error {
		return writeFulfillmentError(c, fulfillment.ErrCredentialUnavailable)
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Error struct {
			Message string         `json:"message"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error.Details["environment"] != "auto" ||
		payload.Error.Message != "Credential Shipping Delivery valid belum tersedia pada live maupun sandbox." {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
}

func TestAmountMismatchErrorResponse(t *testing.T) {
	t.Parallel()
	e := echo.New()
	e.GET("/", func(c *echo.Context) error {
		return writeFulfillmentError(c, fulfillment.ErrAmountMismatch)
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity ||
		!bytes.Contains(response.Body.Bytes(), []byte(`"code":"AMOUNT_MISMATCH"`)) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestFulfillmentCreateRejectsRemovedLegacyFields(t *testing.T) {
	t.Parallel()
	e := echo.New()
	e.POST("/", fulfillmentCreateHandler(fulfillment.NewService(nil, nil, nil)))
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{
		"merchant_reference":"ORDER-1","quote_id":"quote-1","package":{"item_value":100000}
	}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest ||
		!bytes.Contains(response.Body.Bytes(), []byte(`"code":"INVALID_REQUEST"`)) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestFulfillmentHandlerStopsAfterErrorResponse(t *testing.T) {
	t.Parallel()
	e := echo.New()
	service := fulfillment.NewService(nil, nil, nil)
	e.GET("/api/v1/integrations/shipments/:shipment_id", func(c *echo.Context) error {
		request := c.Request().WithContext(tenancy.WithIdentity(
			c.Request().Context(), tenancy.Identity{TenantID: "merchant_123"},
		))
		c.SetRequest(request)
		return fulfillmentGetHandler(service)(c)
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/integrations/shipments/not-a-uuid", nil)
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if bytes.Count(response.Body.Bytes(), []byte(`"code"`)) != 1 ||
		bytes.Contains(response.Body.Bytes(), []byte(`"shipment_id":""`)) {
		t.Fatalf("handler wrote a success response after error: %s", response.Body.String())
	}
}
