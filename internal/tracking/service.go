package tracking

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/emisell/api-kurir/internal/tenancy"
	"golang.org/x/sync/singleflight"
)

var validWaybill = regexp.MustCompile(`^[A-Z0-9-]{6,40}$`)
var validPhoneSuffix = regexp.MustCompile(`^\d{5}$`)
var validReference = regexp.MustCompile(`^[A-Za-z0-9._:/-]{1,128}$`)

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

// RevealWaybill decrypts an AWB that has already passed through the tracking
// repository. It is intentionally used only by the admin monitor handler; the
// public tracking responses and logs continue to expose the masked value.
func (s *Service) RevealWaybill(courierCode string, ciphertext []byte) (string, error) {
	if s == nil || s.cipher == nil {
		return "", errors.New("tracking cipher is unavailable")
	}
	courierCode = strings.ToLower(strings.TrimSpace(courierCode))
	plaintext, err := s.cipher.Decrypt(ciphertext, []byte(courierCode))
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
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

// Verify checks the seller-selected courier first. Only when the provider
// confirms that the AWB is not present do strong format candidates get tried.
// Pattern rules never reject a provider-valid AWB by themselves.
func (s *Service) Verify(
	ctx context.Context,
	adapter Adapter,
	request VerificationRequest,
) (VerificationResult, error) {
	request.CourierCode = strings.ToLower(strings.TrimSpace(request.CourierCode))
	request.Waybill = strings.ToUpper(strings.TrimSpace(request.Waybill))
	request.LastPhoneDigits = strings.TrimSpace(request.LastPhoneDigits)
	if !validWaybill.MatchString(request.Waybill) {
		return VerificationResult{
			Status: "invalid_format", RequestedCourier: request.CourierCode,
			FormatStatus: "invalid", CandidateCouriers: []string{},
			Message: "Format nomor resi tidak dapat diproses.",
		}, ErrInvalidWaybill
	}
	if adapter == nil {
		return VerificationResult{}, ErrAdapterUnavailable
	}
	candidates, formatStatus := rankedCourierCandidates(
		request.Waybill, request.CourierCode, adapter.CourierCodes(),
	)
	verification := VerificationResult{
		Status: "not_found", RequestedCourier: request.CourierCode,
		FormatStatus: formatStatus, CandidateCouriers: candidates,
		Message: "Nomor resi belum ditemukan pada provider.",
	}
	if len(candidates) == 0 || candidates[0] != request.CourierCode {
		return verification, ErrUnsupportedCourier
	}
	if len(candidates) > 3 {
		candidates = candidates[:3]
		verification.CandidateCouriers = candidates
	}
	for _, candidate := range candidates {
		result, err := s.TrackNow(
			ctx, adapter, candidate, request.Waybill, request.LastPhoneDigits,
		)
		verification.ProviderChecked = true
		if err == nil {
			status := "verified"
			message := "Nomor resi valid dan sesuai dengan ekspedisi yang dipilih."
			if candidate != request.CourierCode {
				status = "courier_mismatch"
				message = "Nomor resi valid, tetapi milik ekspedisi lain."
			}
			verification.Status = status
			verification.DetectedCourier = candidate
			verification.Message = message
			verification.Shipment = &Shipment{
				CourierCode: candidate, WaybillMasked: maskWaybill(request.Waybill),
				NormalizedStatus: result.NormalizedStatus, StatusLabel: result.StatusLabel,
				Summary: result.Summary, Events: result.Events, ProviderCode: result.ProviderCode,
				ProviderFetchedAt: &result.FetchedAt, NextRefreshAt: result.NextRefreshAt,
				IsFinal: result.IsFinal, ValidationStatus: "valid", ValidationChecked: &result.FetchedAt,
			}
			return verification, nil
		}
		if errors.Is(err, ErrWaybillNotFound) {
			continue
		}
		verification.Status = "provider_unavailable"
		verification.Message = "Provider belum dapat menyelesaikan verifikasi resi."
		return verification, err
	}
	return verification, ErrWaybillNotFound
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
	if shipment.ValidationStatus == "invalid" {
		return Result{}, ErrWaybillNotFound
	}
	if shipment.ValidationStatus == "not_found" &&
		shipment.NextRefreshAt != nil &&
		s.now().UTC().Before(*shipment.NextRefreshAt) {
		return Result{}, ErrWaybillNotFound
	}
	if shipment.LastErrorCode != "" && shipment.NextRefreshAt != nil &&
		s.now().UTC().Before(*shipment.NextRefreshAt) {
		if shipment.ValidationStatus == "valid" && shipment.ProviderFetchedAt != nil {
			return resultFromShipment(shipment), nil
		}
		return Result{}, trackingErrorFromCode(shipment.LastErrorCode)
	}
	if shipment.PollingStopped {
		if shipment.ValidationStatus == "valid" && shipment.ProviderFetchedAt != nil {
			return resultFromShipment(shipment), nil
		}
		return Result{}, ErrProviderQuota
	}
	if shipment.ProviderFetchedAt != nil &&
		(shipment.IsFinal ||
			(shipment.NextRefreshAt != nil &&
				s.now().UTC().Before(*shipment.NextRefreshAt))) {
		return resultFromShipment(shipment), nil
	}

	result, err := adapter.Track(ctx, request)
	if err != nil {
		if errors.Is(err, ErrWaybillNotFound) {
			fetchedAt := s.now().UTC()
			next, invalid := notFoundCheckpoint(
				fetchedAt,
				shipment.NotFoundCount+1,
				shipment.ProviderHitCount+1,
				shipment.ProviderHitLimit,
			)
			if recordErr := s.repository.RecordNotFoundImmediate(
				ctx,
				shipment.ID,
				fetchedAt,
				next,
				invalid,
			); recordErr != nil {
				return Result{}, recordErr
			}
		} else {
			code := trackingFailureCode(err)
			countProviderHit := code != "ADAPTER_UNAVAILABLE" &&
				code != "DECRYPTION_FAILED" &&
				code != "PROVIDER_QUOTA_EXHAUSTED"
			retryAt := s.now().UTC().Add(retryDelay(
				code,
				shipment.ProviderHitCount,
				s.now(),
			))
			if recordErr := s.repository.RecordImmediateFailure(
				ctx, shipment.ID, code, retryAt, countProviderHit,
			); recordErr != nil {
				return Result{}, recordErr
			}
		}
		return Result{}, err
	}
	if result.FetchedAt.IsZero() {
		result.FetchedAt = s.now().UTC()
	}
	result = ApplyEconomyCheckpoint(
		result,
		shipment.ProviderHitCount+1,
		shipment.ProviderHitLimit,
	)
	if err := s.repository.CompleteImmediate(ctx, shipment.ID, result); err != nil {
		return Result{}, err
	}
	return result, nil
}

func trackingErrorFromCode(code string) error {
	switch code {
	case "PROVIDER_UNAUTHORIZED":
		return ErrProviderUnauthorized
	case "PROVIDER_RATE_LIMITED":
		return ErrProviderRateLimited
	case "PROVIDER_QUOTA_EXHAUSTED":
		return ErrProviderQuota
	case "PROVIDER_TIMEOUT":
		return ErrProviderTimeout
	case "PHONE_VALIDATION_REQUIRED":
		return ErrPhoneSuffixRequired
	case "WAYBILL_NOT_FOUND":
		return ErrWaybillNotFound
	default:
		return ErrProviderUnavailable
	}
}

func (s *Service) Subscribe(
	ctx context.Context,
	request SubscriptionRequest,
) (Subscription, error) {
	request.OrderReference = strings.TrimSpace(request.OrderReference)
	request.FulfillmentReference = strings.TrimSpace(request.FulfillmentReference)
	if !validReference.MatchString(request.OrderReference) ||
		!validReference.MatchString(request.FulfillmentReference) {
		return Subscription{}, ErrInvalidWaybill
	}
	if tenancy.TenantID(ctx) == "" {
		return Subscription{}, ErrNotFound
	}
	shipment, err := s.Register(
		ctx,
		request.CourierCode,
		request.Waybill,
		request.LastPhoneDigits,
	)
	if err != nil {
		return Subscription{}, err
	}
	return s.repository.UpsertSubscription(
		ctx,
		shipment.ID,
		request.OrderReference,
		request.FulfillmentReference,
		0,
	)
}

func (s *Service) ReplaceSubscription(
	ctx context.Context,
	adapter Adapter,
	request SubscriptionRequest,
) (Subscription, VerificationResult, error) {
	current, err := s.Subscription(ctx, request.FulfillmentReference)
	if err != nil {
		return Subscription{}, VerificationResult{}, err
	}
	if current.Shipment.IsFinal {
		return Subscription{}, VerificationResult{}, ErrFinalShipmentLocked
	}
	if request.ExpectedRevision <= 0 || current.Revision != request.ExpectedRevision {
		return Subscription{}, VerificationResult{}, ErrRevisionConflict
	}
	verification, err := s.Verify(ctx, adapter, VerificationRequest{
		CourierCode: request.CourierCode, Waybill: request.Waybill,
		LastPhoneDigits: request.LastPhoneDigits,
	})
	if err != nil {
		return Subscription{}, verification, err
	}
	if verification.Status == "courier_mismatch" {
		return Subscription{}, verification, ErrCourierMismatch
	}
	if verification.Status != "verified" {
		return Subscription{}, verification, ErrWaybillNotFound
	}
	request.OrderReference = strings.TrimSpace(request.OrderReference)
	if !validReference.MatchString(request.OrderReference) {
		return Subscription{}, verification, ErrInvalidWaybill
	}
	shipment, err := s.Register(
		ctx, request.CourierCode, request.Waybill, request.LastPhoneDigits,
	)
	if err != nil {
		return Subscription{}, verification, err
	}
	subscription, err := s.repository.UpsertSubscription(
		ctx, shipment.ID, request.OrderReference, request.FulfillmentReference,
		request.ExpectedRevision,
	)
	return subscription, verification, err
}

func (s *Service) Subscription(
	ctx context.Context,
	fulfillmentReference string,
) (Subscription, error) {
	fulfillmentReference = strings.TrimSpace(fulfillmentReference)
	if !validReference.MatchString(fulfillmentReference) {
		return Subscription{}, ErrNotFound
	}
	return s.repository.GetSubscription(ctx, fulfillmentReference)
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
