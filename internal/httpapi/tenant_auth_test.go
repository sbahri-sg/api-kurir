package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/emisell/api-kurir/internal/tenancy"
	"github.com/labstack/echo/v5"
)

func TestMerchantContextMiddlewarePropagatesMerchant(t *testing.T) {
	t.Parallel()
	e := echo.New()
	e.Use(serviceAPIKeyMiddleware([]string{"emisell-service-key"}))
	e.Use(merchantContextMiddleware(true))
	e.GET("/protected", func(c *echo.Context) error {
		identity, ok := tenancy.FromContext(c.Request().Context())
		if !ok {
			t.Fatal("merchant identity not propagated")
		}
		return c.JSON(http.StatusOK, map[string]string{"merchant_id": identity.TenantID})
	})
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("key", "emisell-service-key")
	request.Header.Set(merchantIDHeader, "merchant_123")
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestMerchantContextMiddlewareRejectsMissingAndInvalidMerchant(t *testing.T) {
	t.Parallel()
	e := echo.New()
	e.Use(serviceAPIKeyMiddleware([]string{"emisell-service-key"}))
	e.Use(merchantContextMiddleware(true))
	e.GET("/protected", func(c *echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("key", "emisell-service-key")
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing merchant status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("key", "emisell-service-key")
	request.Header.Set(merchantIDHeader, "invalid merchant")
	response = httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid merchant status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestMerchantContextRejectsGeneratedCustomerKey(t *testing.T) {
	t.Parallel()
	authenticator := &stubCustomerKeyAuthenticator{valid: true}
	e := echo.New()
	e.Use(customerAPIKeyMiddleware([]string{"emisell-service-key"}, authenticator))
	e.Use(merchantContextMiddleware(false))
	e.GET("/public", func(c *echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/public", nil)
	request.Header.Set("key", "ek_live_customer")
	request.Header.Set(merchantIDHeader, "merchant_123")
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestMerchantContextMiddlewareAllowsMissingOptionalMerchant(t *testing.T) {
	t.Parallel()
	e := echo.New()
	e.Use(merchantContextMiddleware(false))
	e.GET("/public", func(c *echo.Context) error {
		if _, ok := tenancy.FromContext(c.Request().Context()); ok {
			t.Fatal("unexpected merchant context")
		}
		return c.NoContent(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/public", nil)
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
