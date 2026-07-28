package rajaongkir

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/emisell/api-kurir/internal/rates"
	"github.com/emisell/api-kurir/internal/tracking"
)

var defaultTrackingCouriers = []string{
	"jne",
	"sap",
	"ninja",
	"jnt",
	"tiki",
	"wahana",
	"pos",
	"lion",
}

type ProviderCallRecorder interface {
	RecordProviderAPICall(
		ctx context.Context,
		providerCode string,
		credentialAlias string,
		endpoint string,
		requestFingerprint string,
		httpStatus int,
		outcome string,
		duration time.Duration,
		quotaCost int,
		errorCode string,
	) error
}

type TrackingAdapter struct {
	client          *Client
	quota           rates.QuotaRepository
	recorder        ProviderCallRecorder
	credentialAlias string
	dailyLimit      int64
	courierCodes    []string
	now             func() time.Time
}

func NewTrackingAdapter(
	client *Client,
	quota rates.QuotaRepository,
	recorder ProviderCallRecorder,
	credentialAlias string,
	dailyLimit int64,
	courierCodes []string,
) *TrackingAdapter {
	if len(courierCodes) == 0 {
		courierCodes = defaultTrackingCouriers
	}
	seen := make(map[string]struct{}, len(courierCodes))
	normalized := make([]string, 0, len(courierCodes))
	for _, courierCode := range courierCodes {
		courierCode = strings.ToLower(strings.TrimSpace(courierCode))
		if courierCode == "" {
			continue
		}
		if _, exists := seen[courierCode]; exists {
			continue
		}
		seen[courierCode] = struct{}{}
		normalized = append(normalized, courierCode)
	}
	return &TrackingAdapter{
		client:          client,
		quota:           quota,
		recorder:        recorder,
		credentialAlias: credentialAlias,
		dailyLimit:      dailyLimit,
		courierCodes:    normalized,
		now:             time.Now,
	}
}

func (a *TrackingAdapter) Code() string {
	return "rajaongkir"
}

func (a *TrackingAdapter) CourierCodes() []string {
	return append([]string(nil), a.courierCodes...)
}

func (a *TrackingAdapter) Track(
	ctx context.Context,
	request tracking.Request,
) (tracking.Result, error) {
	if !a.supports(request.CourierCode) {
		return tracking.Result{}, tracking.ErrUnsupportedCourier
	}
	fingerprint := trackingRequestFingerprint(request.CourierCode, request.Waybill)
	if err := a.quota.ConsumeProviderHit(
		ctx,
		a.Code(),
		a.credentialAlias,
		a.dailyLimit,
	); err != nil {
		if errors.Is(err, rates.ErrProviderQuotaExhausted) {
			a.recordCall(ctx, fingerprint, 0, "client_error", 0, 0, "LOCAL_QUOTA_EXHAUSTED")
			return tracking.Result{}, tracking.ErrProviderQuota
		}
		return tracking.Result{}, tracking.ErrProviderUnavailable
	}

	startedAt := a.now()
	providerResult, err := a.client.TrackWaybill(ctx, WaybillRequest{
		AWB:             request.Waybill,
		Courier:         request.CourierCode,
		LastPhoneNumber: request.LastPhoneDigits,
	})
	duration := a.now().Sub(startedAt)
	if err != nil {
		code, outcome := providerCallFailure(err)
		a.recordCall(
			ctx,
			fingerprint,
			providerStatusCode(err),
			outcome,
			duration,
			1,
			code,
		)
		return tracking.Result{}, err
	}
	a.recordCall(ctx, fingerprint, 200, "success", duration, 1, "")
	return normalizeTrackingResult(providerResult, a.now().UTC()), nil
}

func (a *TrackingAdapter) supports(courierCode string) bool {
	courierCode = strings.ToLower(strings.TrimSpace(courierCode))
	for _, supported := range a.courierCodes {
		if supported == courierCode {
			return true
		}
	}
	return false
}

func (a *TrackingAdapter) recordCall(
	ctx context.Context,
	fingerprint string,
	httpStatus int,
	outcome string,
	duration time.Duration,
	quotaCost int,
	errorCode string,
) {
	if a.recorder == nil {
		return
	}
	_ = a.recorder.RecordProviderAPICall(
		ctx,
		a.Code(),
		a.credentialAlias,
		"track/waybill",
		fingerprint,
		httpStatus,
		outcome,
		duration,
		quotaCost,
		errorCode,
	)
}

func normalizeTrackingResult(input WaybillTracking, fetchedAt time.Time) tracking.Result {
	currentStatusText := strings.Join([]string{
		input.Summary.Status,
		input.Delivery.Status,
	}, " ")
	status := normalizeTrackingStatus(currentStatusText, input.Delivered)
	if status == "unknown" {
		status = normalizeTrackingStatus(lastManifestText(input.Manifest), false)
	}
	label := trackingStatusLabel(status)
	events := make([]tracking.Event, 0, len(input.Manifest))
	for _, manifest := range input.Manifest {
		occurredAt, ok := parseProviderDateTime(manifest.Date, manifest.Time)
		if !ok {
			continue
		}
		events = append(events, tracking.Event{
			Code:        manifest.Code,
			Description: manifest.Description,
			Location:    manifest.City,
			OccurredAt:  occurredAt,
		})
	}
	summary := map[string]any{
		"courier_code": input.Summary.CourierCode,
		"courier_name": input.Summary.CourierName,
		"service_code": input.Summary.ServiceCode,
		"origin":       firstNonEmpty(input.Summary.Origin, input.Details.Origin),
		"destination":  firstNonEmpty(input.Summary.Destination, input.Details.Destination),
		"weight":       input.Details.Weight,
		"waybill_date": firstNonEmpty(input.Summary.WaybillDate, input.Details.WaybillDate),
		"delivered":    input.Delivered,
	}
	isFinal := status == "delivered" || status == "returned" || status == "cancelled"
	var nextRefreshAt *time.Time
	if !isFinal {
		next := fetchedAt.Add(refreshInterval(status))
		nextRefreshAt = &next
	}
	return tracking.Result{
		NormalizedStatus: status,
		StatusLabel:      label,
		Summary:          summary,
		Events:           events,
		ProviderCode:     "rajaongkir",
		FetchedAt:        fetchedAt,
		NextRefreshAt:    nextRefreshAt,
		IsFinal:          isFinal,
	}
}

func normalizeTrackingStatus(value string, delivered bool) string {
	if delivered {
		return "delivered"
	}
	value = strings.ToLower(value)
	switch {
	case containsAny(value, "delivered", "terkirim", "diterima", "proof of delivery", "pod"):
		return "delivered"
	case containsAny(value, "returned", "return to sender", "retur"):
		return "returned"
	case containsAny(value, "cancelled", "canceled", "dibatalkan"):
		return "cancelled"
	case containsAny(value, "delivery failed", "undelivered", "gagal antar", "gagal kirim"):
		return "delivery_failed"
	case containsAny(value, "out for delivery", "with delivery courier", "dibawa kurir", "proses antar"):
		return "out_for_delivery"
	case containsAny(value, "picked up", "pickup", "collected", "manifested", "received by courier"):
		return "picked_up"
	case containsAny(value, "in transit", "transit", "departed", "gateway", "sorting", "perjalanan"):
		return "in_transit"
	case containsAny(value, "pending", "awaiting pickup", "shipment information", "booking"):
		return "pending_pickup"
	default:
		return "unknown"
	}
}

func trackingStatusLabel(status string) string {
	labels := map[string]string{
		"pending_pickup":   "Menunggu pickup",
		"picked_up":        "Sudah dipickup",
		"in_transit":       "Dalam perjalanan",
		"out_for_delivery": "Sedang diantar",
		"delivered":        "Terkirim",
		"delivery_failed":  "Pengantaran gagal",
		"returned":         "Dikembalikan",
		"cancelled":        "Dibatalkan",
		"unknown":          "Status belum diketahui",
	}
	return labels[status]
}

func refreshInterval(status string) time.Duration {
	switch status {
	case "out_for_delivery":
		return 15 * time.Minute
	case "in_transit":
		return 45 * time.Minute
	case "delivery_failed":
		return 2 * time.Hour
	case "picked_up":
		return time.Hour
	default:
		return 3 * time.Hour
	}
}

func parseProviderDateTime(dateValue, timeValue string) (time.Time, bool) {
	value := strings.TrimSpace(strings.TrimSpace(dateValue) + " " + strings.TrimSpace(timeValue))
	jakarta := time.FixedZone("Asia/Jakarta", 7*60*60)
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"02-01-2006 15:04:05",
		"02-01-2006 15:04",
		"2006-01-02",
	} {
		parsed, err := time.ParseInLocation(layout, value, jakarta)
		if err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

func providerCallFailure(err error) (string, string) {
	switch {
	case errors.Is(err, tracking.ErrProviderUnauthorized):
		return "PROVIDER_UNAUTHORIZED", "client_error"
	case errors.Is(err, tracking.ErrProviderQuota):
		return "PROVIDER_QUOTA_EXHAUSTED", "provider_error"
	case errors.Is(err, tracking.ErrWaybillNotFound):
		return "WAYBILL_NOT_FOUND", "client_error"
	case errors.Is(err, tracking.ErrPhoneSuffixRequired):
		return "PHONE_VALIDATION_REQUIRED", "client_error"
	case errors.Is(err, tracking.ErrProviderTimeout):
		return "PROVIDER_TIMEOUT", "timeout"
	default:
		return "PROVIDER_UNAVAILABLE", "provider_error"
	}
}

func trackingRequestFingerprint(courierCode, waybill string) string {
	hash := sha256.Sum256([]byte(
		strings.ToLower(strings.TrimSpace(courierCode)) + ":" +
			strings.ToUpper(strings.TrimSpace(waybill)),
	))
	return hex.EncodeToString(hash[:])
}

func lastManifestText(manifest []WaybillManifest) string {
	if len(manifest) == 0 {
		return ""
	}
	last := manifest[len(manifest)-1]
	return last.Code + " " + last.Description
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
