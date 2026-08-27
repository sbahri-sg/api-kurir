package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/emisell/api-kurir/internal/fulfillment"
	"github.com/labstack/echo/v5"
)

func fulfillmentCreateHandler(service *fulfillment.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if service == nil {
			return fulfillmentNotConfigured(c)
		}
		var request fulfillment.CreateRequest
		if err := decodeFulfillmentJSON(c, &request); err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "Payload shipment tidak valid.", nil)
		}
		shipment, replayed, err := service.Create(
			c.Request().Context(), c.Request().Header.Get("Idempotency-Key"), request,
		)
		if err != nil {
			return writeFulfillmentError(c, err)
		}
		status := http.StatusCreated
		if replayed {
			status = http.StatusOK
		}
		return c.JSON(status, fulfillmentResponse(c, status, "Shipment created", map[string]any{
			"shipment": shipment, "idempotent_replay": replayed,
		}))
	}
}

func fulfillmentGetHandler(service *fulfillment.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if service == nil {
			return fulfillmentNotConfigured(c)
		}
		shipment, err := service.Get(c.Request().Context(), c.Param("shipment_id"))
		if err != nil {
			return writeFulfillmentError(c, err)
		}
		return c.JSON(http.StatusOK, fulfillmentResponse(c, http.StatusOK, "Success Get Shipment", shipment))
	}
}

func fulfillmentPickupHandler(service *fulfillment.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if service == nil {
			return fulfillmentNotConfigured(c)
		}
		var request fulfillment.PickupRequest
		if err := decodeFulfillmentJSON(c, &request); err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "Payload pickup tidak valid.", nil)
		}
		shipment, replayed, err := service.Pickup(
			c.Request().Context(), c.Param("shipment_id"),
			c.Request().Header.Get("Idempotency-Key"), request,
		)
		if err != nil {
			return writeFulfillmentError(c, err)
		}
		status := http.StatusAccepted
		if replayed {
			status = http.StatusOK
		}
		return c.JSON(status, fulfillmentResponse(c, status, "Pickup requested", map[string]any{
			"shipment": shipment, "idempotent_replay": replayed,
		}))
	}
}

func fulfillmentCancelHandler(service *fulfillment.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if service == nil {
			return fulfillmentNotConfigured(c)
		}
		var request fulfillment.CancelRequest
		if err := decodeFulfillmentJSON(c, &request); err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "Payload pembatalan tidak valid.", nil)
		}
		shipment, replayed, err := service.Cancel(
			c.Request().Context(), c.Param("shipment_id"),
			c.Request().Header.Get("Idempotency-Key"), request,
		)
		if err != nil {
			return writeFulfillmentError(c, err)
		}
		status := http.StatusAccepted
		if replayed {
			status = http.StatusOK
		}
		return c.JSON(status, fulfillmentResponse(c, status, "Cancellation requested", map[string]any{
			"shipment": shipment, "idempotent_replay": replayed,
		}))
	}
}

func fulfillmentLabelHandler(service *fulfillment.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if service == nil {
			return fulfillmentNotConfigured(c)
		}
		label, err := service.Label(
			c.Request().Context(), c.Param("shipment_id"), c.QueryParam("format"),
		)
		if err != nil {
			return writeFulfillmentError(c, err)
		}
		return c.JSON(http.StatusOK, fulfillmentResponse(c, http.StatusOK, "Success Get Shipment Label", label))
	}
}

func fulfillmentHistoryHandler(service *fulfillment.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if service == nil {
			return fulfillmentNotConfigured(c)
		}
		history, err := service.History(c.Request().Context(), c.Param("shipment_id"))
		if err != nil {
			return writeFulfillmentError(c, err)
		}
		return c.JSON(http.StatusOK, fulfillmentResponse(c, http.StatusOK, "Success Get Shipment History", history))
	}
}

func decodeFulfillmentJSON(c *echo.Context, target any) error {
	body := http.MaxBytesReader(c.Response(), c.Request().Body, maxCalculateBodyBytes)
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("payload must contain one JSON object")
	}
	return nil
}

func writeFulfillmentError(c *echo.Context, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, fulfillment.ErrInvalidRequest):
		return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "Data shipment tidak lengkap atau tidak valid.", nil)
	case errors.Is(err, fulfillment.ErrInvalidIdempotency):
		return writeError(c, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key wajib 8-128 karakter.", nil)
	case errors.Is(err, fulfillment.ErrIdempotencyConflict):
		return writeError(c, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "Idempotency-Key atau merchant_reference sudah digunakan untuk payload lain.", nil)
	case errors.Is(err, fulfillment.ErrOperationInProgress):
		return writeError(c, http.StatusConflict, "OPERATION_IN_PROGRESS", "Operasi dengan key tersebut masih diproses; jangan membuat booking baru.", map[string]any{"retryable": true})
	case errors.Is(err, fulfillment.ErrShipmentNotFound):
		return writeError(c, http.StatusNotFound, "SHIPMENT_NOT_FOUND", "Shipment tidak ditemukan pada merchant ini.", nil)
	case errors.Is(err, fulfillment.ErrShippingDisabled):
		return writeError(c, http.StatusUnprocessableEntity, "SHIPPING_PROVIDER_INACTIVE", "Merchant belum mengaktifkan provider pengiriman.", nil)
	case errors.Is(err, fulfillment.ErrProviderUnsupported):
		return writeError(c, http.StatusUnprocessableEntity, "FULFILLMENT_PROVIDER_UNSUPPORTED", "Provider aktif belum mendukung fulfillment melalui API Kurir.", nil)
	case errors.Is(err, fulfillment.ErrCredentialUnavailable):
		return writeError(c, http.StatusUnprocessableEntity, "DELIVERY_CREDENTIAL_REQUIRED", "Credential provider belum memiliki akses Shipping Delivery.", nil)
	case errors.Is(err, fulfillment.ErrProviderUnauthorized):
		return writeError(c, http.StatusBadGateway, "PROVIDER_UNAUTHORIZED", "Credential Shipping Delivery ditolak provider.", nil)
	case errors.Is(err, fulfillment.ErrProviderRejected):
		return writeError(c, http.StatusUnprocessableEntity, "PROVIDER_REJECTED", "Provider menolak data atau status shipment tidak memenuhi syarat.", nil)
	case errors.Is(err, fulfillment.ErrProviderTimeout):
		return writeError(c, http.StatusGatewayTimeout, "PROVIDER_TIMEOUT", "Provider tidak merespons tepat waktu; periksa status sebelum mencoba ulang.", map[string]any{"retryable": false})
	case errors.Is(err, fulfillment.ErrProviderUnavailable):
		return writeError(c, http.StatusServiceUnavailable, "PROVIDER_UNAVAILABLE", "Provider pengiriman sedang tidak tersedia.", map[string]any{"retryable": true})
	case errors.Is(err, fulfillment.ErrPickupNotAllowed):
		return writeError(c, http.StatusConflict, "PICKUP_NOT_ALLOWED", "Pickup hanya dapat dijadwalkan setelah booking provider berhasil.", nil)
	case errors.Is(err, fulfillment.ErrShipmentFinal):
		return writeError(c, http.StatusConflict, "SHIPMENT_FINAL", "Shipment berstatus final dan tidak dapat diubah.", nil)
	case errors.Is(err, fulfillment.ErrLabelUnavailable):
		return writeError(c, http.StatusUnprocessableEntity, "LABEL_UNAVAILABLE", "Label belum tersedia atau shipment belum dijadwalkan pickup.", nil)
	default:
		return err
	}
}

func fulfillmentNotConfigured(c *echo.Context) error {
	return writeError(c, http.StatusServiceUnavailable, "FULFILLMENT_NOT_CONFIGURED", "Fulfillment belum dikonfigurasi.", nil)
}

func fulfillmentResponse(c *echo.Context, status int, message string, data any) map[string]any {
	return map[string]any{
		"meta": map[string]any{
			"message": message, "code": status,
			"status": "success", "request_id": requestID(c),
		},
		"data": data,
	}
}
