package admin

import "time"

var (
	ValidPricingModels = map[string]struct{}{
		"flat": {}, "per_kg": {}, "base_plus_increment": {}, "minimum_then_per_kg": {},
	}
	ValidVerificationStatuses = map[string]struct{}{
		"official_public": {}, "official_contract": {}, "observed": {},
		"needs_contract_confirmation": {},
	}
)

type Overview struct {
	ActiveRateCards     int64 `json:"active_rate_cards"`
	OfficialLocations   int64 `json:"official_locations"`
	LocationMappings    int64 `json:"location_mappings"`
	FreshQuoteSnapshots int64 `json:"fresh_quote_snapshots"`
	QuotaUsedToday      int64 `json:"quota_used_today"`
	QuotaLimitToday     int64 `json:"quota_limit_today"`
	TrackingPendingJobs int64 `json:"tracking_pending_jobs"`
	TrackingShipments   int64 `json:"tracking_shipments"`
}

type TrackingOperationFilter struct {
	Search           string
	CourierCode      string
	ValidationStatus string
	QueueStatus      string
	Limit            int
	Offset           int
}

type TrackingOperationSummary struct {
	Total   int64 `json:"total"`
	Pending int64 `json:"pending"`
	Running int64 `json:"running"`
	Failed  int64 `json:"failed"`
	Invalid int64 `json:"invalid"`
	Final   int64 `json:"final"`
}

type TrackingOperation struct {
	ID                   string     `json:"id"`
	TenantID             string     `json:"tenant_id,omitempty"`
	OrderReference       string     `json:"order_id,omitempty"`
	FulfillmentReference string     `json:"fulfillment_id,omitempty"`
	SubscriptionRevision int        `json:"subscription_revision,omitempty"`
	RevisionHistoryCount int        `json:"revision_history_count"`
	CourierCode          string     `json:"courier"`
	WaybillMasked        string     `json:"waybill"`
	WaybillCiphertext    []byte     `json:"-"`
	ValidationStatus     string     `json:"validation_status"`
	NormalizedStatus     string     `json:"status"`
	StatusLabel          string     `json:"status_label"`
	ProviderCode         string     `json:"provider"`
	ProviderFetchedAt    *time.Time `json:"provider_fetched_at"`
	NextRefreshAt        *time.Time `json:"next_refresh_at"`
	IsFinal              bool       `json:"is_final"`
	LastErrorCode        string     `json:"last_error_code,omitempty"`
	ProviderHitCount     int        `json:"provider_hit_count"`
	ProviderHitLimit     int        `json:"provider_hit_limit"`
	QueueStatus          string     `json:"queue_status"`
	JobAttemptCount      int        `json:"job_attempt_count"`
	JobMaxAttempts       int        `json:"job_max_attempts"`
	JobAvailableAt       *time.Time `json:"job_available_at"`
	JobLockedAt          *time.Time `json:"job_locked_at"`
	JobLockedBy          string     `json:"job_locked_by,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type TrackingOperationPage struct {
	Items   []TrackingOperation      `json:"items"`
	Total   int64                    `json:"total"`
	Summary TrackingOperationSummary `json:"summary"`
}

type RateSnapshot struct {
	ID                   string     `json:"id"`
	TenantID             string     `json:"tenant_id,omitempty"`
	ProviderCredentialID string     `json:"provider_credential_id,omitempty"`
	OriginPublicID       string     `json:"origin_public_id"`
	OriginLabel          string     `json:"origin_label"`
	DestinationPublicID  string     `json:"destination_public_id"`
	DestinationLabel     string     `json:"destination_label"`
	CourierCode          string     `json:"courier_code"`
	CourierName          string     `json:"courier_name"`
	ServiceCode          string     `json:"service_code"`
	ServiceName          string     `json:"service_name"`
	Description          string     `json:"description"`
	RequestedWeightGrams int64      `json:"requested_weight_grams"`
	ReturnedCost         int64      `json:"returned_cost"`
	ETDMinDays           *int       `json:"etd_min_days"`
	ETDMaxDays           *int       `json:"etd_max_days"`
	ProviderCode         string     `json:"provider_code"`
	VerificationStatus   string     `json:"verification_status"`
	SourceType           string     `json:"source_type"`
	FetchedAt            time.Time  `json:"fetched_at"`
	ExpiresAt            *time.Time `json:"expires_at"`
	Fresh                bool       `json:"fresh"`
}

type RateCard struct {
	ID                     string     `json:"id"`
	OriginPublicID         string     `json:"origin_public_id"`
	OriginLabel            string     `json:"origin_label"`
	DestinationPublicID    string     `json:"destination_public_id"`
	DestinationLabel       string     `json:"destination_label"`
	CourierCode            string     `json:"courier_code"`
	CourierName            string     `json:"courier_name"`
	ServiceCode            string     `json:"service_code"`
	ServiceName            string     `json:"service_name"`
	ServiceType            string     `json:"service_type"`
	PricingModel           string     `json:"pricing_model"`
	BaseWeightGrams        int64      `json:"base_weight_grams"`
	BasePrice              int64      `json:"base_price"`
	RatePerIncrement       int64      `json:"rate_per_increment"`
	MinimumWeightGrams     int64      `json:"minimum_weight_grams"`
	MaximumWeightGrams     *int64     `json:"maximum_weight_grams"`
	WeightIncrementGrams   int64      `json:"weight_increment_grams"`
	VolumetricDivisor      *int64     `json:"volumetric_divisor"`
	RoundingProfileCode    string     `json:"rounding_profile_code"`
	RoundingMode           string     `json:"rounding_mode"`
	RoundingIncrementGrams int64      `json:"rounding_increment_grams"`
	RoundingThresholdGrams *int64     `json:"rounding_threshold_grams"`
	ETDMinDays             *int       `json:"etd_min_days"`
	ETDMaxDays             *int       `json:"etd_max_days"`
	VerificationStatus     string     `json:"verification_status"`
	SourceProvider         string     `json:"source_provider"`
	SourceReference        string     `json:"source_reference"`
	EffectiveFrom          time.Time  `json:"effective_from"`
	EffectiveUntil         *time.Time `json:"effective_until"`
	ExpiresAt              *time.Time `json:"expires_at"`
}

type RateCardInput struct {
	OriginPublicID       string
	DestinationPublicID  string
	CourierCode          string
	ServiceCode          string
	PricingModel         string
	BaseWeightGrams      int64
	BasePrice            int64
	RatePerIncrement     int64
	MinimumWeightGrams   int64
	MaximumWeightGrams   *int64
	WeightIncrementGrams int64
	VolumetricDivisor    *int64
	RoundingProfileCode  string
	ETDMinDays           *int
	ETDMaxDays           *int
	VerificationStatus   string
	SourceProvider       string
	SourceReference      string
	EffectiveFrom        time.Time
	ExpiresAt            *time.Time
}

type LocationMapping struct {
	ID                   string     `json:"id"`
	LocationPublicID     string     `json:"location_public_id"`
	LocationLabel        string     `json:"location_label"`
	ProviderCode         string     `json:"provider_code"`
	ProviderLocationID   string     `json:"provider_location_id"`
	ProviderLocationName string     `json:"provider_location_name"`
	Granularity          string     `json:"granularity"`
	VerifiedAt           *time.Time `json:"verified_at"`
	Active               bool       `json:"active"`
}

type LocationMappingInput struct {
	LocationPublicID     string
	ProviderCode         string
	ProviderLocationID   string
	ProviderLocationName string
	Granularity          string
}

type ProviderQuota struct {
	TenantID        string    `json:"tenant_id,omitempty"`
	ProviderCode    string    `json:"provider_code"`
	CredentialAlias string    `json:"credential_alias"`
	QuotaDate       string    `json:"quota_date"`
	DailyLimit      int64     `json:"daily_limit"`
	UsedCount       int64     `json:"used_count"`
	ReservedCount   int64     `json:"reserved_count"`
	RemainingCount  int64     `json:"remaining_count"`
	UsagePercentage float64   `json:"usage_percentage"`
	HealthStatus    string    `json:"health_status"`
	ResetAt         time.Time `json:"reset_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type ShippingProvider struct {
	Code                   string    `json:"code"`
	Name                   string    `json:"name"`
	Logo                   string    `json:"logo"`
	Description            string    `json:"description"`
	BuiltIn                bool      `json:"built_in"`
	RequiresCredential     bool      `json:"requires_credential"`
	Available              bool      `json:"available"`
	DisplayOrder           int       `json:"display_order"`
	InstalledMerchantCount int64     `json:"installed_merchant_count"`
	ActiveMerchantCount    int64     `json:"active_merchant_count"`
	CredentialCount        int64     `json:"credential_count"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type ShippingProviderCreateInput struct {
	Code         string
	Name         string
	Logo         string
	Description  string
	DisplayOrder int
}

type ShippingProviderUpdateInput struct {
	Name         string
	Logo         string
	Description  string
	Available    bool
	DisplayOrder int
}

type Catalog struct {
	Couriers         []CatalogCourier         `json:"couriers"`
	RoundingProfiles []CatalogRoundingProfile `json:"rounding_profiles"`
}

type CatalogCourier struct {
	Code     string           `json:"code"`
	Name     string           `json:"name"`
	Logo     string           `json:"logo"`
	Services []CatalogService `json:"services"`
}

type CatalogService struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Group       string `json:"group"`
	ServiceType string `json:"service_type"`
}

type CatalogRoundingProfile struct {
	Code           string `json:"code"`
	RoundingMode   string `json:"rounding_mode"`
	IncrementGrams int64  `json:"increment_grams"`
	ThresholdGrams *int64 `json:"threshold_grams"`
}
