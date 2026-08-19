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
	e.PUT("/v1/integrations/shipping-services", withTenantIdentity(
		tenantShippingServiceUpdateHandler(service),
	))
	e.GET("/v1/integrations/shipping-services", withTenantIdentity(
		tenantShippingServiceCatalogHandler(service),
	))

	request := httptest.NewRequest(
		http.MethodPut,
		"/v1/integrations/shipping-services",
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

	request = httptest.NewRequest(http.MethodGet, "/v1/integrations/shipping-services", nil)
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
	e.PUT("/v1/integrations/shipping-services", withTenantIdentity(
		tenantShippingServiceUpdateHandler(service),
	))
	request := httptest.NewRequest(
		http.MethodPut,
		"/v1/integrations/shipping-services",
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
