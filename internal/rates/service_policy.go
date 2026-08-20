package rates

import (
	"context"
	"strings"
	"time"
)

const (
	WeightBasisProvided = "provided"

	ReasonWeightBelowMinimum = "WEIGHT_BELOW_MINIMUM"
	ReasonWeightAboveMaximum = "WEIGHT_ABOVE_MAXIMUM"
)

type ServicePolicy struct {
	CourierCode                string
	ServiceCode                string
	ServiceGroup               string
	WeightBasis                string
	MinimumAcceptedWeightGrams int64
	MinimumBillableWeightGrams *int64
	MaximumAcceptedWeightGrams *int64
	SourceType                 string
	SourceReference            string
	VerificationStatus         string
	VerifiedAt                 time.Time
}

type PolicyEvaluation struct {
	Eligible                   bool
	Reason                     string
	WeightBasis                string
	EvaluatedWeightGrams       int64
	MinimumAcceptedWeightGrams int64
	MinimumBillableWeightGrams *int64
	MaximumAcceptedWeightGrams *int64
	SourceType                 string
	SourceReference            string
	VerificationStatus         string
	VerifiedAt                 time.Time
}

type ServicePolicyRepository interface {
	FindActiveServicePolicies(
		ctx context.Context,
		courierCodes []string,
	) ([]ServicePolicy, error)
}

type policySet struct {
	byService map[string]ServicePolicy
	byGroup   map[string]ServicePolicy
}

func newPolicySet(policies []ServicePolicy) policySet {
	set := policySet{
		byService: make(map[string]ServicePolicy),
		byGroup:   make(map[string]ServicePolicy),
	}
	for _, policy := range policies {
		policy.CourierCode = strings.ToLower(strings.TrimSpace(policy.CourierCode))
		policy.ServiceCode = strings.ToUpper(strings.TrimSpace(policy.ServiceCode))
		policy.ServiceGroup = strings.ToLower(strings.TrimSpace(policy.ServiceGroup))
		if policy.CourierCode != "" && policy.ServiceCode != "" {
			set.byService[policy.CourierCode+":"+policy.ServiceCode] = policy
			continue
		}
		if policy.ServiceGroup != "" {
			set.byGroup[policy.ServiceGroup] = policy
		}
	}
	return set
}

func (set policySet) match(card RateCard) (ServicePolicy, bool) {
	courierCode := strings.ToLower(strings.TrimSpace(card.CourierCode))
	serviceCodes := []string{card.CanonicalServiceCode, card.ServiceCode}
	for _, serviceCode := range serviceCodes {
		key := courierCode + ":" + strings.ToUpper(strings.TrimSpace(serviceCode))
		if policy, ok := set.byService[key]; ok {
			return policy, true
		}
	}
	policy, ok := set.byGroup[strings.ToLower(strings.TrimSpace(card.ServiceGroup))]
	return policy, ok
}

func evaluateServicePolicy(request Request, policy ServicePolicy) PolicyEvaluation {
	evaluation := PolicyEvaluation{
		Eligible:                   true,
		WeightBasis:                policy.WeightBasis,
		EvaluatedWeightGrams:       request.ActualWeightGrams,
		MinimumAcceptedWeightGrams: policy.MinimumAcceptedWeightGrams,
		MinimumBillableWeightGrams: policy.MinimumBillableWeightGrams,
		MaximumAcceptedWeightGrams: policy.MaximumAcceptedWeightGrams,
		SourceType:                 policy.SourceType,
		SourceReference:            policy.SourceReference,
		VerificationStatus:         policy.VerificationStatus,
		VerifiedAt:                 policy.VerifiedAt,
	}
	if evaluation.WeightBasis == "" {
		evaluation.WeightBasis = WeightBasisProvided
	}

	if evaluation.EvaluatedWeightGrams < policy.MinimumAcceptedWeightGrams {
		evaluation.Eligible = false
		evaluation.Reason = ReasonWeightBelowMinimum
		return evaluation
	}
	if policy.MaximumAcceptedWeightGrams != nil &&
		evaluation.EvaluatedWeightGrams > *policy.MaximumAcceptedWeightGrams {
		evaluation.Eligible = false
		evaluation.Reason = ReasonWeightAboveMaximum
		return evaluation
	}
	return evaluation
}

func applyPolicyToRateCard(card RateCard, policy ServicePolicy) RateCard {
	if policy.MinimumBillableWeightGrams != nil &&
		*policy.MinimumBillableWeightGrams > card.MinimumWeightGrams {
		card.MinimumWeightGrams = *policy.MinimumBillableWeightGrams
	}
	if policy.MaximumAcceptedWeightGrams != nil &&
		(card.MaximumWeightGrams == nil || *policy.MaximumAcceptedWeightGrams < *card.MaximumWeightGrams) {
		maximum := *policy.MaximumAcceptedWeightGrams
		card.MaximumWeightGrams = &maximum
	}
	return card
}

func attachPolicyEvaluation(result Result, evaluation PolicyEvaluation) Result {
	result.Eligibility = &evaluation
	if evaluation.MinimumBillableWeightGrams != nil {
		minimum := *evaluation.MinimumBillableWeightGrams
		result.Weight.MinimumBillableGrams = minimum
		result.Weight.MinimumGrams = max(result.Weight.MinimumGrams, minimum)
		result.Weight.BillingGrams = max(result.Weight.BillingGrams, minimum)
	}
	result.Weight.MinimumAcceptedGrams = evaluation.MinimumAcceptedWeightGrams
	result.Weight.MaximumAcceptedGrams = evaluation.MaximumAcceptedWeightGrams
	return result
}
