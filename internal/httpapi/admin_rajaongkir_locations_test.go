package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestAdminRajaOngkirLocationSearchReturnsMappedCanonicalID(t *testing.T) {
	t.Parallel()

	e := echo.New()
	e.GET("/v1/admin/locations/rajaongkir", adminRajaOngkirLocationSearchHandler(legacyHTTPRepositoryStub{}))
	request := httptest.NewRequest(http.MethodGet, "/v1/admin/locations/rajaongkir?search=Sasa&limit=12", nil)
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status: got %d, body=%s", response.Code, response.Body.String())
	}

	var payload struct {
		Data []struct {
			ID         string `json:"id"`
			PostalCode string `json:"postal_code"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 1 || payload.Data[0].ID != "loc_legacy_rajaongkir_subdistrict_82995" || payload.Data[0].PostalCode != "97719" {
		t.Fatalf("unexpected locations: %#v", payload.Data)
	}
}
