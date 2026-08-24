package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/emisell/api-kurir/internal/locations"
	"github.com/labstack/echo/v5"
)

type locationHTTPRepositoryStub struct{}

func (locationHTTPRepositoryStub) Search(
	context.Context,
	string,
	int,
	int,
) ([]locations.Location, error) {
	return []locations.Location{{
		PublicID:        "loc_idn_32_73_06_1001",
		CompatibilityID: 3273061001,
		ProvinceID:      "loc_idn_32",
		CityID:          "loc_idn_32_73",
		DistrictID:      "loc_idn_32_73_06",
		SubdistrictID:   "loc_idn_32_73_06_1001",
		Province:        "Jawa Barat",
		City:            "Kota Bandung",
		District:        "Cicendo",
		Subdistrict:     "Husein Sastranegara",
		PostalCode:      "40174",
		PostalCodes:     []string{"40174"},
	}}, nil
}

func (locationHTTPRepositoryStub) ListHierarchy(
	_ context.Context,
	level string,
	parentPublicID string,
) ([]locations.HierarchyLocation, error) {
	switch level {
	case "province":
		return []locations.HierarchyLocation{{
			PublicID:        "loc_idn_32",
			CompatibilityID: 32,
			Name:            "Jawa Barat",
		}}, nil
	case "city":
		if parentPublicID == "loc_idn_32" {
			return []locations.HierarchyLocation{{
				PublicID:        "loc_idn_32_73",
				CompatibilityID: 3273,
				Name:            "Kota Bandung",
			}}, nil
		}
	}
	return []locations.HierarchyLocation{}, nil
}

func (locationHTTPRepositoryStub) ResolvePublicID(
	_ context.Context,
	identifier string,
	_ string,
) (string, error) {
	return identifier, nil
}

func TestLocationSearchIsCompatibleWithRajaOngkirV2Fields(t *testing.T) {
	t.Parallel()

	e := echo.New()
	e.GET(
		"/v1/destination/domestic-destination",
		locationSearchHandler(legacyHTTPRepositoryStub{}),
	)
	request := httptest.NewRequest(
		http.MethodGet,
		"/v1/destination/domestic-destination?search=Husein",
		nil,
	)
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

	var payload struct {
		Meta struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
			Status  string `json:"status"`
		} `json:"meta"`
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Meta.Message != "Success Get Domestic Destinations" ||
		payload.Meta.Code != http.StatusOK ||
		payload.Meta.Status != "success" {
		t.Fatalf("unexpected meta: %#v", payload.Meta)
	}
	if len(payload.Data) != 1 {
		t.Fatalf("unexpected data: %#v", payload.Data)
	}

	item := payload.Data[0]
	for field, want := range map[string]string{
		"id":               "loc_idn_32_73_06_1001",
		"province_id":      "loc_idn_32",
		"city_id":          "loc_idn_32_73",
		"district_id":      "loc_idn_32_73_06",
		"subdistrict_id":   "loc_idn_32_73_06_1001",
		"province_name":    "Jawa Barat",
		"city_name":        "Kota Bandung",
		"district_name":    "Cicendo",
		"subdistrict_name": "Husein Sastranegara",
		"zip_code":         "40174",
	} {
		if got, _ := item[field].(string); got != want {
			t.Errorf("%s: got %q want %q", field, got, want)
		}
	}

	ids := map[string]struct{}{}
	for _, field := range []string{
		"province_id",
		"city_id",
		"district_id",
		"subdistrict_id",
	} {
		ids[item[field].(string)] = struct{}{}
	}
	if len(ids) != 4 {
		t.Fatalf("hierarchy IDs must be distinct: %#v", item)
	}
}

func TestLocationHierarchyUsesParentID(t *testing.T) {
	t.Parallel()

	e := echo.New()
	e.GET(
		"/v1/destination/city/:province_id",
		locationHierarchyHandler(
			locationHTTPRepositoryStub{},
			"city",
			"province_id",
			"Success Get City By Province ID",
		),
	)
	request := httptest.NewRequest(
		http.MethodGet,
		"/v1/destination/city/loc_idn_32",
		nil,
	)
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

	var payload struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 1 ||
		payload.Data[0].ID != "loc_idn_32_73" ||
		payload.Data[0].Name != "Kota Bandung" {
		t.Fatalf("unexpected response: %#v", payload.Data)
	}
}

func TestLocationSearchRajaOngkirV2UsesNumericID(t *testing.T) {
	t.Parallel()

	e := echo.New()
	e.Use(rajaOngkirV2CompatibilityMiddleware())
	e.Use(customerAPIKeyMiddleware([]string{"sdk-key"}, nil))
	e.GET(
		"/api/v1/destination/domestic-destination",
		locationSearchHandler(legacyHTTPRepositoryStub{}),
	)
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/destination/domestic-destination?search=Husein&limit=999&offset=0",
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

	var payload struct {
		Meta map[string]any   `json:"meta"`
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 1 || payload.Data[0]["id"] != float64(82995) {
		t.Fatalf("expected numeric compatibility ID: %#v", payload.Data)
	}
	if _, exists := payload.Data[0]["province_id"]; exists {
		t.Fatalf("compatibility response contains internal extension: %#v", payload.Data[0])
	}
	if _, exists := payload.Meta["request_id"]; exists {
		t.Fatalf("compatibility meta contains internal extension: %#v", payload.Meta)
	}
}

func TestRajaOngkirV2OfficialBasePathAlias(t *testing.T) {
	t.Parallel()

	e := echo.New()
	group := e.Group("/api/v1")
	group.Use(rajaOngkirV2CompatibilityMiddleware())
	registerCustomerRoutes(
		group,
		nil,
		legacyHTTPRepositoryStub{},
		nil,
		nil,
		nil,
		nil,
		[]string{"sdk-key"},
		nil,
	)
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/destination/province",
		nil,
	)
	request.Header.Set("key", "sdk-key")
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

	var payload struct {
		Data []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 1 ||
		payload.Data[0].ID != 32 ||
		payload.Data[0].Name != "MALUKU UTARA" {
		t.Fatalf("unexpected response: %#v", payload.Data)
	}
}

func TestRajaOngkirV2CityOmitsEmptyZipCode(t *testing.T) {
	t.Parallel()

	e := echo.New()
	e.Use(rajaOngkirV2CompatibilityMiddleware())
	e.GET(
		"/api/v1/destination/city/:province_id",
		locationHierarchyHandler(
			legacyHTTPRepositoryStub{},
			"city",
			"province_id",
			"Success Get City By Province ID",
		),
	)
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/destination/city/18",
		nil,
	)
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
	var payload struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 1 || payload.Data[0]["id"] != float64(256) {
		t.Fatalf("unexpected city response: %#v", payload.Data)
	}
	if _, exists := payload.Data[0]["zip_code"]; exists {
		t.Fatalf("empty zip_code must be omitted: %#v", payload.Data[0])
	}
}

func TestCustomerLocationContractUsesPathNotAuthenticationHeader(t *testing.T) {
	t.Parallel()

	e := echo.New()
	canonicalGroup := e.Group("/v1")
	registerCustomerRoutes(
		canonicalGroup,
		nil,
		legacyHTTPRepositoryStub{},
		nil,
		nil,
		nil,
		nil,
		[]string{"sdk-key"},
		nil,
	)
	rajaOngkirGroup := e.Group("/api/v1")
	rajaOngkirGroup.Use(rajaOngkirV2CompatibilityMiddleware())
	registerCustomerRoutes(
		rajaOngkirGroup,
		nil,
		legacyHTTPRepositoryStub{},
		nil,
		nil,
		nil,
		nil,
		[]string{"sdk-key"},
		nil,
	)

	tests := []struct {
		name        string
		path        string
		headerName  string
		headerValue string
		wantID      any
	}{
		{
			name:        "sdk path with key header",
			path:        "/api/v1/destination/domestic-destination?search=Husein",
			headerName:  "key",
			headerValue: "sdk-key",
			wantID:      float64(82995),
		},
		{
			name:        "sdk path with bearer header",
			path:        "/api/v1/destination/domestic-destination?search=Husein",
			headerName:  "Authorization",
			headerValue: "Bearer sdk-key",
			wantID:      float64(82995),
		},
		{
			name:        "canonical path with key header",
			path:        "/v1/destination/domestic-destination?search=Husein",
			headerName:  "key",
			headerValue: "sdk-key",
			wantID:      "loc_idn_32_73_06_1001",
		},
		{
			name:        "canonical path with bearer header",
			path:        "/v1/destination/domestic-destination?search=Husein",
			headerName:  "Authorization",
			headerValue: "Bearer sdk-key",
			wantID:      "loc_idn_32_73_06_1001",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			request.Header.Set(test.headerName, test.headerValue)
			response := httptest.NewRecorder()
			e.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("status: got %d body=%s", response.Code, response.Body.String())
			}
			var payload struct {
				Data []map[string]any `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if len(payload.Data) != 1 || payload.Data[0]["id"] != test.wantID {
				t.Fatalf("id: got %#v want %#v", payload.Data, test.wantID)
			}
		})
	}
}

func TestRajaOngkirV2SearchValidationUses422Envelope(t *testing.T) {
	t.Parallel()

	e := echo.New()
	e.Use(rajaOngkirV2CompatibilityMiddleware())
	e.Use(customerAPIKeyMiddleware([]string{"sdk-key"}, nil))
	e.GET(
		"/api/v1/destination/domestic-destination",
		locationSearchHandler(locationHTTPRepositoryStub{}),
	)
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/destination/domestic-destination",
		nil,
	)
	request.Header.Set("Authorization", "Bearer sdk-key")
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
	if payload.Meta.Code != http.StatusUnprocessableEntity ||
		payload.Meta.Status != "error" ||
		payload.Data != nil {
		t.Fatalf("unexpected error envelope: %#v", payload)
	}
}
