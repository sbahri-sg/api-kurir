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
	ErrCourierMismatch      = errors.New("tracking courier does not match waybill")
	ErrRevisionConflict     = errors.New("tracking subscription revision conflict")
	ErrFinalShipmentLocked  = errors.New("final tracking subscription cannot be replaced")
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
	ShippedAt         *time.Time     `json:"shipped_at"`
	DeliveredAt       *time.Time     `json:"delivered_at"`
	IsFinal           bool           `json:"is_final"`
	RefreshQueued     bool           `json:"refresh_queued"`
	LastErrorCode     string         `json:"last_error_code,omitempty"`
	ValidationStatus  string         `json:"validation_status"`
	ValidationChecked *time.Time     `json:"validation_checked_at,omitempty"`
	NotFoundCount     int            `json:"-"`
	ProviderHitCount  int            `json:"provider_hit_count"`
	ProviderHitLimit  int            `json:"provider_hit_limit"`
	PollingStopped    bool           `json:"polling_stopped"`
}

type Event struct {
	Code        string    `json:"code"`
	Description string    `json:"description"`
	Location    string    `json:"location,omitempty"`
	OccurredAt  time.Time `json:"occurred_at"`
}

type Job struct {
	ID                   string
	ShipmentID           string
	TenantID             string
	ProviderCredentialID string
	CourierCode          string
	Waybill              string
	// WaybillCiphertext is retained only while legacy encrypted rows are
	// migrated to the plaintext waybill column.
	WaybillCiphertext         []byte
	ProviderContextCiphertext []byte
	AttemptCount              int
	MaxAttempts               int
	NotFoundCount             int
	ProviderHitCount          int
	ProviderHitLimit          int
}

type SubscriptionRequest struct {
	OrderReference       string `json:"order_id"`
	FulfillmentReference string `json:"fulfillment_id"`
	CourierCode          string `json:"courier"`
	Waybill              string `json:"waybill"`
	LastPhoneDigits      string `json:"last_phone_number,omitempty"`
	ExpectedRevision     int    `json:"expected_revision,omitempty"`
}

type Subscription struct {
	ID                   string   `json:"id"`
	OrderReference       string   `json:"order_id"`
	FulfillmentReference string   `json:"fulfillment_id"`
	Active               bool     `json:"active"`
	Revision             int      `json:"revision"`
	Shipment             Shipment `json:"shipment"`
}

type SubscriptionRemoval struct {
	ID                   string `json:"id"`
	FulfillmentReference string `json:"fulfillment_id"`
	Active               bool   `json:"active"`
	Revision             int    `json:"revision"`
	Status               string `json:"status"`
	PollingStopped       bool   `json:"polling_stopped"`
	SnapshotRetained     bool   `json:"snapshot_retained"`
}

type VerificationRequest struct {
	CourierCode     string `json:"courier"`
	Waybill         string `json:"waybill"`
	LastPhoneDigits string `json:"last_phone_number,omitempty"`
}

type VerificationResult struct {
	Status            string    `json:"status"`
	RequestedCourier  string    `json:"requested_courier"`
	DetectedCourier   string    `json:"detected_courier,omitempty"`
	FormatStatus      string    `json:"format_status"`
	CandidateCouriers []string  `json:"candidate_couriers"`
	ProviderChecked   bool      `json:"provider_checked"`
	Message           string    `json:"message"`
	Shipment          *Shipment `json:"shipment,omitempty"`
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
	ShippedAt        *time.Time
	DeliveredAt      *time.Time
	IsFinal          bool
}

type Repository interface {
	Register(
		ctx context.Context,
		courierCode string,
		waybillHash string,
		waybill string,
		waybillMasked string,
		providerContextCiphertext []byte,
	) (Shipment, error)
	RegisterImmediate(
		ctx context.Context,
		courierCode string,
		waybillHash string,
		waybill string,
		waybillMasked string,
		providerContextCiphertext []byte,
	) (Shipment, error)
	Claim(ctx context.Context, workerID string, courierCodes []string) (Job, error)
	Complete(ctx context.Context, job Job, result Result) error
	CompleteImmediate(
		ctx context.Context,
		shipmentID string,
		result Result,
	) error
	RecordNotFound(ctx context.Context, job Job, fetchedAt time.Time, nextRefreshAt *time.Time, invalid bool) error
	RecordNotFoundImmediate(ctx context.Context, shipmentID string, fetchedAt time.Time, nextRefreshAt *time.Time, invalid bool) error
	RecordImmediateFailure(ctx context.Context, shipmentID, errorCode string, retryAt time.Time, countProviderHit bool) error
	Fail(ctx context.Context, job Job, errorCode, message string, retryAt time.Time) error
	UpsertSubscription(ctx context.Context, shipmentID, orderReference, fulfillmentReference string, expectedRevision int) (Subscription, error)
	GetSubscription(ctx context.Context, fulfillmentReference string) (Subscription, error)
	DeactivateSubscription(ctx context.Context, fulfillmentReference string) (SubscriptionRemoval, error)
}

type WebhookJob struct {
	ID           string
	EventType    string
	Data         map[string]any
	AttemptCount int
	MaxAttempts  int
	CreatedAt    time.Time
}

type WebhookRepository interface {
	ClaimWebhook(ctx context.Context, workerID string) (WebhookJob, error)
	CompleteWebhook(ctx context.Context, jobID string, httpStatus int) error
	FailWebhook(ctx context.Context, job WebhookJob, httpStatus int, message string, retryAt time.Time, retry bool) error
}

type Adapter interface {
	Code() string
	CourierCodes() []string
	Track(ctx context.Context, request Request) (Result, error)
}
