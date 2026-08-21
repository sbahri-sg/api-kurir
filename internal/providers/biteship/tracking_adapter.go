package biteship

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/emisell/api-kurir/internal/providercredentials"
	"github.com/emisell/api-kurir/internal/rates"
	"github.com/emisell/api-kurir/internal/tenancy"
	"github.com/emisell/api-kurir/internal/tracking"
)

const publicTrackingEndpoint = "trackings/public"

// DefaultTrackingCouriers deliberately contains only non-instant couriers that
// are absent from the default RajaOngkir tracking adapter.
var DefaultTrackingCouriers = []string{
	"ide", "rpx", "sentral", "sicepat",
}

var canonicalToBiteshipCourier = map[string]string{
	"ide":     "idexpress",
	"sentral": "sentralcargo",
}

type CredentialResolver interface {
	ResolveProviderCredential(
		ctx context.Context,
		providerCode string,
	) (secret, credentialAlias string, dailyLimit int64, err error)
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

type DynamicTrackingAdapter struct {
	resolver     CredentialResolver
	baseURL      string
	timeout      time.Duration
	quota        rates.QuotaRepository
	recorder     ProviderCallRecorder
	courierCodes []string
	now          func() time.Time
}

func NewDynamicTrackingAdapter(
	resolver CredentialResolver,
	baseURL string,
	timeout time.Duration,
	quota rates.QuotaRepository,
	recorder ProviderCallRecorder,
	courierCodes []string,
) *DynamicTrackingAdapter {
	if len(courierCodes) == 0 {
		courierCodes = DefaultTrackingCouriers
	}
	return &DynamicTrackingAdapter{
		resolver: resolver, baseURL: baseURL, timeout: timeout,
		quota: quota, recorder: recorder,
		courierCodes: normalizedTrackingCouriers(courierCodes),
		now:          time.Now,
	}
}

func (a *DynamicTrackingAdapter) Code() string { return "biteship" }

func (a *DynamicTrackingAdapter) CourierCodes() []string {
	return append([]string(nil), a.courierCodes...)
}

func (a *DynamicTrackingAdapter) Track(
	ctx context.Context,
	request tracking.Request,
) (tracking.Result, error) {
	if !supportsTrackingCourier(a.courierCodes, request.CourierCode) {
		return tracking.Result{}, tracking.ErrUnsupportedCourier
	}
	// Biteship is an internal platform fallback, not a merchant-selectable
	// provider. Resolve and charge its credential globally while preserving the
	// original merchant context for shipment storage and API-call audit.
	platformCtx := tenancy.WithIdentity(ctx, tenancy.Identity{})
	secret, alias, limit, err := a.resolver.ResolveProviderCredential(platformCtx, a.Code())
	if errors.Is(err, providercredentials.ErrNoActiveCredential) {
		return tracking.Result{}, tracking.ErrProviderUnavailable
	}
	if errors.Is(err, providercredentials.ErrAllCredentialsExhausted) {
		return tracking.Result{}, tracking.ErrProviderQuota
	}
	if err != nil {
		return tracking.Result{}, tracking.ErrProviderUnavailable
	}
	if a.quota == nil {
		return tracking.Result{}, tracking.ErrProviderUnavailable
	}
	fingerprint := biteshipTrackingFingerprint(request.CourierCode, request.Waybill)
	if err := a.quota.ConsumeProviderHit(platformCtx, a.Code(), alias, limit); err != nil {
		if errors.Is(err, rates.ErrProviderQuotaExhausted) {
			a.record(ctx, alias, fingerprint, 0, "client_error", 0, 0, "LOCAL_QUOTA_EXHAUSTED")
			return tracking.Result{}, tracking.ErrProviderQuota
		}
		return tracking.Result{}, tracking.ErrProviderUnavailable
	}

	providerCourier := canonicalToBiteshipCourier[strings.ToLower(request.CourierCode)]
	if providerCourier == "" {
		providerCourier = strings.ToLower(request.CourierCode)
	}
	startedAt := a.now()
	providerResult, err := NewClient(a.baseURL, secret, a.timeout).TrackPublic(
		ctx,
		request.Waybill,
		providerCourier,
	)
	duration := a.now().Sub(startedAt)
	if err != nil {
		mapped := trackingProviderError(err)
		status := 0
		var apiError *APIError
		if errors.As(err, &apiError) {
			status = apiError.StatusCode
		}
		a.record(ctx, alias, fingerprint, status, "provider_error", duration, 1, trackingErrorCode(mapped))
		return tracking.Result{}, mapped
	}
	a.record(ctx, alias, fingerprint, http.StatusOK, "success", duration, 1, "")
	return normalizeBiteshipTracking(providerResult, request.CourierCode, a.now().UTC()), nil
}

func (a *DynamicTrackingAdapter) record(
	ctx context.Context,
	alias, fingerprint string,
	status int,
	outcome string,
	duration time.Duration,
	quotaCost int,
	errorCode string,
) {
	if a.recorder == nil {
		return
	}
	_ = a.recorder.RecordProviderAPICall(
		ctx, a.Code(), alias, publicTrackingEndpoint, fingerprint,
		status, outcome, duration, quotaCost, errorCode,
	)
}

func normalizeBiteshipTracking(
	input PublicTracking,
	courierCode string,
	fetchedAt time.Time,
) tracking.Result {
	status := normalizeBiteshipStatus(input.Status)
	events := make([]tracking.Event, 0, len(input.History))
	for _, history := range input.History {
		occurredAt, err := time.Parse(time.RFC3339, history.UpdatedAt)
		if err != nil {
			continue
		}
		events = append(events, tracking.Event{
			Code:        history.Status,
			Description: history.Note,
			OccurredAt:  occurredAt.UTC(),
		})
	}
	result := tracking.Result{
		NormalizedStatus: status,
		StatusLabel:      biteshipStatusLabel(status),
		Summary: map[string]any{
			"courier_code":    strings.ToLower(strings.TrimSpace(courierCode)),
			"courier_name":    input.Courier.Company,
			"waybill_number":  input.WaybillID,
			"shipper_name":    input.Origin.ContactName,
			"receiver_name":   input.Destination.ContactName,
			"origin":          input.Origin.Address,
			"destination":     input.Destination.Address,
			"status":          input.Status,
			"delivery_status": input.Status,
			"tracking_link":   input.Link,
			"delivered":       status == "delivered",
		},
		Events:       events,
		ProviderCode: "biteship",
		FetchedAt:    fetchedAt,
		IsFinal:      false,
	}
	return tracking.ApplyEconomyCheckpoint(result, 1, tracking.DefaultProviderHitLimit)
}

func normalizeBiteshipStatus(value string) string {
	normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(strings.TrimSpace(value)))
	switch normalized {
	case "confirmed", "allocated", "pickingup":
		return "pending_pickup"
	case "picked":
		return "picked_up"
	case "intransit", "returnintransit", "onhold":
		return "in_transit"
	case "droppingoff":
		return "out_for_delivery"
	case "delivered":
		return "delivered"
	case "returned", "disposed":
		return "returned"
	case "rejected", "couriernotfound", "cancelled", "canceled":
		return "cancelled"
	default:
		return "unknown"
	}
}

func trackingProviderError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return tracking.ErrProviderTimeout
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return tracking.ErrProviderTimeout
	}
	var apiError *APIError
	if !errors.As(err, &apiError) {
		return tracking.ErrProviderUnavailable
	}
	message := strings.ToLower(apiError.Message)
	switch {
	case apiError.StatusCode == http.StatusUnauthorized || apiError.StatusCode == http.StatusForbidden:
		return tracking.ErrProviderUnauthorized
	case apiError.StatusCode == http.StatusTooManyRequests:
		return tracking.ErrProviderRateLimited
	case strings.Contains(message, "balance") || strings.Contains(message, "saldo"):
		return tracking.ErrProviderQuota
	case apiError.Code == 40003001 || apiError.Code == 40003003 || strings.Contains(message, "not found"):
		return tracking.ErrWaybillNotFound
	default:
		return tracking.ErrProviderUnavailable
	}
}

func trackingErrorCode(err error) string {
	switch {
	case errors.Is(err, tracking.ErrProviderUnauthorized):
		return "PROVIDER_UNAUTHORIZED"
	case errors.Is(err, tracking.ErrProviderRateLimited):
		return "PROVIDER_RATE_LIMITED"
	case errors.Is(err, tracking.ErrProviderQuota):
		return "PROVIDER_QUOTA_EXHAUSTED"
	case errors.Is(err, tracking.ErrProviderTimeout):
		return "PROVIDER_TIMEOUT"
	case errors.Is(err, tracking.ErrWaybillNotFound):
		return "WAYBILL_NOT_FOUND"
	default:
		return "PROVIDER_UNAVAILABLE"
	}
}

func normalizedTrackingCouriers(input []string) []string {
	seen := make(map[string]struct{}, len(input))
	result := make([]string, 0, len(input))
	for _, code := range input {
		code = strings.ToLower(strings.TrimSpace(code))
		if code == "" || code == "gojek" || code == "grab" {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, code)
	}
	return result
}

func supportsTrackingCourier(couriers []string, target string) bool {
	for _, courier := range couriers {
		if strings.EqualFold(courier, target) {
			return true
		}
	}
	return false
}

func biteshipTrackingFingerprint(courierCode, waybill string) string {
	hash := sha256.Sum256([]byte(strings.ToLower(courierCode) + ":" + strings.ToUpper(waybill)))
	return hex.EncodeToString(hash[:])
}

func biteshipStatusLabel(status string) string {
	labels := map[string]string{
		"pending_pickup":   "Menunggu pickup",
		"picked_up":        "Sudah dipickup",
		"in_transit":       "Dalam perjalanan",
		"out_for_delivery": "Sedang diantar",
		"delivered":        "Terkirim",
		"returned":         "Dikembalikan",
		"cancelled":        "Dibatalkan",
		"unknown":          "Status belum diketahui",
	}
	return labels[status]
}
