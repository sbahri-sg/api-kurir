package tracking

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound             = errors.New("tracking shipment not found")
	ErrNoJobAvailable       = errors.New("tracking job not available")
	ErrAdapterUnavailable   = errors.New("tracking adapter unavailable")
	ErrInvalidWaybill       = errors.New("tracking waybill is invalid")
	ErrInvalidPhoneSuffix   = errors.New("tracking phone suffix is invalid")
	ErrPhoneSuffixRequired  = errors.New("tracking phone suffix is required")
	ErrUnsupportedCourier   = errors.New("tracking courier is unsupported")
	ErrProviderUnauthorized = errors.New("tracking provider unauthorized")
	ErrProviderRateLimited  = errors.New("tracking provider rate limited")
	ErrProviderQuota        = errors.New("tracking provider quota exhausted")
	ErrProviderTimeout      = errors.New("tracking provider timeout")
	ErrProviderUnavailable  = errors.New("tracking provider unavailable")
	ErrWaybillNotFound      = errors.New("tracking waybill not found")
)

type Shipment struct {
	ID                string         `json:"-"`
	CourierCode       string         `json:"courier"`
	WaybillMasked     string         `json:"waybill"`
	NormalizedStatus  string         `json:"status"`
	StatusLabel       string         `json:"status_label"`
	Summary           map[string]any `json:"summary"`
	Events            []Event        `json:"events"`
	ProviderCode      string         `json:"provider"`
	ProviderFetchedAt *time.Time     `json:"provider_fetched_at"`
	NextRefreshAt     *time.Time     `json:"next_refresh_at"`
	IsFinal           bool           `json:"is_final"`
	RefreshQueued     bool           `json:"refresh_queued"`
	LastErrorCode     string         `json:"last_error_code,omitempty"`
}

type Event struct {
	Code        string    `json:"code"`
	Description string    `json:"description"`
	Location    string    `json:"location,omitempty"`
	OccurredAt  time.Time `json:"occurred_at"`
}

type Job struct {
	ID                        string
	ShipmentID                string
	TenantID                  string
	ProviderCredentialID      string
	CourierCode               string
	WaybillCiphertext         []byte
	ProviderContextCiphertext []byte
	AttemptCount              int
	MaxAttempts               int
}

type Request struct {
	CourierCode     string
	Waybill         string
	LastPhoneDigits string
}

type Result struct {
	NormalizedStatus string
	StatusLabel      string
	Summary          map[string]any
	Events           []Event
	ProviderCode     string
	FetchedAt        time.Time
	NextRefreshAt    *time.Time
	IsFinal          bool
}

type Repository interface {
	Register(
		ctx context.Context,
		courierCode string,
		waybillHash string,
		waybillMasked string,
		waybillCiphertext []byte,
		providerContextCiphertext []byte,
	) (Shipment, error)
	RegisterImmediate(
		ctx context.Context,
		courierCode string,
		waybillHash string,
		waybillMasked string,
		waybillCiphertext []byte,
		providerContextCiphertext []byte,
	) (Shipment, error)
	Claim(ctx context.Context, workerID string, courierCodes []string) (Job, error)
	Complete(ctx context.Context, job Job, result Result) error
	CompleteImmediate(
		ctx context.Context,
		shipmentID string,
		result Result,
	) error
	Fail(ctx context.Context, job Job, errorCode, message string, retryAt time.Time) error
}

type Adapter interface {
	Code() string
	CourierCodes() []string
	Track(ctx context.Context, request Request) (Result, error)
}
