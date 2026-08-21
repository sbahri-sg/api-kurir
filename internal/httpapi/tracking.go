package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/emisell/api-kurir/internal/tracking"
	"github.com/labstack/echo/v5"
)

type trackingRequest struct {
	Waybill         string `json:"waybill"`
	Courier         string `json:"courier"`
	LastPhoneNumber string `json:"last_phone_number,omitempty"`
	Refresh         string `json:"refresh,omitempty"`
}

func trackingVerifyHandler(
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
		var request tracking.VerificationRequest
		if err := decoder.Decode(&request); err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "Payload verifikasi resi tidak valid.", nil)
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "Payload hanya boleh berisi satu objek JSON.", nil)
		}
		result, err := service.Verify(c.Request().Context(), immediateAdapter, request)
		switch {
		case errors.Is(err, tracking.ErrInvalidWaybill), errors.Is(err, tracking.ErrInvalidPhoneSuffix):
			return c.JSON(http.StatusBadRequest, map[string]any{"meta": map[string]any{"request_id": requestID(c)}, "data": result})
		case errors.Is(err, tracking.ErrUnsupportedCourier):
			return writeError(c, http.StatusUnprocessableEntity, "TRACKING_COURIER_UNSUPPORTED", "Courier belum didukung.", nil)
		case errors.Is(err, tracking.ErrWaybillNotFound):
			// Not found is a verification result, not a transport failure.
			return c.JSON(http.StatusOK, map[string]any{"meta": map[string]any{"request_id": requestID(c)}, "data": result})
		case err != nil:
			return c.JSON(http.StatusServiceUnavailable, map[string]any{"meta": map[string]any{"request_id": requestID(c)}, "data": result})
		default:
			return c.JSON(http.StatusOK, map[string]any{"meta": map[string]any{"request_id": requestID(c)}, "data": result})
		}
	}
}

func trackingPublicHandler(
	service *tracking.Service,
	immediateAdapter tracking.Adapter,
) echo.HandlerFunc {
	legacyHandler := trackingHandler(service)
	return func(c *echo.Context) error {
		if !rajaOngkirV2Compatibility(c) {
			return legacyHandler(c)
		}
		return trackingRajaOngkirV2Handler(service, immediateAdapter)(c)
	}
}

func trackingRajaOngkirV2Handler(
	service *tracking.Service,
	immediateAdapter tracking.Adapter,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if service == nil || immediateAdapter == nil {
			return writeError(
				c,
				http.StatusServiceUnavailable,
				"TRACKING_NOT_CONFIGURED",
				"Tracking provider belum dikonfigurasi.",
				nil,
			)
		}

		c.Request().Body = http.MaxBytesReader(
			c.Response(),
			c.Request().Body,
			maxCalculateBodyBytes,
		)
		if err := c.Request().ParseForm(); err != nil {
			return writeError(
				c,
				http.StatusBadRequest,
				"INVALID_REQUEST",
				"Payload tracking tidak valid.",
				nil,
			)
		}

		request := tracking.Request{
			CourierCode:     c.Request().FormValue("courier"),
			Waybill:         c.Request().FormValue("awb"),
			LastPhoneDigits: c.Request().FormValue("last_phone_number"),
		}
		result, err := service.TrackNow(
			c.Request().Context(),
			immediateAdapter,
			request.CourierCode,
			request.Waybill,
			request.LastPhoneDigits,
		)
		switch {
		case errors.Is(err, tracking.ErrInvalidWaybill),
			errors.Is(err, tracking.ErrInvalidPhoneSuffix):
			return writeError(
				c,
				http.StatusBadRequest,
				"INVALID_REQUEST",
				"AWB, courier, atau last_phone_number tidak valid.",
				nil,
			)
		case errors.Is(err, tracking.ErrUnsupportedCourier):
			return writeError(
				c,
				http.StatusUnprocessableEntity,
				"INVALID_COURIER",
				"Courier tidak mendukung tracking.",
				nil,
			)
		case errors.Is(err, tracking.ErrWaybillNotFound):
			return writeError(
				c,
				http.StatusNotFound,
				"WAYBILL_NOT_FOUND",
				"Checking AWB not found.",
				nil,
			)
		case errors.Is(err, tracking.ErrPhoneSuffixRequired):
			return writeError(
				c,
				http.StatusBadRequest,
				"PHONE_VALIDATION_REQUIRED",
				"Provider membutuhkan 5 digit terakhir nomor telepon penerima.",
				nil,
			)
		case errors.Is(err, tracking.ErrProviderQuota),
			errors.Is(err, tracking.ErrProviderRateLimited):
			return writeError(
				c,
				http.StatusServiceUnavailable,
				"PROVIDER_QUOTA_EXHAUSTED",
				"Provider tracking sedang dibatasi.",
				nil,
			)
		case errors.Is(err, tracking.ErrProviderUnauthorized):
			return writeError(
				c,
				http.StatusBadGateway,
				"PROVIDER_AUTHENTICATION_FAILED",
				"Autentikasi provider tracking gagal.",
				nil,
			)
		case errors.Is(err, tracking.ErrProviderTimeout):
			return writeError(
				c,
				http.StatusGatewayTimeout,
				"PROVIDER_TIMEOUT",
				"Provider tracking tidak merespons tepat waktu.",
				nil,
			)
		case errors.Is(err, tracking.ErrAdapterUnavailable),
			errors.Is(err, tracking.ErrProviderUnavailable):
			return writeError(
				c,
				http.StatusBadGateway,
				"PROVIDER_ERROR",
				"Provider tracking belum dapat memberikan data.",
				nil,
			)
		case err != nil:
			return err
		}

		return c.JSON(http.StatusOK, map[string]any{
			"meta": map[string]any{
				"message": "Success Tracking AWB",
				"code":    http.StatusOK,
				"status":  "success",
			},
			"data": rajaOngkirTrackingData(request, result),
		})
	}
}

func rajaOngkirTrackingData(
	request tracking.Request,
	result tracking.Result,
) map[string]any {
	summary := result.Summary
	waybill := strings.ToUpper(strings.TrimSpace(request.Waybill))
	courierCode := firstString(
		trackingSummaryString(summary, "courier_code"),
		strings.ToLower(strings.TrimSpace(request.CourierCode)),
	)
	status := firstString(
		trackingSummaryString(summary, "status"),
		result.StatusLabel,
		result.NormalizedStatus,
	)
	waybillDate := trackingSummaryString(summary, "waybill_date")
	shipperName := trackingSummaryString(summary, "shipper_name")
	receiverName := trackingSummaryString(summary, "receiver_name")
	origin := trackingSummaryString(summary, "origin")
	destination := trackingSummaryString(summary, "destination")

	manifest := make([]map[string]any, 0, len(result.Events))
	jakarta := time.FixedZone("Asia/Jakarta", 7*60*60)
	for _, event := range result.Events {
		occurredAt := event.OccurredAt.In(jakarta)
		manifest = append(manifest, map[string]any{
			"manifest_code":        event.Code,
			"manifest_description": event.Description,
			"manifest_date":        occurredAt.Format("2006-01-02"),
			"manifest_time":        occurredAt.Format("15:04:05"),
			"city_name":            event.Location,
		})
	}

	delivered := result.NormalizedStatus == "delivered"
	if value, ok := summary["delivered"].(bool); ok {
		delivered = value
	}
	return map[string]any{
		"delivered": delivered,
		"summary": map[string]any{
			"courier_code":   courierCode,
			"courier_name":   trackingSummaryString(summary, "courier_name"),
			"waybill_number": waybill,
			"service_code":   trackingSummaryString(summary, "service_code"),
			"waybill_date":   waybillDate,
			"shipper_name":   shipperName,
			"receiver_name":  receiverName,
			"origin":         origin,
			"destination":    destination,
			"status":         status,
		},
		"details": map[string]any{
			"waybill_number":    waybill,
			"waybill_date":      waybillDate,
			"waybill_time":      trackingSummaryString(summary, "waybill_time"),
			"weight":            trackingSummaryString(summary, "weight"),
			"origin":            origin,
			"destination":       destination,
			"shipper_name":      shipperName,
			"shipper_address1":  trackingSummaryString(summary, "shipper_address1"),
			"shipper_address2":  trackingSummaryString(summary, "shipper_address2"),
			"shipper_address3":  trackingSummaryString(summary, "shipper_address3"),
			"shipper_city":      trackingSummaryString(summary, "shipper_city"),
			"receiver_name":     receiverName,
			"receiver_address1": trackingSummaryString(summary, "receiver_address1"),
			"receiver_address2": trackingSummaryString(summary, "receiver_address2"),
			"receiver_address3": trackingSummaryString(summary, "receiver_address3"),
			"receiver_city":     trackingSummaryString(summary, "receiver_city"),
		},
		"delivery_status": map[string]any{
			"status":       firstString(trackingSummaryString(summary, "delivery_status"), status),
			"pod_receiver": trackingSummaryString(summary, "pod_receiver"),
			"pod_date":     trackingSummaryString(summary, "pod_date"),
			"pod_time":     trackingSummaryString(summary, "pod_time"),
		},
		"manifest": manifest,
	}
}

func trackingSummaryString(summary map[string]any, key string) string {
	value := summary[key]
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func firstString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
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
