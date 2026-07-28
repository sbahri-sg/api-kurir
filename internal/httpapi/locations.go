package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/emisell/api-kurir/internal/locations"
	"github.com/labstack/echo/v5"
)

func locationSearchHandler(repository locations.Repository) echo.HandlerFunc {
	return func(c *echo.Context) error {
		search := strings.TrimSpace(c.QueryParam("search"))
		if len(search) < 2 {
			return writeError(
				c,
				http.StatusBadRequest,
				"INVALID_REQUEST",
				"Parameter search minimal 2 karakter.",
				nil,
			)
		}

		limit := 20
		if rawLimit := strings.TrimSpace(c.QueryParam("limit")); rawLimit != "" {
			parsed, err := strconv.Atoi(rawLimit)
			if err != nil || parsed < 1 || parsed > 50 {
				return writeError(
					c,
					http.StatusBadRequest,
					"INVALID_REQUEST",
					"Parameter limit harus antara 1 dan 50.",
					nil,
				)
			}
			limit = parsed
		}

		ctx, cancel := timeBoundContext(c.Request().Context(), 500*time.Millisecond)
		defer cancel()
		result, err := repository.Search(ctx, search, limit)
		if err != nil {
			return err
		}

		data := make([]map[string]any, 0, len(result))
		for _, location := range result {
			data = append(data, map[string]any{
				"id":          location.PublicID,
				"label":       location.Label(),
				"province":    location.Province,
				"city":        location.City,
				"district":    location.District,
				"subdistrict": location.Subdistrict,
				"postal_code": location.PostalCode,
				"postal_codes": location.PostalCodes,
			})
		}
		return c.JSON(http.StatusOK, map[string]any{
			"meta": map[string]any{
				"request_id":  requestID(c),
				"next_cursor": nil,
			},
			"data": data,
		})
	}
}
