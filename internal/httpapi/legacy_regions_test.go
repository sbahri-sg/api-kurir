package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/locations"
	"github.com/emisell/api-kurir/internal/rates"
	"github.com/labstack/echo/v5"
)

type legacyHTTPRepositoryStub struct {
	locationHTTPRepositoryStub
}

func (legacyHTTPRepositoryStub) SearchLegacy(
	_ context.Context,
	provider string,
	_ string,
	_ int,
	_ int,
) ([]locations.LegacyRegion, error) {
	if provider != legacyRegionProvider {
		return nil, locations.ErrLocationNotFound
	}
	return []locations.LegacyRegion{{
		ID:                "82995",
		CanonicalPublicID: "loc_legacy_rajaongkir_subdistrict_82995",
		Name:              "SASA",
		PostalCode:        "97719",
		ProvinceID:        "32",
		ProvinceName:      "MALUKU UTARA",
		CityID:            "688",
		CityName:          "TERNATE",
		DistrictID:        "7126",
		DistrictName:      "TERNATE SELATAN",
	}}, nil
}

func (legacyHTTPRepositoryStub) ListLegacyHierarchy(
	_ context.Context,
	provider string,
	level string,
	parentID string,
) ([]locations.LegacyRegion, error) {
	if provider != legacyRegionProvider {
		return nil, locations.ErrLocationNotFound
	}
	switch level {
	case "province":
		return []locations.LegacyRegion{{
			ID:                "32",
			CanonicalPublicID: "loc_idn_82",
			Name:              "MALUKU UTARA",
			ProvinceID:        "32",
			ProvinceName:      "MALUKU UTARA",
		}}, nil
	case "city":
		if parentID == "18" {
			return []locations.LegacyRegion{{
				ID:                "256",
				CanonicalPublicID: "loc_legacy_rajaongkir_city_256",
				Name:              "JEMBER",
				ProvinceID:        "18",
				ProvinceName:      "JAWA TIMUR",
				CityID:            "256",
				CityName:          "JEMBER",
			}}, nil
		}
	case "subdistrict":
		if parentID == "7126" {
			return []locations.LegacyRegion{{
				ID:                "82995",
				CanonicalPublicID: "loc_legacy_rajaongkir_subdistrict_82995",
				Name:              "SASA",
				PostalCode:        "97719",
				ProvinceID:        "32",
				CityID:            "688",
				DistrictID:        "7126",
			}}, nil
		}
	}
	return []locations.LegacyRegion{}, nil
}

func (legacyHTTPRepositoryStub) FindLegacyRegion(
	_ context.Context,
	provider string,
	level string,
	identifier string,
) (locations.LegacyRegion, error) {
	if provider == legacyRegionProvider && level == "province" && identifier == "32" {
		return locations.LegacyRegion{
			ID:                "32",
			CanonicalPublicID: "loc_idn_82",
			Name:              "MALUKU UTARA",
			ProvinceID:        "32",
			ProvinceName:      "MALUKU UTARA",
		}, nil
	}
	return locations.LegacyRegion{}, locations.ErrLocationNotFound
}

func (legacyHTTPRepositoryStub) ResolveLegacyPublicID(
	_ context.Context,
	provider string,
	level string,
	identifier string,
) (string, error) {
	if provider != legacyRegionProvider || (level != "district" && level != "subdistrict") {
		return "", locations.ErrLocationNotFound
	}
	return "loc_legacy_rajaongkir_" + level + "_" + identifier, nil
}

func TestLegacyRegionIDNamespaceDoesNotCollideWithKemendagri(t *testing.T) {
	t.Parallel()

	e := echo.New()
	e.GET(
		"/regions/provinces/:id",
		legacyRegionByIDHandler(legacyHTTPRepositoryStub{}, "province"),
	)
	request := httptest.NewRequest(http.MethodGet, "/regions/provinces/32", nil)
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Data struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.ID != 32 || payload.Data.Name != "MALUKU UTARA" {
		t.Fatalf("legacy ID was interpreted as official ID: %#v", payload.Data)
	}
}

func TestLegacySubdistrictResponseKeepsNumericHierarchyIDs(t *testing.T) {
	t.Parallel()

	e := echo.New()
	e.GET(
		"/regions/subdistricts",
		legacyHierarchyListHandler(
			legacyHTTPRepositoryStub{},
			"subdistrict",
			"districtId",
		),
	)
	request := httptest.NewRequest(
		http.MethodGet,
		"/regions/subdistricts?districtId=7126",
		nil,
	)
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Data []struct {
			ID         int64  `json:"id"`
			DistrictID int64  `json:"districtId"`
			CityID     int64  `json:"cityId"`
			ProvinceID int64  `json:"provinceId"`
			PostalCode string `json:"zipCode"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 1 ||
		payload.Data[0].ID != 82995 ||
		payload.Data[0].DistrictID != 7126 ||
		payload.Data[0].CityID != 688 ||
		payload.Data[0].ProvinceID != 32 ||
		payload.Data[0].PostalCode != "97719" {
		t.Fatalf("unexpected legacy hierarchy: %#v", payload.Data)
	}
}

func TestLegacyDomesticCostTranslatesIDsAndReturnsServiceName(t *testing.T) {
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
			PricingModel:           "flat",
			BasePrice:              55000,
			WeightIncrementGrams:   1000,
			RoundingMode:           "ceil",
			RoundingIncrementGrams: 1000,
		}},
	}, time.Second)

	e := echo.New()
	e.GET(
		"/shipping/domestic-cost",
		legacyDomesticCostHandler(service, legacyHTTPRepositoryStub{}),
	)
	request := httptest.NewRequest(
		http.MethodGet,
		"/shipping/domestic-cost?origin=7126&destination=6000&weight=10000&courier=jne",
		nil,
	)
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Data []struct {
			Code        string `json:"code"`
			Service     string `json:"service"`
			ServiceName string `json:"serviceName"`
			Cost        int64  `json:"cost"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 1 ||
		payload.Data[0].Code != "jne" ||
		payload.Data[0].Service != "JTR>130" ||
		payload.Data[0].ServiceName != "TRUCKING" ||
		payload.Data[0].Cost != 55000 {
		t.Fatalf("unexpected legacy rate response: %#v", payload.Data)
	}
}
