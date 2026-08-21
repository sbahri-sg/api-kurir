package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
)

func TestAdminTrackingOperationDeleteRejectsInvalidID(t *testing.T) {
	t.Parallel()

	e := echo.New()
	e.DELETE("/v1/admin/tracking-operations/:id", adminTrackingOperationDeleteHandler(nil))
	request := httptest.NewRequest(http.MethodDelete, "/v1/admin/tracking-operations/not-a-uuid", nil)
	recorder := httptest.NewRecorder()

	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", recorder.Code)
	}
}

func TestNormalizeRateCardInput(t *testing.T) {
	t.Parallel()

	result, err := normalizeRateCardInput(createRateCardRequest{
		OriginPublicID:       "loc_origin",
		DestinationPublicID:  "loc_destination",
		CourierCode:          "JNE",
		ServiceCode:          "JTR",
		PricingModel:         "minimum_then_per_kg",
		BaseWeightGrams:      10_000,
		BasePrice:            40_000,
		RatePerIncrement:     4_000,
		MinimumWeightGrams:   10_000,
		WeightIncrementGrams: 1_000,
		RoundingProfileCode:  "jne-jtr-public-2026",
		VerificationStatus:   "official_contract",
		SourceProvider:       "JNE",
		EffectiveFrom:        "2026-07-28T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.CourierCode != "jne" ||
		result.SourceProvider != "jne" ||
		!result.EffectiveFrom.Equal(time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected normalized input: %#v", result)
	}
}

func TestNormalizeRateCardRejectsInvalidMaximum(t *testing.T) {
	t.Parallel()

	maximum := int64(5_000)
	_, err := normalizeRateCardInput(createRateCardRequest{
		OriginPublicID:       "loc_origin",
		DestinationPublicID:  "loc_destination",
		CourierCode:          "jne",
		ServiceCode:          "JTR",
		PricingModel:         "per_kg",
		MinimumWeightGrams:   10_000,
		MaximumWeightGrams:   &maximum,
		WeightIncrementGrams: 1_000,
		RoundingProfileCode:  "profile",
		VerificationStatus:   "observed",
		SourceProvider:       "manual",
	})
	if err == nil {
		t.Fatal("expected invalid maximum weight error")
	}
}
