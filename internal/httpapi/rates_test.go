package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
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
