package hosted

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	courierspkg "github.com/emisell/api-kurir/internal/couriers"
	"github.com/emisell/api-kurir/internal/providercredentials"
	"github.com/emisell/api-kurir/internal/rates"
	"github.com/emisell/api-kurir/internal/tracking"
)

type TrackingAdapter struct {
	providerCode string
	client       *Client
	credentials  RateCredentialResolver
	quota        rates.QuotaRepository
	couriers     []string
	now          func() time.Time
}

func NewTrackingAdapter(
	providerCode string,
	client *Client,
	credentials RateCredentialResolver,
	quota rates.QuotaRepository,
	couriers []string,
) *TrackingAdapter {
	normalizedCouriers := make([]string, 0, len(couriers))
	seen := make(map[string]struct{}, len(couriers))
	for _, courierCode := range couriers {
		courierCode = courierspkg.NormalizeCode(courierCode)
		if courierCode == "" {
			continue
		}
		if _, exists := seen[courierCode]; exists {
			continue
		}
		seen[courierCode] = struct{}{}
		normalizedCouriers = append(normalizedCouriers, courierCode)
	}
	return &TrackingAdapter{
		providerCode: providerCode, client: client, credentials: credentials,
		quota: quota, couriers: normalizedCouriers, now: time.Now,
	}
}

func (a *TrackingAdapter) Code() string           { return a.providerCode }
func (a *TrackingAdapter) CourierCodes() []string { return append([]string(nil), a.couriers...) }

func (a *TrackingAdapter) Track(ctx context.Context, request tracking.Request) (tracking.Result, error) {
	if !containsCourier(a.couriers, request.CourierCode) {
		return tracking.Result{}, tracking.ErrUnsupportedCourier
	}
	secret, alias, limit, err := a.credentials.ResolveProviderCredentialForCapability(
		ctx, a.providerCode, "tracking:read",
	)
	if errors.Is(err, providercredentials.ErrNoActiveCredential) {
		return tracking.Result{}, tracking.ErrProviderUnavailable
	}
	if errors.Is(err, providercredentials.ErrAllCredentialsExhausted) {
		return tracking.Result{}, tracking.ErrProviderQuota
	}
	if err != nil || a.client == nil || a.quota == nil {
		return tracking.Result{}, tracking.ErrProviderUnavailable
	}
	if err := a.quota.ConsumeProviderHit(ctx, a.providerCode, alias, limit); err != nil {
		if errors.Is(err, rates.ErrProviderQuotaExhausted) {
			return tracking.Result{}, tracking.ErrProviderQuota
		}
		return tracking.Result{}, tracking.ErrProviderUnavailable
	}
	input := map[string]string{
		"waybill_number": request.Waybill,
		"courier_code":   request.CourierCode,
	}
	if request.LastPhoneDigits != "" {
		input["last_phone_number"] = request.LastPhoneDigits
	}
	var response struct {
		Data struct {
			WaybillNumber string `json:"waybill_number"`
			CourierCode   string `json:"courier_code"`
			CourierName   string `json:"courier_name"`
			ServiceCode   string `json:"service_code"`
			Status        string `json:"status"`
			Delivered     bool   `json:"delivered"`
			Origin        string `json:"origin"`
			Destination   string `json:"destination"`
			History       []struct {
				Code        string `json:"code"`
				Description string `json:"description"`
				EventAt     string `json:"event_at"`
				Location    string `json:"location"`
			} `json:"history"`
		} `json:"data"`
	}
	if err := a.client.Do(ctx, http.MethodPost, "tracking/waybills", "key", secret, input, &response); err != nil {
		return tracking.Result{}, mapTrackingError(err)
	}
	fetchedAt := a.now().UTC()
	status := normalizeStatus(response.Data.Status, response.Data.Delivered)
	events := make([]tracking.Event, 0, len(response.Data.History))
	var shippedAt, deliveredAt *time.Time
	for _, item := range response.Data.History {
		occurredAt, ok := parseEventTime(item.EventAt)
		if !ok {
			continue
		}
		events = append(events, tracking.Event{
			Code: item.Code, Description: item.Description,
			Location: item.Location, OccurredAt: occurredAt,
		})
		eventStatus := normalizeStatus(item.Code+" "+item.Description, false)
		if indicatesShipped(eventStatus) {
			shippedAt = earliest(shippedAt, occurredAt)
		}
		if eventStatus == "delivered" {
			deliveredAt = earliest(deliveredAt, occurredAt)
		}
	}
	if status == "delivered" && deliveredAt == nil {
		deliveredAt = timePointer(fetchedAt)
	}
	if indicatesShipped(status) && shippedAt == nil {
		shippedAt = timePointer(fetchedAt)
	}
	result := tracking.Result{
		NormalizedStatus: status, StatusLabel: statusLabel(status),
		Summary: map[string]any{
			"waybill_number": response.Data.WaybillNumber,
			"courier_code":   normalizedCourierCode(response.Data.CourierCode, request.CourierCode),
			"courier_name":   response.Data.CourierName,
			"service_code":   response.Data.ServiceCode,
			"origin":         response.Data.Origin,
			"destination":    response.Data.Destination,
			"status":         response.Data.Status,
			"delivered":      response.Data.Delivered,
		},
		Events: events, ProviderCode: a.providerCode, FetchedAt: fetchedAt,
		ShippedAt: shippedAt, DeliveredAt: deliveredAt,
		IsFinal: status == "delivered" || status == "cancelled" || status == "returned",
	}
	return tracking.ApplyEconomyCheckpoint(result, 1, tracking.DefaultProviderHitLimit), nil
}

func mapTrackingError(err error) error {
	var upstream *HTTPError
	if errors.As(err, &upstream) {
		switch upstream.Status {
		case http.StatusUnauthorized, http.StatusForbidden:
			return tracking.ErrProviderUnauthorized
		case http.StatusNotFound, http.StatusUnprocessableEntity:
			return tracking.ErrWaybillNotFound
		case http.StatusTooManyRequests:
			return tracking.ErrProviderRateLimited
		default:
			return tracking.ErrProviderUnavailable
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return tracking.ErrProviderTimeout
	}
	return tracking.ErrProviderUnavailable
}

func containsCourier(items []string, value string) bool {
	value = courierspkg.NormalizeCode(value)
	for _, item := range items {
		if courierspkg.NormalizeCode(item) == value {
			return true
		}
	}
	return false
}

func normalizedCourierCode(providerValue, requestedValue string) string {
	if normalized := courierspkg.NormalizeCode(providerValue); normalized != "" {
		return normalized
	}
	return courierspkg.NormalizeCode(requestedValue)
}

func parseEventTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", time.RFC3339} {
		if parsed, err := time.ParseInLocation(layout, value, time.FixedZone("WIB", 7*60*60)); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

func normalizeStatus(value string, delivered bool) string {
	if delivered {
		return "delivered"
	}
	value = strings.ToLower(value)
	switch {
	case strings.Contains(value, "deliver"), strings.Contains(value, "diterima"), strings.Contains(value, "terkirim"):
		return "delivered"
	case strings.Contains(value, "return"), strings.Contains(value, "retur"):
		return "returned"
	case strings.Contains(value, "cancel"), strings.Contains(value, "batal"):
		return "cancelled"
	case strings.Contains(value, "out for delivery"), strings.Contains(value, "diantar"):
		return "out_for_delivery"
	case strings.Contains(value, "pickup"), strings.Contains(value, "picked"):
		return "picked_up"
	case strings.Contains(value, "transit"), strings.Contains(value, "sorting"), strings.Contains(value, "gateway"), strings.Contains(value, "perjalanan"):
		return "in_transit"
	case strings.Contains(value, "pending"), strings.Contains(value, "booking"):
		return "pending_pickup"
	default:
		return "unknown"
	}
}

func statusLabel(status string) string {
	labels := map[string]string{
		"pending_pickup": "Menunggu pickup", "picked_up": "Sudah dipickup",
		"in_transit": "Dalam perjalanan", "out_for_delivery": "Sedang diantar",
		"delivered": "Terkirim", "returned": "Dikembalikan", "cancelled": "Dibatalkan",
	}
	if label := labels[status]; label != "" {
		return label
	}
	return "Status belum diketahui"
}

func indicatesShipped(status string) bool {
	return status == "picked_up" || status == "in_transit" || status == "out_for_delivery" || status == "delivered" || status == "returned"
}

func earliest(current *time.Time, candidate time.Time) *time.Time {
	if current == nil || candidate.Before(*current) {
		return timePointer(candidate)
	}
	return current
}

func timePointer(value time.Time) *time.Time { value = value.UTC(); return &value }
