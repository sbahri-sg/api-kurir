package tracking

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/emisell/api-kurir/internal/tenancy"
	"golang.org/x/sync/singleflight"
)

var validWaybill = regexp.MustCompile(`^[A-Z0-9-]{6,40}$`)
var validPhoneSuffix = regexp.MustCompile(`^\d{5}$`)

type Service struct {
	repository        Repository
	cipher            *Cipher
	supportedCouriers map[string]struct{}
	now               func() time.Time
	immediateGroup    singleflight.Group
}

func NewService(
	repository Repository,
	cipher *Cipher,
	supportedCouriers ...string,
) *Service {
	supported := make(map[string]struct{}, len(supportedCouriers))
	for _, courierCode := range supportedCouriers {
		courierCode = strings.ToLower(strings.TrimSpace(courierCode))
		if courierCode != "" {
			supported[courierCode] = struct{}{}
		}
	}
	return &Service{
		repository:        repository,
		cipher:            cipher,
		supportedCouriers: supported,
		now:               time.Now,
	}
}

func (s *Service) Register(
	ctx context.Context,
	courierCode, waybill, lastPhoneDigits string,
) (Shipment, error) {
	request, err := s.normalizeRequest(Request{
		CourierCode:     courierCode,
		Waybill:         waybill,
		LastPhoneDigits: lastPhoneDigits,
	})
	if err != nil {
		return Shipment{}, err
	}

	tenantID := tenancy.TenantID(ctx)
	hash := sha256.Sum256([]byte(trackingHashMaterial(tenantID, request)))
	waybillHash := hex.EncodeToString(hash[:])
	ciphertext, err := s.cipher.Encrypt(
		[]byte(request.Waybill),
		[]byte(request.CourierCode),
	)
	if err != nil {
		return Shipment{}, err
	}
	var providerContextCiphertext []byte
	if request.LastPhoneDigits != "" {
		contextJSON, err := json.Marshal(map[string]string{
			"last_phone_number": request.LastPhoneDigits,
		})
		if err != nil {
			return Shipment{}, err
		}
		providerContextCiphertext, err = s.cipher.Encrypt(
			contextJSON,
			[]byte(request.CourierCode+":provider-context"),
		)
		if err != nil {
			return Shipment{}, err
		}
	}
	return s.repository.Register(
		ctx,
		request.CourierCode,
		waybillHash,
		maskWaybill(request.Waybill),
		ciphertext,
		providerContextCiphertext,
	)
}

func (s *Service) TrackNow(
	ctx context.Context,
	adapter Adapter,
	courierCode, waybill, lastPhoneDigits string,
) (Result, error) {
	if adapter == nil {
		return Result{}, ErrAdapterUnavailable
	}
	request, err := s.normalizeRequest(Request{
		CourierCode:     courierCode,
		Waybill:         waybill,
		LastPhoneDigits: lastPhoneDigits,
	})
	if err != nil {
		return Result{}, err
	}
	tenantID := tenancy.TenantID(ctx)
	hash := sha256.Sum256([]byte(trackingHashMaterial(tenantID, request)))
	waybillHash := hex.EncodeToString(hash[:])

	value, err, _ := s.immediateGroup.Do(waybillHash, func() (any, error) {
		return s.trackNow(ctx, adapter, request, waybillHash)
	})
	if err != nil {
		return Result{}, err
	}
	return value.(Result), nil
}

func (s *Service) trackNow(
	ctx context.Context,
	adapter Adapter,
	request Request,
	waybillHash string,
) (Result, error) {
	ciphertext, err := s.cipher.Encrypt(
		[]byte(request.Waybill),
		[]byte(request.CourierCode),
	)
	if err != nil {
		return Result{}, err
	}
	var providerContextCiphertext []byte
	if request.LastPhoneDigits != "" {
		contextJSON, err := json.Marshal(map[string]string{
			"last_phone_number": request.LastPhoneDigits,
		})
		if err != nil {
			return Result{}, err
		}
		providerContextCiphertext, err = s.cipher.Encrypt(
			contextJSON,
			[]byte(request.CourierCode+":provider-context"),
		)
		if err != nil {
			return Result{}, err
		}
	}

	shipment, err := s.repository.RegisterImmediate(
		ctx,
		request.CourierCode,
		waybillHash,
		maskWaybill(request.Waybill),
		ciphertext,
		providerContextCiphertext,
	)
	if err != nil {
		return Result{}, err
	}
	if shipment.ProviderFetchedAt != nil &&
		(shipment.IsFinal ||
			(shipment.NextRefreshAt != nil &&
				s.now().UTC().Before(*shipment.NextRefreshAt))) {
		return resultFromShipment(shipment), nil
	}

	result, err := adapter.Track(ctx, request)
	if err != nil {
		return Result{}, err
	}
	if result.FetchedAt.IsZero() {
		result.FetchedAt = s.now().UTC()
	}
	if err := s.repository.CompleteImmediate(ctx, shipment.ID, result); err != nil {
		return Result{}, err
	}
	return result, nil
}

func (s *Service) normalizeRequest(request Request) (Request, error) {
	request.CourierCode = strings.ToLower(strings.TrimSpace(request.CourierCode))
	request.Waybill = strings.ToUpper(strings.TrimSpace(request.Waybill))
	request.LastPhoneDigits = strings.TrimSpace(request.LastPhoneDigits)
	if request.CourierCode == "" || !validWaybill.MatchString(request.Waybill) {
		return Request{}, ErrInvalidWaybill
	}
	if len(s.supportedCouriers) > 0 {
		if _, supported := s.supportedCouriers[request.CourierCode]; !supported {
			return Request{}, ErrUnsupportedCourier
		}
	}
	if request.LastPhoneDigits != "" &&
		!validPhoneSuffix.MatchString(request.LastPhoneDigits) {
		return Request{}, ErrInvalidPhoneSuffix
	}
	return request, nil
}

func resultFromShipment(shipment Shipment) Result {
	return Result{
		NormalizedStatus: shipment.NormalizedStatus,
		StatusLabel:      shipment.StatusLabel,
		Summary:          shipment.Summary,
		Events:           shipment.Events,
		ProviderCode:     shipment.ProviderCode,
		FetchedAt:        valueOrZero(shipment.ProviderFetchedAt),
		NextRefreshAt:    shipment.NextRefreshAt,
		IsFinal:          shipment.IsFinal,
	}
}

func valueOrZero(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func maskWaybill(waybill string) string {
	if len(waybill) <= 4 {
		return strings.Repeat("*", len(waybill))
	}
	return strings.Repeat("*", len(waybill)-4) + waybill[len(waybill)-4:]
}

func trackingHashMaterial(tenantID string, request Request) string {
	value := request.CourierCode + ":" + request.Waybill
	if tenantID == "" {
		return value
	}
	return tenantID + ":" + value
}
