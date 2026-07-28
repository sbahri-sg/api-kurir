package rates

import (
	"errors"
	"fmt"
	"math"
)

var (
	ErrInvalidShipment    = errors.New("invalid shipment")
	ErrInvalidRateCard    = errors.New("invalid rate card")
	ErrUnsupportedPricing = errors.New("unsupported pricing model")
	ErrWeightExceeded     = errors.New("maximum service weight exceeded")
)

func Calculate(request Request, card RateCard) (Result, error) {
	if request.ActualWeightGrams <= 0 {
		return Result{}, fmt.Errorf("%w: actual weight must be positive", ErrInvalidShipment)
	}
	if err := validateCard(card); err != nil {
		return Result{}, err
	}

	volumetric, err := volumetricWeight(request.Dimensions, card.VolumetricDivisor)
	if err != nil {
		return Result{}, err
	}
	chargeable := max(request.ActualWeightGrams, volumetric)
	if card.MaximumWeightGrams != nil && chargeable > *card.MaximumWeightGrams {
		return Result{}, ErrWeightExceeded
	}

	minimumApplied := max(chargeable, card.MinimumWeightGrams)
	rounded, err := roundWeight(
		minimumApplied,
		card.RoundingIncrementGrams,
		card.RoundingMode,
		card.RoundingThresholdGrams,
	)
	if err != nil {
		return Result{}, err
	}
	billing := max(rounded, card.MinimumWeightGrams)

	shipping, err := calculatePrice(billing, card)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Card:       card,
		SourceType: "local_rate_card",
		Weight: WeightBreakdown{
			ActualGrams:     request.ActualWeightGrams,
			VolumetricGrams: volumetric,
			ChargeableGrams: chargeable,
			RoundedGrams:    rounded,
			BillingGrams:    billing,
			MinimumGrams:    card.MinimumWeightGrams,
			RoundingProfile: card.RoundingProfileCode,
		},
		Cost: CostBreakdown{
			Shipping: shipping,
			Total:    shipping,
		},
	}, nil
}

func validateCard(card RateCard) error {
	if card.WeightIncrementGrams <= 0 {
		return fmt.Errorf("%w: weight increment must be positive", ErrInvalidRateCard)
	}
	if card.RoundingIncrementGrams <= 0 {
		return fmt.Errorf("%w: rounding increment must be positive", ErrInvalidRateCard)
	}
	if card.MinimumWeightGrams < 0 || card.BasePrice < 0 || card.RatePerIncrement < 0 {
		return fmt.Errorf("%w: weight and price cannot be negative", ErrInvalidRateCard)
	}
	return nil
}

func volumetricWeight(dimensions *Dimensions, divisor *int64) (int64, error) {
	if dimensions == nil {
		return 0, nil
	}
	if dimensions.LengthCM <= 0 || dimensions.WidthCM <= 0 || dimensions.HeightCM <= 0 {
		return 0, fmt.Errorf("%w: dimensions must be positive", ErrInvalidShipment)
	}
	if divisor == nil || *divisor <= 0 {
		return 0, fmt.Errorf("%w: volumetric divisor is not configured", ErrInvalidRateCard)
	}

	if dimensions.LengthCM > math.MaxInt64/dimensions.WidthCM {
		return 0, fmt.Errorf("%w: dimensions overflow", ErrInvalidShipment)
	}
	volume := dimensions.LengthCM * dimensions.WidthCM
	if volume > math.MaxInt64/dimensions.HeightCM {
		return 0, fmt.Errorf("%w: dimensions overflow", ErrInvalidShipment)
	}
	volume *= dimensions.HeightCM
	if volume > math.MaxInt64/1000 {
		return 0, fmt.Errorf("%w: dimensions overflow", ErrInvalidShipment)
	}
	return ceilDiv(volume*1000, *divisor), nil
}

func roundWeight(weight, increment int64, mode string, threshold *int64) (int64, error) {
	if increment <= 0 {
		return 0, fmt.Errorf("%w: rounding increment must be positive", ErrInvalidRateCard)
	}
	remainder := weight % increment
	if remainder == 0 || mode == "none" {
		return weight, nil
	}

	switch mode {
	case "ceil":
		return (weight/increment + 1) * increment, nil
	case "floor":
		return (weight / increment) * increment, nil
	case "threshold":
		if threshold == nil || *threshold < 0 || *threshold > increment {
			return 0, fmt.Errorf("%w: invalid rounding threshold", ErrInvalidRateCard)
		}
		if remainder < *threshold {
			return (weight / increment) * increment, nil
		}
		return (weight/increment + 1) * increment, nil
	default:
		return 0, fmt.Errorf("%w: unknown rounding mode %q", ErrInvalidRateCard, mode)
	}
}

func calculatePrice(billing int64, card RateCard) (int64, error) {
	switch card.PricingModel {
	case "flat":
		return card.BasePrice, nil
	case "per_kg":
		units := ceilDiv(billing, card.WeightIncrementGrams)
		incremental, err := checkedMultiply(units, card.RatePerIncrement)
		if err != nil {
			return 0, err
		}
		return checkedAdd(card.BasePrice, incremental)
	case "base_plus_increment", "minimum_then_per_kg":
		if billing <= card.BaseWeightGrams {
			return card.BasePrice, nil
		}
		units := ceilDiv(billing-card.BaseWeightGrams, card.WeightIncrementGrams)
		incremental, err := checkedMultiply(units, card.RatePerIncrement)
		if err != nil {
			return 0, err
		}
		return checkedAdd(card.BasePrice, incremental)
	default:
		return 0, fmt.Errorf("%w: %s", ErrUnsupportedPricing, card.PricingModel)
	}
}

func ceilDiv(value, divisor int64) int64 {
	if value == 0 {
		return 0
	}
	return 1 + (value-1)/divisor
}

func checkedMultiply(a, b int64) (int64, error) {
	if a != 0 && b > math.MaxInt64/a {
		return 0, fmt.Errorf("%w: price overflow", ErrInvalidRateCard)
	}
	return a * b, nil
}

func checkedAdd(a, b int64) (int64, error) {
	if b > math.MaxInt64-a {
		return 0, fmt.Errorf("%w: price overflow", ErrInvalidRateCard)
	}
	return a + b, nil
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
