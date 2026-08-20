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

func trackingSubscriptionCreateHandler(service *tracking.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if service == nil {
			return writeError(c, http.StatusServiceUnavailable, "TRACKING_NOT_CONFIGURED", "Tracking belum dikonfigurasi.", nil)
		}
		body := http.MaxBytesReader(c.Response(), c.Request().Body, maxCalculateBodyBytes)
		decoder := json.NewDecoder(body)
		decoder.DisallowUnknownFields()
		var request tracking.SubscriptionRequest
		if err := decoder.Decode(&request); err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "Payload subscription tracking tidak valid.", nil)
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "Payload hanya boleh berisi satu objek JSON.", nil)
		}
		subscription, err := service.Subscribe(c.Request().Context(), request)
		switch {
		case errors.Is(err, tracking.ErrInvalidWaybill), errors.Is(err, tracking.ErrInvalidPhoneSuffix):
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "Order, fulfillment, courier, atau AWB tidak valid.", nil)
		case errors.Is(err, tracking.ErrUnsupportedCourier):
			return writeError(c, http.StatusUnprocessableEntity, "TRACKING_COURIER_UNSUPPORTED", "Courier belum didukung.", nil)
		case err != nil:
			return err
		}
		status := http.StatusOK
		if subscription.Shipment.ProviderFetchedAt == nil || subscription.Shipment.RefreshQueued {
			status = http.StatusAccepted
		}
		return c.JSON(status, map[string]any{
			"meta": map[string]any{
				"message":    "Tracking subscription registered",
				"code":       status,
				"status":     "success",
				"request_id": requestID(c),
			},
			"data": subscription,
		})
	}
}

func trackingSubscriptionGetHandler(service *tracking.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if service == nil {
			return writeError(c, http.StatusServiceUnavailable, "TRACKING_NOT_CONFIGURED", "Tracking belum dikonfigurasi.", nil)
		}
		fulfillmentID := strings.TrimSpace(c.Param("fulfillment_id"))
		subscription, err := service.Subscription(c.Request().Context(), fulfillmentID)
		if errors.Is(err, tracking.ErrNotFound) {
			return writeError(c, http.StatusNotFound, "TRACKING_SUBSCRIPTION_NOT_FOUND", "Subscription tracking tidak ditemukan.", nil)
		}
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, map[string]any{
			"meta": map[string]any{
				"message":    "Success Get Tracking Subscription",
				"code":       http.StatusOK,
				"status":     "success",
				"request_id": requestID(c),
			},
			"data": subscription,
		})
	}
}
