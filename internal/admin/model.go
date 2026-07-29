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

type RateSnapshot struct {
	ID                   string     `json:"id"`
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

type Catalog struct {
	Couriers         []CatalogCourier         `json:"couriers"`
	RoundingProfiles []CatalogRoundingProfile `json:"rounding_profiles"`
}

type CatalogCourier struct {
	Code     string           `json:"code"`
	Name     string           `json:"name"`
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
