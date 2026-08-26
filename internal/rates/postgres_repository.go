package rates

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) FindActiveCourierPresentations(
	ctx context.Context,
	courierCodes []string,
) (map[string]CourierPresentation, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			c.code,
			c.name,
			c.logo_url,
			coalesce(service.code, ''),
			coalesce(service.name, '')
		FROM couriers c
		LEFT JOIN courier_services service
		  ON service.courier_id = c.id
		 AND service.active
		WHERE c.active
		  AND c.code = ANY($1::text[])
		ORDER BY c.code, service.code
	`, courierCodes)
	if err != nil {
		return nil, fmt.Errorf("query active courier presentations: %w", err)
	}
	defer rows.Close()

	presentations := make(map[string]CourierPresentation, len(courierCodes))
	for rows.Next() {
		var code, name, logo, serviceCode, serviceName string
		if err := rows.Scan(&code, &name, &logo, &serviceCode, &serviceName); err != nil {
			return nil, fmt.Errorf("scan active courier presentation: %w", err)
		}
		presentation := presentations[code]
		presentation.Name = name
		presentation.Logo = logo
		if presentation.ServiceNames == nil {
			presentation.ServiceNames = make(map[string]string)
		}
		if serviceCode != "" && serviceName != "" {
			presentation.ServiceNames[serviceCode] = serviceName
		}
		presentations[code] = presentation
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active courier presentations: %w", err)
	}
	return presentations, nil
}

func (r *PostgresRepository) FindActiveCourierLogos(
	ctx context.Context,
	courierCodes []string,
) (map[string]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.code, c.logo_url
		FROM couriers c
		WHERE c.active
		  AND c.code = ANY($1::text[])
	`, courierCodes)
	if err != nil {
		return nil, fmt.Errorf("query active courier logos: %w", err)
	}
	defer rows.Close()

	logos := make(map[string]string, len(courierCodes))
	for rows.Next() {
		var code, logo string
		if err := rows.Scan(&code, &logo); err != nil {
			return nil, fmt.Errorf("scan active courier logo: %w", err)
		}
		logos[code] = logo
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active courier logos: %w", err)
	}
	return logos, nil
}

func (r *PostgresRepository) FindActiveServicePolicies(
	ctx context.Context,
	courierCodes []string,
) ([]ServicePolicy, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			coalesce(c.code, ''),
			coalesce(cs.code, ''),
			coalesce(policy.service_group, ''),
			policy.weight_basis,
			policy.minimum_accepted_weight_grams,
			policy.minimum_billable_weight_grams,
			policy.maximum_accepted_weight_grams,
			policy.source_type,
			coalesce(policy.source_reference, ''),
			policy.verification_status,
			policy.verified_at
		FROM courier_service_shipping_policies policy
		LEFT JOIN courier_services cs ON cs.id = policy.courier_service_id
		LEFT JOIN couriers c ON c.id = cs.courier_id
		WHERE policy.active
		  AND policy.effective_from <= now()
		  AND (policy.effective_until IS NULL OR policy.effective_until > now())
		  AND (policy.courier_service_id IS NULL OR c.code = ANY($1::text[]))
		ORDER BY policy.courier_service_id NULLS LAST, policy.effective_from DESC
	`, courierCodes)
	if err != nil {
		return nil, fmt.Errorf("query active service shipping policies: %w", err)
	}
	defer rows.Close()

	policies := make([]ServicePolicy, 0)
	for rows.Next() {
		var policy ServicePolicy
		if err := rows.Scan(
			&policy.CourierCode,
			&policy.ServiceCode,
			&policy.ServiceGroup,
			&policy.WeightBasis,
			&policy.MinimumAcceptedWeightGrams,
			&policy.MinimumBillableWeightGrams,
			&policy.MaximumAcceptedWeightGrams,
			&policy.SourceType,
			&policy.SourceReference,
			&policy.VerificationStatus,
			&policy.VerifiedAt,
		); err != nil {
			return nil, fmt.Errorf("scan active service shipping policy: %w", err)
		}
		policies = append(policies, policy)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active service shipping policies: %w", err)
	}
	return policies, nil
}

func (r *PostgresRepository) FindActiveRateCards(
	ctx context.Context,
	request Request,
) ([]RateCard, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT ON (c.code, cs.code)
			rc.id::text,
			c.code,
			c.name,
			c.logo_url,
			cs.code,
			cs.name,
			cs.code,
			cs.service_group,
			cs.service_type,
			''::text,
			rc.pricing_model,
			rc.currency,
			rc.base_weight_grams,
			rc.base_price,
			rc.rate_per_increment,
			rc.minimum_weight_grams,
			rc.maximum_weight_grams,
			rc.weight_increment_grams,
			rc.volumetric_divisor,
			rp.code,
			rp.rounding_mode,
			rp.increment_grams,
			rp.threshold_grams,
			rc.etd_min_days,
			rc.etd_max_days,
			rc.verification_status,
			rc.source_provider,
			coalesce(rc.source_reference, ''),
			rc.effective_from,
			rc.fetched_at,
			rc.expires_at
		FROM rate_cards rc
		JOIN locations origin ON origin.id = rc.origin_location_id
		JOIN locations destination ON destination.id = rc.destination_location_id
		JOIN courier_services cs ON cs.id = rc.courier_service_id
		JOIN couriers c ON c.id = cs.courier_id
		JOIN rounding_profiles rp ON rp.id = rc.rounding_profile_id
		WHERE origin.public_id = $1
		  AND destination.public_id = $2
		  AND c.code = ANY($3::text[])
		  AND c.active
		  AND cs.active
		  AND rc.effective_from <= now()
		  AND (rc.effective_until IS NULL OR rc.effective_until > now())
		  AND (rc.expires_at IS NULL OR rc.expires_at > now())
		  AND rc.verification_status <> 'deprecated'
		  AND ($4 OR rc.verification_status <> 'needs_contract_confirmation')
		ORDER BY c.code, cs.code, rc.effective_from DESC
	`,
		request.Origin,
		request.Destination,
		request.Couriers,
		request.IncludeUnverified,
	)
	if err != nil {
		return nil, fmt.Errorf("query active rate cards: %w", err)
	}
	defer rows.Close()

	cards := make([]RateCard, 0)
	for rows.Next() {
		var card RateCard
		if err := rows.Scan(
			&card.ID,
			&card.CourierCode,
			&card.CourierName,
			&card.CourierLogo,
			&card.ServiceCode,
			&card.ServiceName,
			&card.CanonicalServiceCode,
			&card.ServiceGroup,
			&card.ServiceType,
			&card.ServiceVariantCode,
			&card.PricingModel,
			&card.Currency,
			&card.BaseWeightGrams,
			&card.BasePrice,
			&card.RatePerIncrement,
			&card.MinimumWeightGrams,
			&card.MaximumWeightGrams,
			&card.WeightIncrementGrams,
			&card.VolumetricDivisor,
			&card.RoundingProfileCode,
			&card.RoundingMode,
			&card.RoundingIncrementGrams,
			&card.RoundingThresholdGrams,
			&card.ETDMinDays,
			&card.ETDMaxDays,
			&card.VerificationStatus,
			&card.SourceProvider,
			&card.SourceReference,
			&card.EffectiveFrom,
			&card.FetchedAt,
			&card.ExpiresAt,
		); err != nil {
			return nil, fmt.Errorf("scan active rate card: %w", err)
		}
		cards = append(cards, card)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active rate cards: %w", err)
	}
	return cards, nil
}
