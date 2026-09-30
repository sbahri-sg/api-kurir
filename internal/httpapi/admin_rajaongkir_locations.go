package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/emisell/api-kurir/internal/locations"
	"github.com/labstack/echo/v5"
)

// adminRajaOngkirLocationSearchHandler returns only locations with an active
// RajaOngkir mapping. The canonical ID can be passed directly to the rate API.
func adminRajaOngkirLocationSearchHandler(repository locations.LegacyRepository) echo.HandlerFunc {
	return func(c *echo.Context) error {
		search := strings.TrimSpace(c.QueryParam("search"))
		if len(search) < 2 {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "Parameter search minimal 2 karakter.", nil)
		}

		limit := 20
		if raw := strings.TrimSpace(c.QueryParam("limit")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 1 || parsed > 50 {
				return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "Parameter limit berada di luar rentang yang didukung.", nil)
			}
			limit = parsed
		}

		ctx, cancel := timeBoundContext(c.Request().Context(), 2*time.Second)
		defer cancel()
		result, err := repository.SearchLegacy(ctx, legacyRegionProvider, search, limit, 0)
		if err != nil {
			return err
		}

		data := make([]map[string]any, 0, len(result))
		for _, location := range result {
			data = append(data, map[string]any{
				"id":               location.CanonicalPublicID,
				"label":            legacyLocationLabel(location),
				"province_id":      location.ProvinceID,
				"city_id":          location.CityID,
				"district_id":      location.DistrictID,
				"subdistrict_id":   location.ID,
				"province_name":    location.ProvinceName,
				"city_name":        location.CityName,
				"district_name":    location.DistrictName,
				"subdistrict_name": location.Name,
				"zip_code":         location.PostalCode,
				"province":         location.ProvinceName,
				"city":             location.CityName,
				"district":         location.DistrictName,
				"subdistrict":      location.Name,
				"postal_code":      location.PostalCode,
				"postal_codes":     []string{location.PostalCode},
			})
		}
		return c.JSON(http.StatusOK, map[string]any{"data": data})
	}
}
