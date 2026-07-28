package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/emisell/api-kurir/internal/tracking"
	"github.com/labstack/echo/v5"
)

type trackingRequest struct {
	Waybill         string `json:"waybill"`
	Courier         string `json:"courier"`
	LastPhoneNumber string `json:"last_phone_number,omitempty"`
	Refresh         string `json:"refresh,omitempty"`
}

func trackingHandler(service *tracking.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if service == nil {
			return writeError(
				c,
				http.StatusServiceUnavailable,
				"TRACKING_NOT_CONFIGURED",
				"Fondasi tracking tersedia tetapi belum diaktifkan.",
				nil,
			)
		}
		var request trackingRequest
		body := http.MaxBytesReader(c.Response(), c.Request().Body, maxCalculateBodyBytes)
		decoder := json.NewDecoder(body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			return writeError(
				c,
				http.StatusBadRequest,
				"INVALID_REQUEST",
				"Payload tracking tidak valid.",
				nil,
			)
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return writeError(
				c,
				http.StatusBadRequest,
				"INVALID_REQUEST",
				"Payload hanya boleh berisi satu objek JSON.",
				nil,
			)
		}
		refresh := strings.ToLower(strings.TrimSpace(request.Refresh))
		if refresh == "" {
			refresh = "if_stale"
		}
		if refresh != "if_stale" {
			return writeError(
				c,
				http.StatusBadRequest,
				"INVALID_REQUEST",
				"Fase ini hanya mendukung refresh=if_stale.",
				nil,
			)
		}
		result, err := service.Register(
			c.Request().Context(),
			request.Courier,
			request.Waybill,
			request.LastPhoneNumber,
		)
		if errors.Is(err, tracking.ErrInvalidWaybill) ||
			errors.Is(err, tracking.ErrInvalidPhoneSuffix) {
			return writeError(
				c,
				http.StatusBadRequest,
				"INVALID_REQUEST",
				"Courier atau nomor resi tidak valid.",
				nil,
			)
		}
		if errors.Is(err, tracking.ErrUnsupportedCourier) {
			return writeError(
				c,
				http.StatusUnprocessableEntity,
				"TRACKING_COURIER_UNSUPPORTED",
				"Courier belum didukung oleh adapter tracking aktif.",
				nil,
			)
		}
		if err != nil {
			return err
		}
		status := http.StatusOK
		if result.ProviderFetchedAt == nil || result.RefreshQueued {
			status = http.StatusAccepted
		}
		return c.JSON(status, map[string]any{
			"meta": map[string]any{"request_id": requestID(c)},
			"data": result,
		})
	}
}
