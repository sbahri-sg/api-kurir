package tracking

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
)

var validWaybill = regexp.MustCompile(`^[A-Z0-9-]{6,40}$`)
var validPhoneSuffix = regexp.MustCompile(`^\d{5}$`)

type Service struct {
	repository        Repository
	cipher            *Cipher
	supportedCouriers map[string]struct{}
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
	}
}

func (s *Service) Register(
	ctx context.Context,
	courierCode, waybill, lastPhoneDigits string,
) (Shipment, error) {
	courierCode = strings.ToLower(strings.TrimSpace(courierCode))
	waybill = strings.ToUpper(strings.TrimSpace(waybill))
	lastPhoneDigits = strings.TrimSpace(lastPhoneDigits)
	if courierCode == "" || !validWaybill.MatchString(waybill) {
		return Shipment{}, ErrInvalidWaybill
	}
	if len(s.supportedCouriers) > 0 {
		if _, supported := s.supportedCouriers[courierCode]; !supported {
			return Shipment{}, ErrUnsupportedCourier
		}
	}
	if lastPhoneDigits != "" && !validPhoneSuffix.MatchString(lastPhoneDigits) {
		return Shipment{}, ErrInvalidPhoneSuffix
	}

	hash := sha256.Sum256([]byte(courierCode + ":" + waybill))
	waybillHash := hex.EncodeToString(hash[:])
	ciphertext, err := s.cipher.Encrypt([]byte(waybill), []byte(courierCode))
	if err != nil {
		return Shipment{}, err
	}
	var providerContextCiphertext []byte
	if lastPhoneDigits != "" {
		contextJSON, err := json.Marshal(map[string]string{
			"last_phone_number": lastPhoneDigits,
		})
		if err != nil {
			return Shipment{}, err
		}
		providerContextCiphertext, err = s.cipher.Encrypt(
			contextJSON,
			[]byte(courierCode+":provider-context"),
		)
		if err != nil {
			return Shipment{}, err
		}
	}
	return s.repository.Register(
		ctx,
		courierCode,
		waybillHash,
		maskWaybill(waybill),
		ciphertext,
		providerContextCiphertext,
	)
}

func maskWaybill(waybill string) string {
	if len(waybill) <= 4 {
		return strings.Repeat("*", len(waybill))
	}
	return strings.Repeat("*", len(waybill)-4) + waybill[len(waybill)-4:]
}
