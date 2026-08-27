package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/emisell/api-kurir/internal/fulfillment"
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
