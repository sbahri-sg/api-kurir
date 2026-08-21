package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/emisell/api-kurir/internal/couriers"
	"github.com/emisell/api-kurir/internal/merchantshipping"
	"github.com/emisell/api-kurir/internal/tenancy"
	"github.com/labstack/echo/v5"
)

type shippingPreferenceRepository struct {
	preference merchantshipping.Preference
}

func (repository *shippingPreferenceRepository) Get(
	context.Context,
	string,
) (merchantshipping.Preference, error) {
	return repository.preference, nil
}

func (repository *shippingPreferenceRepository) Replace(
	_ context.Context,
	_ string,
	input merchantshipping.UpdateInput,
) (merchantshipping.Preference, error) {
	repository.preference = merchantshipping.Preference{
		Configured:    true,
		Mode:          input.Mode,
		EnabledGroups: input.EnabledGroups,
		Services:      input.Services,
		Version:       1,
	}
	return repository.preference, nil
}

type shippingCourierRepository struct{}

func (shippingCourierRepository) List(context.Context) ([]couriers.Courier, error) {
	return []couriers.Courier{{
		Code: "jne",
		Name: "JNE",
		Services: []couriers.Service{
			{Code: "REG", Name: "JNE Regular", Group: "regular", ServiceType: "parcel"},
			{Code: "SPS", Name: "JNE Super Speed", Group: "express", ServiceType: "parcel"},
		},
	}}, nil
}

func TestTenantShippingServicesPutAndGet(t *testing.T) {
	t.Parallel()
	repository := &shippingPreferenceRepository{}
	service := merchantshipping.NewService(repository, shippingCourierRepository{})
	e := echo.New()
	e.PUT("/api/v1/integrations/shipping-services", withTenantIdentity(
		tenantShippingServiceUpdateHandler(service),
	))
	e.GET("/api/v1/integrations/shipping-services", withTenantIdentity(
		tenantShippingServiceCatalogHandler(service),
	))

	request := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/integrations/shipping-services",
		bytes.NewBufferString(`{
			"mode":"custom",
			"enabled_groups":[],
			"services":[{"courier_code":"JNE","service_code":"reg"}]
		}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/integrations/shipping-services", nil)
	response = httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", response.Code, response.Body.String())
	}
	if body := response.Body.String(); !bytes.Contains([]byte(body), []byte(`"selection_state":"partial"`)) ||
		!bytes.Contains([]byte(body), []byte(`"service_code":"REG"`)) {
		t.Fatalf("unexpected GET body=%s", body)
	}
}

func TestTenantShippingServicesRejectsUnknownService(t *testing.T) {
	t.Parallel()
	service := merchantshipping.NewService(
		&shippingPreferenceRepository{},
		shippingCourierRepository{},
	)
	e := echo.New()
	e.PUT("/api/v1/integrations/shipping-services", withTenantIdentity(
		tenantShippingServiceUpdateHandler(service),
	))
	request := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/integrations/shipping-services",
		bytes.NewBufferString(`{
			"mode":"custom",
			"enabled_groups":[],
			"services":[{"courier_code":"jne","service_code":"NOT_REAL"}]
		}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestTenantShippingServicesOnlyAcceptsCustomMode(t *testing.T) {
	t.Parallel()
	service := merchantshipping.NewService(
		&shippingPreferenceRepository{},
		shippingCourierRepository{},
	)
	e := echo.New()
	e.PUT("/api/v1/integrations/shipping-services", withTenantIdentity(
		tenantShippingServiceUpdateHandler(service),
	))
	request := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/integrations/shipping-services",
		bytes.NewBufferString(`{
			"mode":"groups",
			"enabled_groups":["regular"],
			"services":[]
		}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest ||
		!bytes.Contains(response.Body.Bytes(), []byte("INVALID_SHIPPING_SERVICE_PREFERENCE")) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestTenantShippingServicesReturnsAndEnforcesLimits(t *testing.T) {
	t.Parallel()
	service := merchantshipping.NewService(
		&shippingPreferenceRepository{},
		shippingCourierRepository{},
		merchantshipping.WithSelectionLimits(1, 1),
	)
	e := echo.New()
	e.PUT("/api/v1/integrations/shipping-services", withTenantIdentity(
		tenantShippingServiceUpdateHandler(service),
	))
	e.GET("/api/v1/integrations/shipping-services", withTenantIdentity(
		tenantShippingServiceCatalogHandler(service),
	))

	request := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/integrations/shipping-services",
		bytes.NewBufferString(`{
			"mode":"custom",
			"enabled_groups":[],
			"services":[
				{"courier_code":"jne","service_code":"REG"},
				{"courier_code":"jne","service_code":"SPS"}
			]
		}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity ||
		!bytes.Contains(response.Body.Bytes(), []byte("SHIPPING_SERVICE_LIMIT_EXCEEDED")) ||
		!bytes.Contains(response.Body.Bytes(), []byte(`"max_services":1`)) {
		t.Fatalf("PUT status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/integrations/shipping-services", nil)
	response = httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusOK ||
		!bytes.Contains(response.Body.Bytes(), []byte(`"maximum":1`)) ||
		!bytes.Contains(response.Body.Bytes(), []byte(`"selectable":true`)) {
		t.Fatalf("GET status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRegisterTenantIntegrationRoutesUsesCanonicalAPIPath(t *testing.T) {
	t.Parallel()
	e := echo.New()
	registerTenantIntegrationRoutes(
		e.Group("/api/v1/integrations"),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	registerTenantIntegrationRoutes(
		e.Group("/v1/integrations"),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	routes := make(map[string]bool)
	for _, route := range e.Router().Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, expected := range []string{
		"GET /api/v1/integrations/provider-credentials",
		"POST /api/v1/integrations/provider-credentials",
		"POST /api/v1/integrations/provider-credentials/:provider_code/disable",
		"GET /api/v1/integrations/providers",
		"POST /api/v1/integrations/providers/:provider_code/activate",
		"POST /api/v1/integrations/providers/:provider_code/deactivate",
		"GET /api/v1/integrations/shipping-services",
		"PUT /api/v1/integrations/shipping-services",
		"POST /api/v1/integrations/tracking/subscriptions",
		"GET /api/v1/integrations/tracking/subscriptions/:fulfillment_id",
		"GET /v1/integrations/provider-credentials",
	} {
		if !routes[expected] {
			t.Fatalf("route %q is not registered", expected)
		}
	}
}

func withTenantIdentity(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		request := c.Request().WithContext(tenancy.WithIdentity(
			c.Request().Context(),
			tenancy.Identity{TenantID: "merchant_123"},
		))
		c.SetRequest(request)
		return next(c)
	}
}
