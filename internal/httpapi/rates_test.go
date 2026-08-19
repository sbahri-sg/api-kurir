package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/rates"
	"github.com/labstack/echo/v5"
)

type staticRateRepository struct {
	cards []rates.RateCard
}

func (r staticRateRepository) FindActiveRateCards(context.Context, rates.Request) ([]rates.RateCard, error) {
	return r.cards, nil
}

func TestCalculateHandlerRejectsMultipleJSONValues(t *testing.T) {
	t.Parallel()

	service := rates.NewService(staticRateRepository{}, time.Second)
	e := echo.New()
	e.POST("/v1/calculate/domestic-cost", calculateRateHandler(service))

	body := bytes.NewBufferString(
		`{"origin":"a","destination":"b","weight":1000,"courier":"jne"} {"unexpected":true}`,
	)
	request := httptest.NewRequest(http.MethodPost, "/v1/calculate/domestic-cost", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	e.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want %d, body=%s", response.Code, http.StatusBadRequest, response.Body.String())
	}
}

func TestNormalizeCalculateRequestSortsAndDeduplicatesCouriers(t *testing.T) {
	t.Parallel()

	request, err := normalizeCalculateRequest(calculateRequest{
		Origin:      " loc_origin ",
		Destination: "loc_destination",
		Weight:      1200,
		Courier:     "TIKI:jne:tiki",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Couriers) != 2 || request.Couriers[0] != "jne" || request.Couriers[1] != "tiki" {
		t.Fatalf("unexpected courier normalization: %#v", request.Couriers)
	}
}

func TestRateResponseIncludesCanonicalServiceGrouping(t *testing.T) {
	t.Parallel()

	calculatedAt := time.Date(2026, time.July, 29, 10, 0, 0, 0, time.UTC)
	response := rateResponse(rates.Result{
		Card: rates.RateCard{
			CourierCode:          "jne",
			CourierName:          "JNE",
			ServiceCode:          "JTR>130",
			ServiceName:          "JNE Trucking",
			CanonicalServiceCode: "JTR",
			ServiceGroup:         "cargo",
			ServiceType:          "cargo",
			ServiceVariantCode:   "JTR>130",
			EffectiveFrom:        calculatedAt,
			FetchedAt:            calculatedAt,
		},
	}, calculatedAt)

	service, ok := response["service"].(map[string]any)
	if !ok {
		t.Fatalf("service response has unexpected type: %#v", response["service"])
	}
	if service["code"] != "JTR>130" ||
		service["canonical_code"] != "JTR" ||
		service["group"] != "cargo" ||
		service["type"] != "cargo" ||
		service["variant_code"] != "JTR>130" {
		t.Fatalf("unexpected canonical service response: %#v", service)
	}
}

func TestRajaOngkirV2CalculateAcceptsFormAndReturnsFlatResponse(t *testing.T) {
	t.Parallel()

	service := rates.NewService(staticRateRepository{
		cards: []rates.RateCard{{
			CourierCode:            "jne",
			CourierName:            "JNE",
			ServiceCode:            "REG",
			ServiceName:            "Layanan Reguler",
			PricingModel:           "flat",
			BasePrice:              15000,
			WeightIncrementGrams:   1000,
			RoundingMode:           "ceil",
			RoundingIncrementGrams: 1000,
		}},
	}, time.Second)
	e := echo.New()
	e.Use(rajaOngkirV2CompatibilityMiddleware())
	e.Use(customerAPIKeyMiddleware([]string{"sdk-key"}, nil))
	e.POST(
		"/api/v1/calculate/domestic-cost",
		calculatePublicRateHandler(
			service,
			legacyHTTPRepositoryStub{},
			"subdistrict",
		),
	)

	form := url.Values{}
	form.Set("origin", "3273061001")
	form.Set("destination", "3212122001")
	form.Set("weight", "1000")
	form.Set("courier", "jne")
	form.Set("price", "lowest")
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/calculate/domestic-cost",
		bytes.NewBufferString(form.Encode()),
	)
	request.Header.Set("Authorization", "Bearer sdk-key")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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
		Meta map[string]any `json:"meta"`
		Data []struct {
			Name             string  `json:"name"`
			Code             string  `json:"code"`
			Service          string  `json:"service"`
			Description      string  `json:"description"`
			Cost             int64   `json:"cost"`
			ETD              string  `json:"etd"`
			CanonicalService *string `json:"canonical_service"`
			ServiceGroup     *string `json:"service_group"`
			ServiceType      *string `json:"service_type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 1 ||
		payload.Data[0].Code != "jne" ||
		payload.Data[0].Service != "REG" ||
		payload.Data[0].Cost != 15000 {
		t.Fatalf("unexpected response: %#v", payload)
	}
	if payload.Data[0].CanonicalService != nil ||
		payload.Data[0].ServiceGroup != nil ||
		payload.Data[0].ServiceType != nil {
		t.Fatalf("default contract must remain RajaOngkir-compatible: %#v", payload.Data[0])
	}
	if _, exists := payload.Meta["request_id"]; exists {
		t.Fatalf("compatibility meta contains internal extension: %#v", payload.Meta)
	}
}

func TestRajaOngkirV2CalculateCanIncludeServiceGrouping(t *testing.T) {
	t.Parallel()

	service := rates.NewService(staticRateRepository{
		cards: []rates.RateCard{{
			CourierCode:            "jne",
			CourierName:            "JNE",
			ServiceCode:            "JTR>130",
			ServiceName:            "JNE Trucking",
			CanonicalServiceCode:   "JTR",
			ServiceGroup:           "cargo",
			ServiceType:            "cargo",
			ServiceVariantCode:     "JTR>130",
			PricingModel:           "flat",
			BasePrice:              75_000,
			WeightIncrementGrams:   1_000,
			RoundingMode:           "ceil",
			RoundingIncrementGrams: 1_000,
		}},
	}, time.Second)
	e := echo.New()
	e.Use(rajaOngkirV2CompatibilityMiddleware())
	e.POST(
		"/api/v1/calculate/domestic-cost",
		calculatePublicRateHandler(service, legacyHTTPRepositoryStub{}, "subdistrict"),
	)

	form := url.Values{
		"origin":        {"3273061001"},
		"destination":   {"3212122001"},
		"weight":        {"10000"},
		"courier":       {"jne"},
		"include_group": {"true"},
	}
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/calculate/domestic-cost",
		bytes.NewBufferString(form.Encode()),
	)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}

	var payload struct {
		Data []struct {
			Service          string `json:"service"`
			CanonicalService string `json:"canonical_service"`
			ServiceGroup     string `json:"service_group"`
			ServiceType      string `json:"service_type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 1 ||
		payload.Data[0].Service != "JTR>130" ||
		payload.Data[0].CanonicalService != "JTR" ||
		payload.Data[0].ServiceGroup != "cargo" ||
		payload.Data[0].ServiceType != "cargo" {
		t.Fatalf("unexpected enriched response: %#v", payload.Data)
	}
}

func TestRajaOngkirV2CalculateRejectsInvalidIncludeGroup(t *testing.T) {
	t.Parallel()

	e := echo.New()
	e.Use(rajaOngkirV2CompatibilityMiddleware())
	e.POST(
		"/api/v1/calculate/domestic-cost",
		calculatePublicRateHandler(
			rates.NewService(staticRateRepository{}, time.Second),
			legacyHTTPRepositoryStub{},
			"subdistrict",
		),
	)
	form := url.Values{
		"origin":        {"3273061001"},
		"destination":   {"3212122001"},
		"weight":        {"1000"},
		"courier":       {"jne"},
		"include_group": {"yes-please"},
	}
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/calculate/domestic-cost",
		bytes.NewBufferString(form.Encode()),
	)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
}

func TestRajaOngkirV2CalculateContractDoesNotDependOnAuthenticationHeader(t *testing.T) {
	t.Parallel()

	service := rates.NewService(staticRateRepository{
		cards: []rates.RateCard{{
			CourierCode:            "jne",
			CourierName:            "JNE",
			ServiceCode:            "REG",
			ServiceName:            "Layanan Reguler",
			PricingModel:           "flat",
			BasePrice:              15_000,
			WeightIncrementGrams:   1_000,
			RoundingMode:           "ceil",
			RoundingIncrementGrams: 1_000,
		}},
	}, time.Second)
	e := echo.New()
	e.Use(rajaOngkirV2CompatibilityMiddleware())
	e.Use(customerAPIKeyMiddleware([]string{"sdk-key"}, nil))
	e.POST(
		"/api/v1/calculate/domestic-cost",
		calculatePublicRateHandler(service, legacyHTTPRepositoryStub{}, "subdistrict"),
	)

	responses := make([][]byte, 0, 2)
	for _, headers := range []map[string]string{
		{"key": "sdk-key"},
		{"Authorization": "Bearer sdk-key"},
	} {
		form := url.Values{
			"origin":      {"3273061001"},
			"destination": {"3212122001"},
			"weight":      {"1000"},
			"courier":     {"jne"},
		}
		request := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/calculate/domestic-cost",
			bytes.NewBufferString(form.Encode()),
		)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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
		t.Fatalf("authentication header changed contract:\nkey=%s\nbearer=%s", responses[0], responses[1])
	}
}

func TestRajaOngkirV2CalculateRejectsJSONWith415(t *testing.T) {
	t.Parallel()

	e := echo.New()
	e.Use(rajaOngkirV2CompatibilityMiddleware())
	e.POST(
		"/api/v1/calculate/domestic-cost",
		calculatePublicRateHandler(
			rates.NewService(staticRateRepository{}, time.Second),
			legacyHTTPRepositoryStub{},
			"subdistrict",
		),
	)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/calculate/domestic-cost",
		bytes.NewBufferString(`{"origin":4911,"destination":25976,"weight":1000,"courier":"jne"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)

	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status: got %d want %d body=%s", response.Code, http.StatusUnsupportedMediaType, response.Body)
	}
	var payload struct {
		Meta struct {
			Code int `json:"code"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Meta.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("unexpected response: %s", response.Body)
	}
}

func TestRajaOngkirV2CalculateAcceptsMultipartForm(t *testing.T) {
	t.Parallel()

	service := rates.NewService(staticRateRepository{
		cards: []rates.RateCard{{
			CourierCode:            "jne",
			CourierName:            "JNE",
			ServiceCode:            "REG",
			ServiceName:            "Layanan Reguler",
			PricingModel:           "flat",
			BasePrice:              15_000,
			WeightIncrementGrams:   1_000,
			RoundingMode:           "ceil",
			RoundingIncrementGrams: 1_000,
		}},
	}, time.Second)
	e := echo.New()
	e.Use(rajaOngkirV2CompatibilityMiddleware())
	e.POST(
		"/api/v1/calculate/domestic-cost",
		calculatePublicRateHandler(service, legacyHTTPRepositoryStub{}, "subdistrict"),
	)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range map[string]string{
		"origin": "3273061001", "destination": "3212122001",
		"weight": "1000", "courier": "jne",
	} {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/calculate/domestic-cost", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", response.Code, http.StatusOK, response.Body)
	}
}

func TestRajaOngkirV2CalculateRejectsInvalidCourierWith422(t *testing.T) {
	t.Parallel()

	service := rates.NewService(staticRateRepository{}, time.Second)
	e := echo.New()
	e.Use(rajaOngkirV2CompatibilityMiddleware())
	e.Use(customerAPIKeyMiddleware([]string{"sdk-key"}, nil))
	e.POST(
		"/api/v1/calculate/domestic-cost",
		calculatePublicRateHandler(
			service,
			locationHTTPRepositoryStub{},
			"subdistrict",
		),
	)

	form := url.Values{}
	form.Set("origin", "3273061001")
	form.Set("destination", "3212122001")
	form.Set("weight", "1000")
	form.Set("courier", "JNE!")
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/calculate/domestic-cost",
		bytes.NewBufferString(form.Encode()),
	)
	request.Header.Set("key", "sdk-key")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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
		t.Fatalf("unexpected response: %#v", payload)
	}
}
