package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Overview(ctx context.Context) (Overview, error) {
	var result Overview
	err := r.pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM rate_cards
			 WHERE effective_from <= now()
			   AND (effective_until IS NULL OR effective_until > now())
			   AND (expires_at IS NULL OR expires_at > now())
			   AND verification_status <> 'deprecated'),
			(SELECT count(*) FROM locations
			 WHERE active AND official_region_code IS NOT NULL),
			(SELECT count(*) FROM provider_location_mappings WHERE active),
			(SELECT count(*) FROM rate_snapshots
			 WHERE source_type = 'provider_quote' AND expires_at > now()),
			(SELECT coalesce(sum(used_count), 0) FROM provider_quota_ledger
			 WHERE quota_date = (now() AT TIME ZONE 'Asia/Jakarta')::date),
			(SELECT coalesce(sum(daily_limit), 0) FROM provider_quota_ledger
			 WHERE quota_date = (now() AT TIME ZONE 'Asia/Jakarta')::date),
			(SELECT count(*) FROM tracking_refresh_jobs WHERE status IN ('pending', 'running')),
			(SELECT count(*) FROM tracking_shipments)
	`).Scan(
		&result.ActiveRateCards,
		&result.OfficialLocations,
		&result.LocationMappings,
		&result.FreshQuoteSnapshots,
		&result.QuotaUsedToday,
		&result.QuotaLimitToday,
		&result.TrackingPendingJobs,
		&result.TrackingShipments,
	)
	if err != nil {
		return Overview{}, fmt.Errorf("query admin overview: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) ListTrackingOperations(
	ctx context.Context,
	filter TrackingOperationFilter,
) (TrackingOperationPage, error) {
	filter.Search = strings.TrimSpace(filter.Search)
	filter.CourierCode = strings.ToLower(strings.TrimSpace(filter.CourierCode))
	filter.ValidationStatus = strings.ToLower(strings.TrimSpace(filter.ValidationStatus))
	filter.QueueStatus = strings.ToLower(strings.TrimSpace(filter.QueueStatus))

	var page TrackingOperationPage
	err := r.pool.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE job.status = 'pending'),
		       count(*) FILTER (WHERE job.status = 'running'),
		       count(*) FILTER (WHERE job.status = 'dead'),
		       count(*) FILTER (WHERE shipment.validation_status = 'invalid'),
		       count(*) FILTER (WHERE shipment.is_final)
		FROM tracking_shipments shipment
		LEFT JOIN LATERAL (
			SELECT status
			FROM tracking_refresh_jobs
			WHERE shipment_id = shipment.id
			ORDER BY CASE status WHEN 'running' THEN 0 WHEN 'pending' THEN 1 WHEN 'dead' THEN 2 ELSE 3 END,
			         updated_at DESC
			LIMIT 1
		) job ON true
	`).Scan(&page.Summary.Total, &page.Summary.Pending, &page.Summary.Running,
		&page.Summary.Failed, &page.Summary.Invalid, &page.Summary.Final)
	if err != nil {
		return TrackingOperationPage{}, fmt.Errorf("summarize tracking operations: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		WITH operations AS (
			SELECT shipment.id::text,
			       shipment.tenant_id,
			       coalesce(subscription.order_reference, '') AS order_reference,
			       coalesce(subscription.fulfillment_reference, '') AS fulfillment_reference,
			       coalesce(subscription.revision, 0) AS subscription_revision,
			       coalesce(revisions.history_count, 0) AS revision_history_count,
			       shipment.courier_code,
			       coalesce(shipment.waybill, shipment.waybill_masked) AS waybill_masked,
			       shipment.waybill_ciphertext,
			       shipment.validation_status,
			       shipment.normalized_status,
			       coalesce(shipment.status_label, '') AS status_label,
			       coalesce(shipment.provider_code, '') AS provider_code,
			       shipment.provider_fetched_at,
			       shipment.next_refresh_at,
			       shipment.is_final,
			       coalesce(shipment.last_error_code, '') AS last_error_code,
			       shipment.provider_hit_count,
			       shipment.provider_hit_limit,
			       CASE
			         WHEN shipment.is_final THEN 'final'
			         WHEN job.status IS NULL THEN 'idle'
			         ELSE job.status
			       END AS queue_status,
			       coalesce(job.attempt_count, 0) AS job_attempt_count,
			       coalesce(job.max_attempts, 0) AS job_max_attempts,
			       job.available_at,
			       job.locked_at,
			       coalesce(job.locked_by, '') AS job_locked_by,
			       shipment.created_at,
			       shipment.updated_at
			FROM tracking_shipments shipment
			LEFT JOIN LATERAL (
				SELECT *
				FROM tracking_subscriptions
				WHERE shipment_id = shipment.id AND active
				ORDER BY updated_at DESC
				LIMIT 1
			) subscription ON true
			LEFT JOIN LATERAL (
				SELECT count(*)::integer AS history_count
				FROM tracking_subscription_revisions
				WHERE subscription_id = subscription.id
			) revisions ON true
			LEFT JOIN LATERAL (
				SELECT status, attempt_count, max_attempts, available_at,
				       locked_at, locked_by, updated_at
				FROM tracking_refresh_jobs
				WHERE shipment_id = shipment.id
				ORDER BY CASE status WHEN 'running' THEN 0 WHEN 'pending' THEN 1 WHEN 'dead' THEN 2 ELSE 3 END,
				         updated_at DESC
				LIMIT 1
			) job ON true
		)
		SELECT operations.*, count(*) OVER()
		FROM operations
		WHERE ($1 = '' OR lower(
			courier_code || ' ' || waybill_masked || ' ' || tenant_id || ' ' ||
			order_reference || ' ' || fulfillment_reference || ' ' || provider_code
		) LIKE '%' || lower($1) || '%')
		  AND ($2 = '' OR courier_code = $2)
		  AND ($3 = '' OR validation_status = $3)
		  AND ($4 = '' OR queue_status = $4)
		ORDER BY CASE queue_status WHEN 'running' THEN 0 WHEN 'pending' THEN 1 WHEN 'dead' THEN 2 ELSE 3 END,
		         updated_at DESC
		LIMIT $5 OFFSET $6
	`, filter.Search, filter.CourierCode, filter.ValidationStatus,
		filter.QueueStatus, filter.Limit, filter.Offset)
	if err != nil {
		return TrackingOperationPage{}, fmt.Errorf("list tracking operations: %w", err)
	}
	defer rows.Close()
	page.Items = make([]TrackingOperation, 0)
	for rows.Next() {
		var item TrackingOperation
		var total int64
		if err := rows.Scan(
			&item.ID, &item.TenantID, &item.OrderReference,
			&item.FulfillmentReference, &item.SubscriptionRevision,
			&item.RevisionHistoryCount, &item.CourierCode, &item.WaybillMasked,
			&item.WaybillCiphertext,
			&item.ValidationStatus, &item.NormalizedStatus, &item.StatusLabel,
			&item.ProviderCode, &item.ProviderFetchedAt, &item.NextRefreshAt,
			&item.IsFinal, &item.LastErrorCode, &item.ProviderHitCount,
			&item.ProviderHitLimit, &item.QueueStatus, &item.JobAttemptCount,
			&item.JobMaxAttempts, &item.JobAvailableAt, &item.JobLockedAt,
			&item.JobLockedBy, &item.CreatedAt, &item.UpdatedAt, &total,
		); err != nil {
			return TrackingOperationPage{}, fmt.Errorf("scan tracking operation: %w", err)
		}
		page.Total = total
		page.Items = append(page.Items, item)
	}
	if err := rows.Err(); err != nil {
		return TrackingOperationPage{}, fmt.Errorf("iterate tracking operations: %w", err)
	}
	return page, nil
}

// DeleteTrackingOperation permanently removes a tracking shipment and every
// dependent queue, snapshot history, subscription, revision, and webhook row.
// The foreign keys use ON DELETE CASCADE; the transaction keeps the audit trail
// even though the operational tracking data itself is removed.
func (r *PostgresRepository) DeleteTrackingOperation(
	ctx context.Context,
	id, actorAlias, requestID string,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete tracking operation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var auditDetails map[string]any
	err = tx.QueryRow(ctx, `
		SELECT jsonb_build_object(
			'courier', courier_code,
			'waybill_masked', waybill_masked,
			'validation_status', validation_status,
			'normalized_status', normalized_status,
			'provider', coalesce(provider_code, '')
		)
		FROM tracking_shipments
		WHERE id = $1::uuid
		FOR UPDATE
	`, id).Scan(&auditDetails)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("find tracking operation for deletion: %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM tracking_shipments WHERE id = $1::uuid`, id); err != nil {
		return fmt.Errorf("delete tracking operation: %w", err)
	}
	if err := insertAudit(
		ctx, tx, actorAlias, "delete_permanently", "tracking_shipment", id, requestID, auditDetails,
	); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delete tracking operation: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ListRateSnapshots(
	ctx context.Context,
	search string,
	limit, offset int,
) ([]RateSnapshot, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			rs.id::text,
			rs.tenant_id,
			rs.integration_id,
			origin.public_id,
			concat_ws(', ', origin.subdistrict, origin.district, origin.city, origin.province),
			destination.public_id,
			concat_ws(', ', destination.subdistrict, destination.district, destination.city, destination.province),
			rs.courier_code,
			coalesce(rs.courier_name, rs.courier_code),
			rs.service_code,
			coalesce(rs.service_name, rs.service_code),
			coalesce(rs.description, ''),
			rs.requested_weight_grams,
			rs.returned_cost,
			rs.etd_min_days,
			rs.etd_max_days,
			rs.provider_code,
			rs.verification_status,
			rs.source_type,
			rs.fetched_at,
			rs.expires_at,
			(rs.expires_at IS NOT NULL AND rs.expires_at > now())
		FROM rate_snapshots rs
		JOIN locations origin ON origin.id = rs.origin_location_id
		JOIN locations destination ON destination.id = rs.destination_location_id
		WHERE rs.source_type = 'provider_quote'
		  AND (
			$1 = '' OR
			lower(rs.courier_code || ' ' || rs.service_code || ' ' ||
				coalesce(rs.courier_name, '') || ' ' || coalesce(rs.service_name, '')) LIKE '%' || lower($1) || '%' OR
			origin.search_text LIKE '%' || lower($1) || '%' OR
			destination.search_text LIKE '%' || lower($1) || '%' OR
			lower(rs.provider_code) LIKE '%' || lower($1) || '%' OR
			lower(rs.tenant_id) LIKE '%' || lower($1) || '%'
		  )
		ORDER BY rs.fetched_at DESC, rs.courier_code, rs.service_code
		LIMIT $2 OFFSET $3
	`, strings.TrimSpace(search), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list provider rate snapshots: %w", err)
	}
	defer rows.Close()

	items := make([]RateSnapshot, 0)
	for rows.Next() {
		var item RateSnapshot
		if err := rows.Scan(
			&item.ID,
			&item.TenantID,
			&item.ProviderCredentialID,
			&item.OriginPublicID,
			&item.OriginLabel,
			&item.DestinationPublicID,
			&item.DestinationLabel,
			&item.CourierCode,
			&item.CourierName,
			&item.ServiceCode,
			&item.ServiceName,
			&item.Description,
			&item.RequestedWeightGrams,
			&item.ReturnedCost,
			&item.ETDMinDays,
			&item.ETDMaxDays,
			&item.ProviderCode,
			&item.VerificationStatus,
			&item.SourceType,
			&item.FetchedAt,
			&item.ExpiresAt,
			&item.Fresh,
		); err != nil {
			return nil, fmt.Errorf("scan provider rate snapshot: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate provider rate snapshots: %w", err)
	}
	return items, nil
}

func (r *PostgresRepository) ListRateCards(
	ctx context.Context,
	search string,
	limit, offset int,
) ([]RateCard, error) {
	rows, err := r.pool.Query(ctx, rateCardSelect+`
		WHERE (
			$1 = '' OR
			lower(c.code || ' ' || cs.code || ' ' || cs.name) LIKE '%' || lower($1) || '%' OR
			lower(concat_ws(' ', origin.subdistrict, origin.district, origin.city, origin.province)) LIKE '%' || lower($1) || '%' OR
			lower(concat_ws(' ', destination.subdistrict, destination.district, destination.city, destination.province)) LIKE '%' || lower($1) || '%'
		)
		ORDER BY rc.effective_from DESC, c.code, cs.code
		LIMIT $2 OFFSET $3
	`, strings.TrimSpace(search), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list admin rate cards: %w", err)
	}
	defer rows.Close()

	items := make([]RateCard, 0)
	for rows.Next() {
		item, err := scanRateCard(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admin rate cards: %w", err)
	}
	return items, nil
}

func (r *PostgresRepository) CreateRateCard(
	ctx context.Context,
	input RateCardInput,
	actorAlias, requestID string,
) (RateCard, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return RateCard{}, fmt.Errorf("begin create rate card: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var originID, destinationID, serviceID, profileID string
	err = tx.QueryRow(ctx, `
		SELECT origin.id::text, destination.id::text, cs.id::text, rp.id::text
		FROM locations origin
		CROSS JOIN locations destination
		CROSS JOIN courier_services cs
		JOIN couriers c ON c.id = cs.courier_id
		CROSS JOIN rounding_profiles rp
		WHERE origin.public_id = $1
		  AND destination.public_id = $2
		  AND c.code = $3
		  AND cs.code = $4
		  AND rp.code = $5
		  AND origin.active
		  AND destination.active
		  AND c.active
		  AND cs.active
		LIMIT 1
	`,
		input.OriginPublicID,
		input.DestinationPublicID,
		input.CourierCode,
		input.ServiceCode,
		input.RoundingProfileCode,
	).Scan(&originID, &destinationID, &serviceID, &profileID)
	if errors.Is(err, pgx.ErrNoRows) {
		return RateCard{}, ErrNotFound
	}
	if err != nil {
		return RateCard{}, fmt.Errorf("resolve rate card references: %w", err)
	}

	var nextEffective *time.Time
	err = tx.QueryRow(ctx, `
		SELECT min(effective_from)
		FROM rate_cards
		WHERE origin_location_id = $1::uuid
		  AND destination_location_id = $2::uuid
		  AND courier_service_id = $3::uuid
		  AND effective_from > $4
		  AND verification_status <> 'deprecated'
	`, originID, destinationID, serviceID, input.EffectiveFrom).Scan(&nextEffective)
	if err != nil {
		return RateCard{}, fmt.Errorf("find next rate card version: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE rate_cards
		SET effective_until = $4,
		    updated_at = now()
		WHERE origin_location_id = $1::uuid
		  AND destination_location_id = $2::uuid
		  AND courier_service_id = $3::uuid
		  AND effective_from < $4
		  AND (effective_until IS NULL OR effective_until > $4)
		  AND verification_status <> 'deprecated'
	`, originID, destinationID, serviceID, input.EffectiveFrom); err != nil {
		return RateCard{}, fmt.Errorf("close previous rate card version: %w", err)
	}

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO rate_cards (
			origin_location_id,
			destination_location_id,
			courier_service_id,
			pricing_model,
			base_weight_grams,
			base_price,
			rate_per_increment,
			minimum_weight_grams,
			maximum_weight_grams,
			weight_increment_grams,
			volumetric_divisor,
			rounding_profile_id,
			etd_min_days,
			etd_max_days,
			verification_status,
			source_provider,
			source_reference,
			effective_from,
			effective_until,
			fetched_at,
			expires_at
		)
		VALUES (
			$1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9, $10,
			$11, $12::uuid, $13, $14, $15, $16, $17, $18, $19, now(), $20
		)
		RETURNING id::text
	`,
		originID,
		destinationID,
		serviceID,
		input.PricingModel,
		input.BaseWeightGrams,
		input.BasePrice,
		input.RatePerIncrement,
		input.MinimumWeightGrams,
		input.MaximumWeightGrams,
		input.WeightIncrementGrams,
		input.VolumetricDivisor,
		profileID,
		input.ETDMinDays,
		input.ETDMaxDays,
		input.VerificationStatus,
		input.SourceProvider,
		nullIfBlank(input.SourceReference),
		input.EffectiveFrom,
		nextEffective,
		input.ExpiresAt,
	).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return RateCard{}, ErrConflict
		}
		return RateCard{}, fmt.Errorf("insert rate card: %w", err)
	}

	if err := insertAudit(
		ctx,
		tx,
		actorAlias,
		"create",
		"rate_card",
		id,
		requestID,
		map[string]any{
			"origin": input.OriginPublicID, "destination": input.DestinationPublicID,
			"courier": input.CourierCode, "service": input.ServiceCode,
		},
	); err != nil {
		return RateCard{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RateCard{}, fmt.Errorf("commit create rate card: %w", err)
	}
	return r.getRateCard(ctx, id)
}

func (r *PostgresRepository) DeprecateRateCard(
	ctx context.Context,
	id, actorAlias, requestID string,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin deprecate rate card: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE rate_cards
		SET verification_status = 'deprecated',
		    effective_until = CASE
		        WHEN effective_until IS NULL THEN greatest(now(), effective_from)
		        ELSE least(effective_until, greatest(now(), effective_from))
		    END,
		    updated_at = now()
		WHERE id = $1::uuid
		  AND verification_status <> 'deprecated'
	`, id)
	if err != nil {
		return fmt.Errorf("deprecate rate card: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := insertAudit(
		ctx, tx, actorAlias, "deprecate", "rate_card", id, requestID, nil,
	); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit deprecate rate card: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ListLocationMappings(
	ctx context.Context,
	search, providerCode string,
	limit, offset int,
) ([]LocationMapping, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			plm.id::text,
			l.public_id,
			concat_ws(', ', l.subdistrict, l.district, l.city, l.province),
			plm.provider_code,
			plm.provider_location_id,
			coalesce(plm.provider_location_name, ''),
			plm.granularity,
			plm.verified_at,
			plm.active
		FROM provider_location_mappings plm
		JOIN locations l ON l.id = plm.location_id
		WHERE ($1 = '' OR plm.provider_code = $1)
		  AND (
			$2 = '' OR
			l.search_text LIKE '%' || lower($2) || '%' OR
			lower(plm.provider_location_id) LIKE '%' || lower($2) || '%' OR
			lower(coalesce(plm.provider_location_name, '')) LIKE '%' || lower($2) || '%'
		  )
		ORDER BY plm.updated_at DESC
		LIMIT $3 OFFSET $4
	`, strings.ToLower(strings.TrimSpace(providerCode)), strings.TrimSpace(search), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list provider location mappings: %w", err)
	}
	defer rows.Close()

	items := make([]LocationMapping, 0)
	for rows.Next() {
		var item LocationMapping
		if err := rows.Scan(
			&item.ID,
			&item.LocationPublicID,
			&item.LocationLabel,
			&item.ProviderCode,
			&item.ProviderLocationID,
			&item.ProviderLocationName,
			&item.Granularity,
			&item.VerifiedAt,
			&item.Active,
		); err != nil {
			return nil, fmt.Errorf("scan provider location mapping: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate provider location mappings: %w", err)
	}
	return items, nil
}

func (r *PostgresRepository) UpsertLocationMapping(
	ctx context.Context,
	input LocationMappingInput,
	actorAlias, requestID string,
) (LocationMapping, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return LocationMapping{}, fmt.Errorf("begin upsert location mapping: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var locationID string
	err = tx.QueryRow(ctx, `
		SELECT id::text
		FROM locations
		WHERE public_id = $1 AND active
	`, input.LocationPublicID).Scan(&locationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return LocationMapping{}, ErrNotFound
	}
	if err != nil {
		return LocationMapping{}, fmt.Errorf("resolve local location: %w", err)
	}

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO provider_location_mappings (
			location_id,
			provider_code,
			provider_location_id,
			provider_location_name,
			granularity,
			verified_at,
			active
		)
		VALUES ($1::uuid, $2, $3, $4, $5, now(), true)
		ON CONFLICT (location_id, provider_code, granularity) DO UPDATE
		SET provider_location_id = EXCLUDED.provider_location_id,
		    provider_location_name = EXCLUDED.provider_location_name,
		    verified_at = now(),
		    active = true,
		    updated_at = now()
		RETURNING id::text
	`,
		locationID,
		input.ProviderCode,
		input.ProviderLocationID,
		nullIfBlank(input.ProviderLocationName),
		input.Granularity,
	).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return LocationMapping{}, ErrConflict
		}
		return LocationMapping{}, fmt.Errorf("upsert provider location mapping: %w", err)
	}
	if err := insertAudit(
		ctx,
		tx,
		actorAlias,
		"upsert",
		"location_mapping",
		id,
		requestID,
		map[string]any{
			"location": input.LocationPublicID, "provider": input.ProviderCode,
			"provider_location_id": input.ProviderLocationID,
		},
	); err != nil {
		return LocationMapping{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return LocationMapping{}, fmt.Errorf("commit provider location mapping: %w", err)
	}
	return r.getLocationMapping(ctx, id)
}

func (r *PostgresRepository) ListProviderQuotas(
	ctx context.Context,
	limit int,
) ([]ProviderQuota, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			tenant_id,
			provider_code,
			credential_alias,
			quota_date::text,
			daily_limit,
			used_count,
			reserved_count,
			greatest(daily_limit - used_count - reserved_count, 0),
			CASE
				WHEN daily_limit = 0 THEN 0
				ELSE round(((used_count + reserved_count)::numeric / daily_limit::numeric) * 100, 2)
			END::float8,
			health_status,
			reset_at,
			updated_at
		FROM provider_quota_ledger
		ORDER BY quota_date DESC, provider_code, credential_alias
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list provider quota: %w", err)
	}
	defer rows.Close()

	items := make([]ProviderQuota, 0)
	for rows.Next() {
		var item ProviderQuota
		if err := rows.Scan(
			&item.TenantID,
			&item.ProviderCode,
			&item.CredentialAlias,
			&item.QuotaDate,
			&item.DailyLimit,
			&item.UsedCount,
			&item.ReservedCount,
			&item.RemainingCount,
			&item.UsagePercentage,
			&item.HealthStatus,
			&item.ResetAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan provider quota: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate provider quota: %w", err)
	}
	return items, nil
}

func (r *PostgresRepository) Catalog(ctx context.Context) (Catalog, error) {
	result := Catalog{
		Couriers:         make([]CatalogCourier, 0),
		RoundingProfiles: make([]CatalogRoundingProfile, 0),
	}
	rows, err := r.pool.Query(ctx, `
		SELECT c.code, c.name, cs.code, cs.name, cs.service_group, cs.service_type
		FROM couriers c
		JOIN courier_services cs ON cs.courier_id = c.id
		WHERE c.active AND cs.active
		ORDER BY c.name, cs.code
	`)
	if err != nil {
		return Catalog{}, fmt.Errorf("query admin courier catalog: %w", err)
	}
	courierIndexes := make(map[string]int)
	for rows.Next() {
		var courierCode, courierName string
		var service CatalogService
		if err := rows.Scan(
			&courierCode,
			&courierName,
			&service.Code,
			&service.Name,
			&service.Group,
			&service.ServiceType,
		); err != nil {
			rows.Close()
			return Catalog{}, fmt.Errorf("scan admin courier catalog: %w", err)
		}
		index, exists := courierIndexes[courierCode]
		if !exists {
			index = len(result.Couriers)
			courierIndexes[courierCode] = index
			result.Couriers = append(result.Couriers, CatalogCourier{
				Code: courierCode, Name: courierName, Services: make([]CatalogService, 0),
			})
		}
		result.Couriers[index].Services = append(result.Couriers[index].Services, service)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Catalog{}, fmt.Errorf("iterate admin courier catalog: %w", err)
	}
	rows.Close()

	profileRows, err := r.pool.Query(ctx, `
		SELECT code, rounding_mode, increment_grams, threshold_grams
		FROM rounding_profiles
		WHERE verification_status <> 'deprecated'
		  AND effective_from <= current_date
		  AND (effective_until IS NULL OR effective_until >= current_date)
		ORDER BY code
	`)
	if err != nil {
		return Catalog{}, fmt.Errorf("query rounding profile catalog: %w", err)
	}
	defer profileRows.Close()
	for profileRows.Next() {
		var profile CatalogRoundingProfile
		if err := profileRows.Scan(
			&profile.Code, &profile.RoundingMode, &profile.IncrementGrams, &profile.ThresholdGrams,
		); err != nil {
			return Catalog{}, fmt.Errorf("scan rounding profile catalog: %w", err)
		}
		result.RoundingProfiles = append(result.RoundingProfiles, profile)
	}
	if err := profileRows.Err(); err != nil {
		return Catalog{}, fmt.Errorf("iterate rounding profile catalog: %w", err)
	}
	return result, nil
}

const rateCardSelect = `
	SELECT
		rc.id::text,
		origin.public_id,
		concat_ws(', ', origin.subdistrict, origin.district, origin.city, origin.province),
		destination.public_id,
		concat_ws(', ', destination.subdistrict, destination.district, destination.city, destination.province),
		c.code,
		c.name,
		cs.code,
		cs.name,
		cs.service_type,
		rc.pricing_model,
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
		rc.effective_until,
		rc.expires_at
	FROM rate_cards rc
	JOIN locations origin ON origin.id = rc.origin_location_id
	JOIN locations destination ON destination.id = rc.destination_location_id
	JOIN courier_services cs ON cs.id = rc.courier_service_id
	JOIN couriers c ON c.id = cs.courier_id
	JOIN rounding_profiles rp ON rp.id = rc.rounding_profile_id
`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRateCard(row rowScanner) (RateCard, error) {
	var item RateCard
	if err := row.Scan(
		&item.ID,
		&item.OriginPublicID,
		&item.OriginLabel,
		&item.DestinationPublicID,
		&item.DestinationLabel,
		&item.CourierCode,
		&item.CourierName,
		&item.ServiceCode,
		&item.ServiceName,
		&item.ServiceType,
		&item.PricingModel,
		&item.BaseWeightGrams,
		&item.BasePrice,
		&item.RatePerIncrement,
		&item.MinimumWeightGrams,
		&item.MaximumWeightGrams,
		&item.WeightIncrementGrams,
		&item.VolumetricDivisor,
		&item.RoundingProfileCode,
		&item.RoundingMode,
		&item.RoundingIncrementGrams,
		&item.RoundingThresholdGrams,
		&item.ETDMinDays,
		&item.ETDMaxDays,
		&item.VerificationStatus,
		&item.SourceProvider,
		&item.SourceReference,
		&item.EffectiveFrom,
		&item.EffectiveUntil,
		&item.ExpiresAt,
	); err != nil {
		return RateCard{}, fmt.Errorf("scan admin rate card: %w", err)
	}
	return item, nil
}

func (r *PostgresRepository) getRateCard(ctx context.Context, id string) (RateCard, error) {
	item, err := scanRateCard(r.pool.QueryRow(ctx, rateCardSelect+" WHERE rc.id = $1::uuid", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return RateCard{}, ErrNotFound
	}
	return item, err
}

func (r *PostgresRepository) getLocationMapping(
	ctx context.Context,
	id string,
) (LocationMapping, error) {
	var item LocationMapping
	err := r.pool.QueryRow(ctx, `
		SELECT
			plm.id::text,
			l.public_id,
			concat_ws(', ', l.subdistrict, l.district, l.city, l.province),
			plm.provider_code,
			plm.provider_location_id,
			coalesce(plm.provider_location_name, ''),
			plm.granularity,
			plm.verified_at,
			plm.active
		FROM provider_location_mappings plm
		JOIN locations l ON l.id = plm.location_id
		WHERE plm.id = $1::uuid
	`, id).Scan(
		&item.ID,
		&item.LocationPublicID,
		&item.LocationLabel,
		&item.ProviderCode,
		&item.ProviderLocationID,
		&item.ProviderLocationName,
		&item.Granularity,
		&item.VerifiedAt,
		&item.Active,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return LocationMapping{}, ErrNotFound
	}
	if err != nil {
		return LocationMapping{}, fmt.Errorf("get provider location mapping: %w", err)
	}
	return item, nil
}

func insertAudit(
	ctx context.Context,
	tx pgx.Tx,
	actorAlias, action, resourceType, resourceID, requestID string,
	details map[string]any,
) error {
	if details == nil {
		details = map[string]any{}
	}
	encoded, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode audit details: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO admin_audit_logs (
			actor_alias, action, resource_type, resource_id, request_id, details_json
		)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb)
	`, actorAlias, action, resourceType, resourceID, requestID, string(encoded))
	if err != nil {
		return fmt.Errorf("insert admin audit log: %w", err)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505"
}

func nullIfBlank(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}
