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
		case errors.Is(err, tracking.ErrRevisionConflict):
			return writeError(c, http.StatusConflict, "TRACKING_REVISION_CONFLICT", "AWB fulfillment sudah berubah. Gunakan endpoint penggantian dengan expected_revision.", nil)
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

func trackingSubscriptionReplaceHandler(
	service *tracking.Service,
	immediateAdapter tracking.Adapter,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if service == nil || immediateAdapter == nil {
			return writeError(c, http.StatusServiceUnavailable, "TRACKING_NOT_CONFIGURED", "Tracking belum dikonfigurasi.", nil)
		}
		body := http.MaxBytesReader(c.Response(), c.Request().Body, maxCalculateBodyBytes)
		decoder := json.NewDecoder(body)
		decoder.DisallowUnknownFields()
		var request tracking.SubscriptionRequest
		if err := decoder.Decode(&request); err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "Payload penggantian resi tidak valid.", nil)
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "Payload hanya boleh berisi satu objek JSON.", nil)
		}
		request.FulfillmentReference = strings.TrimSpace(c.Param("fulfillment_id"))
		subscription, verification, err := service.ReplaceSubscription(c.Request().Context(), immediateAdapter, request)
		switch {
		case errors.Is(err, tracking.ErrNotFound):
			return writeError(c, http.StatusNotFound, "TRACKING_SUBSCRIPTION_NOT_FOUND", "Subscription tracking tidak ditemukan.", nil)
		case errors.Is(err, tracking.ErrRevisionConflict):
			return writeError(c, http.StatusConflict, "TRACKING_REVISION_CONFLICT", "Data resi sudah diperbarui oleh proses lain. Muat ulang revision terbaru.", nil)
		case errors.Is(err, tracking.ErrFinalShipmentLocked):
			return writeError(c, http.StatusConflict, "TRACKING_FINAL_LOCKED", "Resi dengan status final tidak dapat diganti tanpa proses admin.", nil)
		case errors.Is(err, tracking.ErrCourierMismatch):
			return c.JSON(http.StatusUnprocessableEntity, map[string]any{"meta": map[string]any{"request_id": requestID(c)}, "data": verification})
		case errors.Is(err, tracking.ErrInvalidWaybill), errors.Is(err, tracking.ErrInvalidPhoneSuffix):
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "Order, courier, atau AWB tidak valid.", nil)
		case errors.Is(err, tracking.ErrWaybillNotFound):
			return c.JSON(http.StatusUnprocessableEntity, map[string]any{"meta": map[string]any{"request_id": requestID(c)}, "data": verification})
		case err != nil:
			return err
		}
		return c.JSON(http.StatusOK, map[string]any{
			"meta": map[string]any{"message": "Tracking subscription replaced", "code": http.StatusOK, "status": "success", "request_id": requestID(c)},
			"data": map[string]any{"subscription": subscription, "verification": verification},
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

func trackingSubscriptionDeleteHandler(service *tracking.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if service == nil {
			return writeError(c, http.StatusServiceUnavailable, "TRACKING_NOT_CONFIGURED", "Tracking belum dikonfigurasi.", nil)
		}
		fulfillmentID := strings.TrimSpace(c.Param("fulfillment_id"))
		removal, err := service.RemoveSubscription(c.Request().Context(), fulfillmentID)
		if errors.Is(err, tracking.ErrNotFound) {
			return writeError(c, http.StatusNotFound, "TRACKING_SUBSCRIPTION_NOT_FOUND", "Subscription tracking tidak ditemukan pada merchant.", nil)
		}
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, map[string]any{
			"meta": map[string]any{
				"message":    "Tracking subscription removed",
				"code":       http.StatusOK,
				"status":     "success",
				"request_id": requestID(c),
			},
			"data": removal,
		})
	}
}
