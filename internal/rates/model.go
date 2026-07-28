package rates

import "time"

type Dimensions struct {
	LengthCM int64
	WidthCM  int64
	HeightCM int64
}

type Request struct {
	Origin            string
	Destination       string
	ActualWeightGrams int64
	Couriers          []string
	Dimensions        *Dimensions
	ItemValue         int64
	IncludeUnverified bool
}

type RateCard struct {
	ID                     string
	CourierCode            string
	CourierName            string
	ServiceCode            string
	ServiceName            string
	ServiceType            string
	PricingModel           string
	Currency               string
	BaseWeightGrams        int64
	BasePrice              int64
	RatePerIncrement       int64
	MinimumWeightGrams     int64
	MaximumWeightGrams     *int64
	WeightIncrementGrams   int64
	VolumetricDivisor      *int64
	RoundingProfileCode    string
	RoundingMode           string
	RoundingIncrementGrams int64
	RoundingThresholdGrams *int64
	ETDMinDays             *int
	ETDMaxDays             *int
	VerificationStatus     string
	SourceProvider         string
	SourceReference        string
	EffectiveFrom          time.Time
	FetchedAt              time.Time
	ExpiresAt              *time.Time
}

type WeightBreakdown struct {
	ActualGrams     int64
	VolumetricGrams int64
	ChargeableGrams int64
	RoundedGrams    int64
	BillingGrams    int64
	MinimumGrams    int64
	RoundingProfile string
}

type CostBreakdown struct {
	Shipping  int64
	Surcharge int64
	Insurance int64
	Tax       int64
	Total     int64
}

type Result struct {
	Card       RateCard
	Weight     WeightBreakdown
	Cost       CostBreakdown
	SourceType string
}
