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
			status := http.StatusBadRequest
			if rajaOngkirV2Compatibility(c) {
				status = http.StatusUnprocessableEntity
			}
			return writeError(
				c,
				status,
				"INVALID_REQUEST",
				"Parameter search minimal 2 karakter.",
				nil,
			)
		}

		limit := 20
		maxLimit := 50
		if rajaOngkirV2Compatibility(c) {
			maxLimit = 1000
		}
		if rawLimit := strings.TrimSpace(c.QueryParam("limit")); rawLimit != "" {
			parsed, err := strconv.Atoi(rawLimit)
			if err != nil || parsed < 1 || parsed > maxLimit {
				status := http.StatusBadRequest
				if rajaOngkirV2Compatibility(c) {
					status = http.StatusUnprocessableEntity
				}
				return writeError(
					c,
					status,
					"INVALID_REQUEST",
					"Parameter limit berada di luar rentang yang didukung.",
					nil,
				)
			}
			limit = parsed
		}
		offset := 0
		if rawOffset := strings.TrimSpace(c.QueryParam("offset")); rawOffset != "" {
			parsed, err := strconv.Atoi(rawOffset)
			if err != nil || parsed < 0 || parsed > 100_000 {
				status := http.StatusBadRequest
				if rajaOngkirV2Compatibility(c) {
					status = http.StatusUnprocessableEntity
				}
				return writeError(
					c,
					status,
					"INVALID_REQUEST",
					"Parameter offset harus antara 0 dan 100.000.",
					nil,
				)
			}
			offset = parsed
		}

		ctx, cancel := timeBoundContext(c.Request().Context(), 500*time.Millisecond)
		defer cancel()
		result, err := repository.Search(ctx, search, limit, offset)
		if err != nil {
			return err
		}
		if rajaOngkirV2Compatibility(c) && len(result) == 0 {
			return writeError(
				c,
				http.StatusNotFound,
				"NOT_FOUND",
				"Domestic Destinations Data not found",
				nil,
			)
		}

		data := make([]map[string]any, 0, len(result))
		for _, location := range result {
			if rajaOngkirV2Compatibility(c) {
				data = append(data, map[string]any{
					"id":               location.CompatibilityID,
					"label":            location.Label(),
					"province_name":    location.Province,
					"city_name":        location.City,
					"district_name":    location.District,
					"subdistrict_name": location.Subdistrict,
					"zip_code":         location.PostalCode,
				})
				continue
			}
			data = append(data, map[string]any{
				"id":               location.PublicID,
				"label":            location.Label(),
				"province_id":      location.ProvinceID,
				"city_id":          location.CityID,
				"district_id":      location.DistrictID,
				"subdistrict_id":   location.SubdistrictID,
				"province_name":    location.Province,
				"city_name":        location.City,
				"district_name":    location.District,
				"subdistrict_name": location.Subdistrict,
				"zip_code":         location.PostalCode,
				"province":         location.Province,
				"city":             location.City,
				"district":         location.District,
				"subdistrict":      location.Subdistrict,
				"postal_code":      location.PostalCode,
				"postal_codes":     location.PostalCodes,
			})
		}
		if rajaOngkirV2Compatibility(c) {
			return c.JSON(http.StatusOK, map[string]any{
				"meta": map[string]any{
					"message": "Success Get Domestic Destinations",
					"code":    http.StatusOK,
					"status":  "success",
				},
				"data": data,
			})
		}
		return c.JSON(http.StatusOK, map[string]any{
			"meta": map[string]any{
				"message":     "Success Get Domestic Destinations",
				"code":        http.StatusOK,
				"status":      "success",
				"request_id":  requestID(c),
				"next_cursor": nil,
			},
			"data": data,
		})
	}
}

func locationHierarchyHandler(
	repository locations.Repository,
	level string,
	parentParameter string,
	successMessage string,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		parentPublicID := ""
		if parentParameter != "" {
			parentPublicID = strings.TrimSpace(c.Param(parentParameter))
			if parentPublicID == "" {
				return writeError(
					c,
					http.StatusBadRequest,
					"INVALID_REQUEST",
					"ID parent lokasi wajib diisi.",
					nil,
				)
			}
		}

		ctx, cancel := timeBoundContext(c.Request().Context(), time.Second)
		defer cancel()
		result, err := repository.ListHierarchy(ctx, level, parentPublicID)
		if err != nil {
			return err
		}

		data := make([]map[string]any, 0, len(result))
		for _, location := range result {
			id := any(location.PublicID)
			if rajaOngkirV2Compatibility(c) {
				id = location.CompatibilityID
			}
			item := map[string]any{
				"id":   id,
				"name": location.Name,
			}
			if level != "province" {
				item["zip_code"] = location.PostalCode
			}
			data = append(data, item)
		}

		if rajaOngkirV2Compatibility(c) {
			return c.JSON(http.StatusOK, map[string]any{
				"meta": map[string]any{
					"message": successMessage,
					"code":    http.StatusOK,
					"status":  "success",
				},
				"data": data,
			})
		}
		return c.JSON(http.StatusOK, map[string]any{
			"meta": map[string]any{
				"message":    successMessage,
				"code":       http.StatusOK,
				"status":     "success",
				"request_id": requestID(c),
			},
			"data": data,
		})
	}
}
