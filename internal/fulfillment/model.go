package fulfillment

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidRequest        = errors.New("fulfillment request is invalid")
	ErrInvalidIdempotency    = errors.New("idempotency key is invalid")
	ErrIdempotencyConflict   = errors.New("idempotency key payload conflict")
	ErrOperationInProgress   = errors.New("fulfillment operation is still in progress")
	ErrShipmentNotFound      = errors.New("fulfillment shipment not found")
	ErrShippingDisabled      = errors.New("shipping provider is not active")
	ErrProviderUnsupported   = errors.New("fulfillment provider is unsupported")
	ErrCredentialUnavailable = errors.New("fulfillment credential capability is unavailable")
	ErrProviderUnauthorized  = errors.New("fulfillment provider unauthorized")
	ErrProviderRejected      = errors.New("fulfillment provider rejected request")
	ErrProviderUnavailable   = errors.New("fulfillment provider unavailable")
	ErrProviderTimeout       = errors.New("fulfillment provider timeout")
	ErrPickupNotAllowed      = errors.New("fulfillment pickup is not allowed")
	ErrShipmentFinal         = errors.New("fulfillment shipment is final")
	ErrLabelUnavailable      = errors.New("fulfillment label is unavailable")
	ErrQuoteNotFound         = errors.New("fulfillment quote not found")
	ErrQuoteExpired          = errors.New("fulfillment quote expired")
	ErrQuoteMismatch         = errors.New("fulfillment quote does not match request")
	ErrQuoteConsumed         = errors.New("fulfillment quote already consumed")
	ErrAmountMismatch        = errors.New("fulfillment payment amount does not match items")
	ErrNoLifecycleJob        = errors.New("fulfillment lifecycle job is not available")
	ErrNoWebhookJob          = errors.New("fulfillment webhook job is not available")
)

const (
	StatusBookingPending      = "booking_pending"
	StatusBooked              = "booked"
	StatusPickupRequested     = "pickup_requested"
	StatusPickedUp            = "picked_up"
	StatusInTransit           = "in_transit"
	StatusOutForDelivery      = "out_for_delivery"
	StatusDelivered           = "delivered"
	StatusCancellationPending = "cancellation_pending"
	StatusCancelled           = "cancelled"
	StatusProblem             = "problem"
	StatusBookingFailed       = "booking_failed"
)

type Address struct {
	Name          string   `json:"name"`
	Phone         string   `json:"phone"`
	Email         string   `json:"email,omitempty"`
	Address       string   `json:"address"`
	AddressNote   string   `json:"address_note,omitempty"`
	DestinationID int64    `json:"destination_id"`
	OfficialCode  string   `json:"official_code,omitempty"`
	PostalCode    string   `json:"postal_code,omitempty"`
	Latitude      *float64 `json:"latitude,omitempty"`
	Longitude     *float64 `json:"longitude,omitempty"`
}

type Item struct {
	Name        string `json:"name"`
	Variant     string `json:"variant,omitempty"`
	Quantity    int    `json:"quantity"`
	UnitPrice   int64  `json:"unit_price"`
	Subtotal    int64  `json:"subtotal"`
	WeightGrams int64  `json:"weight_grams"`
}

type Package struct {
	WeightGrams int64  `json:"weight_grams"`
	LengthCM    int    `json:"length_cm"`
	WidthCM     int    `json:"width_cm"`
	HeightCM    int    `json:"height_cm"`
	Items       []Item `json:"items"`
}

type Payment struct {
	Type                     string `json:"type"`
	ItemsSubtotal            int64  `json:"items_subtotal"`
	OrderDiscount            int64  `json:"order_discount"`
	TaxAmount                int64  `json:"tax_amount"`
	ShippingCost             int64  `json:"shipping_cost"`
	ShippingDiscount         int64  `json:"shipping_discount"`
	ServiceFee               int64  `json:"-"`
	AdditionalCost           int64  `json:"additional_cost"`
	ProviderShippingDiscount int64  `json:"-"`
	ProviderAdditionalCost   int64  `json:"-"`
	GrandTotal               int64  `json:"grand_total"`
	CODValue                 int64  `json:"cod_value,omitempty"`
	InsuranceValue           int64  `json:"insurance_value,omitempty"`
}

type QuotePackage struct {
	WeightGrams int64 `json:"weight_grams"`
	LengthCM    int   `json:"length_cm"`
	WidthCM     int   `json:"width_cm"`
	HeightCM    int   `json:"height_cm"`
	ItemValue   int64 `json:"item_value"`
}

type QuoteLocation struct {
	DestinationID int64    `json:"destination_id"`
	Latitude      *float64 `json:"latitude,omitempty"`
	Longitude     *float64 `json:"longitude,omitempty"`
}

type QuoteRequest struct {
	ProviderCode  string        `json:"provider_code,omitempty"`
	CourierCodes  []string      `json:"courier_codes,omitempty"`
	ServiceGroups []string      `json:"service_groups,omitempty"`
	Origin        QuoteLocation `json:"origin"`
	Destination   QuoteLocation `json:"destination"`
	Package       QuotePackage  `json:"package"`
	PaymentType   string        `json:"payment_type"`
}

// Quote is the canonical, server-side price lock consumed by CreateRequest.
// Provider-native identifiers remain internal and are never returned to Emisell.
type Quote struct {
	ID               string    `json:"quote_id"`
	ProviderCode     string    `json:"provider_code"`
	Environment      string    `json:"environment"`
	CourierCode      string    `json:"courier_code"`
	CourierName      string    `json:"courier_name"`
	ServiceCode      string    `json:"service_code"`
	ServiceName      string    `json:"service_name"`
	ServiceGroup     string    `json:"service_group"`
	DeliveryMode     string    `json:"delivery_mode"`
	ShippingCost     int64     `json:"shipping_cost"`
	ShippingCashback int64     `json:"shipping_cashback,omitempty"`
	ServiceFee       int64     `json:"service_fee,omitempty"`
	AdditionalCost   int64     `json:"additional_cost,omitempty"`
	GrandTotal       int64     `json:"grand_total"`
	CODValue         int64     `json:"cod_value,omitempty"`
	InsuranceValue   int64     `json:"insurance_value,omitempty"`
	Currency         string    `json:"currency"`
	ETD              string    `json:"etd,omitempty"`
	ExpiresAt        time.Time `json:"expires_at"`

	ProviderQuoteID   string `json:"-"`
	NativeServiceCode string `json:"-"`
	CredentialAlias   string `json:"-"`
	BindingHash       []byte `json:"-"`
}

type ProviderQuote struct {
	ProviderQuoteID  string
	CourierCode      string
	CourierName      string
	ServiceCode      string
	ServiceName      string
	ServiceGroup     string
	DeliveryMode     string
	ShippingCost     int64
	ShippingCashback int64
	ServiceFee       int64
	AdditionalCost   int64
	GrandTotal       int64
	CODValue         int64
	InsuranceValue   int64
	Currency         string
	ETD              string
	ExpiresAt        time.Time
}

type CreateRequest struct {
	ProviderCode      string  `json:"provider_code,omitempty"`
	MerchantReference string  `json:"merchant_reference"`
	QuoteID           string  `json:"quote_id"`
	BrandName         string  `json:"brand_name,omitempty"`
	CourierCode       string  `json:"courier_code"`
	ServiceCode       string  `json:"service_code"`
	DeliveryMode      string  `json:"delivery_mode"`
	Fulfillment       string  `json:"fulfillment"`
	Sender            Address `json:"sender"`
	Recipient         Address `json:"recipient"`
	Package           Package `json:"package"`
	Payment           Payment `json:"payment"`
	Notes             string  `json:"notes,omitempty"`
}

type PickupRequest struct {
	Mode        string    `json:"mode"`
	ScheduledAt time.Time `json:"scheduled_at,omitempty"`

	// PackageWeightGrams is internal provider context populated from the
	// immutable shipment snapshot. It is never accepted from merchant input.
	PackageWeightGrams int64 `json:"-"`
}

type CancelRequest struct {
	ReasonCode string `json:"reason_code"`
	Reason     string `json:"reason,omitempty"`
}

type Shipment struct {
	ID                         string     `json:"shipment_id"`
	MerchantReference          string     `json:"merchant_reference"`
	ProviderCode               string     `json:"provider_code"`
	ProviderShipmentID         string     `json:"provider_shipment_id,omitempty"`
	QuoteID                    string     `json:"quote_id"`
	CourierCode                string     `json:"courier_code"`
	ServiceCode                string     `json:"service_code"`
	DeliveryMode               string     `json:"delivery_mode"`
	Fulfillment                string     `json:"fulfillment"`
	AWB                        string     `json:"awb,omitempty"`
	Status                     string     `json:"status"`
	ProviderStatus             string     `json:"provider_status,omitempty"`
	ShippingCost               int64      `json:"shipping_cost"`
	Currency                   string     `json:"currency"`
	PackageWeightGrams         int64      `json:"-"`
	LabelAvailable             bool       `json:"label_available"`
	TrackingRegistrationStatus string     `json:"tracking_registration_status"`
	TrackingShipmentID         string     `json:"tracking_shipment_id,omitempty"`
	LiveTrackingURL            string     `json:"live_tracking_url,omitempty"`
	LastReconciledAt           *time.Time `json:"last_reconciled_at,omitempty"`
	NextReconcileAt            *time.Time `json:"next_reconcile_at,omitempty"`
	ReconcileAttemptCount      int        `json:"reconcile_attempt_count"`
	ReconcileError             string     `json:"reconcile_error,omitempty"`
	CreatedAt                  time.Time  `json:"created_at"`
	UpdatedAt                  time.Time  `json:"updated_at"`
	CancelledAt                *time.Time `json:"cancelled_at,omitempty"`
}

type HistoryEvent struct {
	ID             int64     `json:"id"`
	Status         string    `json:"status"`
	ProviderStatus string    `json:"provider_status,omitempty"`
	Description    string    `json:"description"`
	OccurredAt     time.Time `json:"occurred_at"`
}

type Label struct {
	Format      string     `json:"format"`
	ContentType string     `json:"content_type"`
	URL         string     `json:"url,omitempty"`
	Base64      string     `json:"base64,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

type ProviderCreateResult struct {
	ProviderShipmentID string
	AWB                string
	ProviderStatus     string
	Status             string
}

type ProviderPickupResult struct {
	ProviderOperationID string
	AWB                 string
	ProviderStatus      string
	Status              string
}

type ProviderCancelResult struct {
	ProviderStatus string
	Status         string
}

type ProviderDetailResult struct {
	AWB             string
	ProviderStatus  string
	Status          string
	LiveTrackingURL string
}

type DetailAdapter interface {
	Detail(
		ctx context.Context,
		credential string,
		shipment Shipment,
	) (ProviderDetailResult, error)
}

type LifecycleJob struct {
	ID           string
	TenantID     string
	ShipmentID   string
	JobType      string
	AttemptCount int
	MaxAttempts  int
}

type TrackingRegistration struct {
	TenantID             string
	OrderReference       string
	FulfillmentReference string
	CourierCode          string
	Waybill              string
}

type TrackingRegistrar interface {
	RegisterFulfillmentTracking(
		ctx context.Context,
		input TrackingRegistration,
	) (trackingShipmentID string, err error)
}

type LifecycleRepository interface {
	ClaimLifecycleJob(ctx context.Context, workerID string) (LifecycleJob, error)
	GetLifecycleShipment(ctx context.Context, tenantID, shipmentID string) (Shipment, error)
	CompleteTrackingRegistration(
		ctx context.Context,
		job LifecycleJob,
		trackingShipmentID string,
	) error
	CompleteReconciliation(
		ctx context.Context,
		job LifecycleJob,
		result ProviderDetailResult,
		nextRefreshAt *time.Time,
	) error
	FailLifecycleJob(
		ctx context.Context,
		job LifecycleJob,
		errorCode, message string,
		retryAt time.Time,
	) error
	EnqueueReconciliation(ctx context.Context, shipmentID string) error
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
	ClaimFulfillmentWebhook(ctx context.Context, workerID string) (WebhookJob, error)
	CompleteFulfillmentWebhook(ctx context.Context, jobID string, httpStatus int) error
	FailFulfillmentWebhook(
		ctx context.Context,
		job WebhookJob,
		httpStatus int,
		message string,
		retryAt time.Time,
		retry bool,
	) error
}

type Adapter interface {
	Code() string
	Create(ctx context.Context, credential string, request CreateRequest) (ProviderCreateResult, error)
	Pickup(ctx context.Context, credential string, shipment Shipment, request PickupRequest) (ProviderPickupResult, error)
	Label(ctx context.Context, credential string, shipment Shipment, format string) (Label, error)
	Cancel(ctx context.Context, credential string, shipment Shipment, request CancelRequest) (ProviderCancelResult, error)
}

type QuoteAdapter interface {
	Quote(ctx context.Context, credential string, request QuoteRequest) ([]ProviderQuote, error)
}

type Repository interface {
	SaveQuotes(ctx context.Context, tenantID string, quotes []Quote) error
	GetQuote(ctx context.Context, tenantID, quoteID string) (Quote, error)
	ReserveCreate(ctx context.Context, input ReserveCreateInput) (Shipment, bool, error)
	CompleteCreate(ctx context.Context, tenantID, shipmentID string, result ProviderCreateResult) (Shipment, error)
	FailCreate(ctx context.Context, tenantID, shipmentID, providerStatus string) error
	Get(ctx context.Context, tenantID, shipmentID string) (Shipment, error)
	ReserveOperation(ctx context.Context, input ReserveOperationInput) (bool, error)
	CompletePickup(ctx context.Context, tenantID, shipmentID, operationKey string, result ProviderPickupResult) (Shipment, error)
	CompleteCancel(ctx context.Context, tenantID, shipmentID, operationKey string, result ProviderCancelResult) (Shipment, error)
	FailOperation(ctx context.Context, tenantID, shipmentID, operationType, operationKey, providerStatus string) error
	History(ctx context.Context, tenantID, shipmentID string) ([]HistoryEvent, error)
	GetLabel(ctx context.Context, tenantID, shipmentID, format string) (Label, error)
	SaveLabel(ctx context.Context, tenantID, shipmentID string, label Label) error
}

type ReserveCreateInput struct {
	ID                 string
	TenantID           string
	ProviderCode       string
	MerchantReference  string
	QuoteID            string
	CourierCode        string
	ServiceCode        string
	DeliveryMode       string
	Fulfillment        string
	ShippingCost       int64
	Currency           string
	PackageWeightGrams int64
	IdempotencyKey     string
	RequestHash        []byte
	RequestCiphertext  []byte
}

type ReserveOperationInput struct {
	ID             string
	TenantID       string
	ShipmentID     string
	OperationType  string
	IdempotencyKey string
	RequestHash    []byte
}

type ProviderCatalog interface {
	ActiveProviderCode(ctx context.Context, tenantID string) (string, error)
}

type CredentialResolver interface {
	ResolveProviderCredentialForCapability(ctx context.Context, providerCode, capability string) (secret, alias string, dailyLimit int64, err error)
}
