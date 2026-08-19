package rates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/emisell/api-kurir/internal/tenancy"
	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepository) FindFreshProviderQuotes(
	ctx context.Context,
	request Request,
	providerCode string,
) ([]ProviderQuote, error) {
	fingerprint := providerRequestFingerprint(request)
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT ON (courier_code, service_code)
			provider_code,
			courier_code,
			coalesce(courier_name, courier_code),
			service_code,
			coalesce(service_name, service_code),
			coalesce(description, ''),
			canonical_service_code,
			service_group,
			service_type,
			service_variant_code,
			returned_cost,
			etd_min_days,
			etd_max_days,
			verification_status,
			fetched_at,
			expires_at
		FROM rate_snapshots
		WHERE request_fingerprint = $1
		  AND provider_code = $2
		  AND tenant_id = $3
		  AND integration_id = $4
		  AND source_type = 'provider_quote'
		  AND expires_at > now()
		ORDER BY courier_code, service_code, fetched_at DESC
	`, fingerprint, providerCode, request.TenantID, request.IntegrationID)
	if err != nil {
		return nil, fmt.Errorf("query provider quote snapshots: %w", err)
	}
	defer rows.Close()

	quotes := make([]ProviderQuote, 0)
	for rows.Next() {
		var quote ProviderQuote
		if err := rows.Scan(
			&quote.ProviderCode,
			&quote.CourierCode,
			&quote.CourierName,
			&quote.ServiceCode,
			&quote.ServiceName,
			&quote.Description,
			&quote.CanonicalServiceCode,
			&quote.ServiceGroup,
			&quote.ServiceType,
			&quote.ServiceVariantCode,
			&quote.Cost,
			&quote.ETDMinDays,
			&quote.ETDMaxDays,
			&quote.VerificationStatus,
			&quote.FetchedAt,
			&quote.ExpiresAt,
		); err != nil {
			return nil, fmt.Errorf("scan provider quote snapshot: %w", err)
		}
		quotes = append(quotes, quote)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate provider quote snapshots: %w", err)
	}
	return quotes, nil
}

func (r *PostgresRepository) SaveProviderQuotes(
	ctx context.Context,
	request Request,
	quotes []ProviderQuote,
) error {
	if len(quotes) == 0 {
		return nil
	}
	dimensionsJSON, err := json.Marshal(request.Dimensions)
	if err != nil {
		return fmt.Errorf("encode quote dimensions: %w", err)
	}
	fingerprint := providerRequestFingerprint(request)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin provider quote snapshots: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, quote := range quotes {
		quote = classifyProviderQuote(quote)
		responseHash := providerQuoteHash(quote)
		commandTag, err := tx.Exec(ctx, `
			INSERT INTO rate_snapshots (
				origin_location_id,
				destination_location_id,
				courier_code,
				courier_name,
				service_code,
				service_name,
				description,
				canonical_service_code,
				service_group,
				service_type,
				service_variant_code,
				requested_weight_grams,
				requested_dimensions_json,
				request_fingerprint,
				returned_cost,
				etd_min_days,
				etd_max_days,
				raw_response_hash,
				provider_code,
				verification_status,
				source_type,
				fetched_at,
				expires_at,
				tenant_id,
				integration_id
			)
			SELECT
				origin.id,
				destination.id,
				$4,
				$5,
				$6,
				$7,
				$8,
				$9,
				$10,
				$11,
				$12,
				$13,
				$14,
				$15,
				$16,
				$17,
				$18,
				$19,
				$20,
				$21,
				'provider_quote',
				$22,
				$23,
				$24,
				$25
			FROM locations origin
			CROSS JOIN locations destination
			WHERE origin.public_id = $1
			  AND destination.public_id = $2
			  AND $3 <> ''
		`,
			request.Origin,
			request.Destination,
			quote.ProviderCode,
			quote.CourierCode,
			quote.CourierName,
			quote.ServiceCode,
			quote.ServiceName,
			quote.Description,
			quote.CanonicalServiceCode,
			quote.ServiceGroup,
			quote.ServiceType,
			quote.ServiceVariantCode,
			request.ActualWeightGrams,
			string(dimensionsJSON),
			fingerprint,
			quote.Cost,
			quote.ETDMinDays,
			quote.ETDMaxDays,
			responseHash,
			quote.ProviderCode,
			quote.VerificationStatus,
			quote.FetchedAt,
			quote.ExpiresAt,
			request.TenantID,
			request.IntegrationID,
		)
		if err != nil {
			return fmt.Errorf("insert provider quote snapshot: %w", err)
		}
		if commandTag.RowsAffected() != 1 {
			return ErrProviderLocationMapping
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO courier_service_aliases (
				provider_code,
				courier_id,
				raw_service_code,
				raw_service_name,
				canonical_service_id,
				classification_source,
				source_reference,
				first_seen_at,
				last_seen_at
			)
			SELECT
				$1,
				c.id,
				$3,
				$4,
				cs.id,
				$6,
				nullif($7, ''),
				$8,
				$8
			FROM couriers c
			LEFT JOIN courier_services cs
			  ON cs.courier_id = c.id
			 AND cs.code = $5
			WHERE c.code = $2
			ON CONFLICT (provider_code, courier_id, raw_service_code) DO UPDATE
			SET raw_service_name = CASE
			        WHEN EXCLUDED.raw_service_name <> ''
			        THEN EXCLUDED.raw_service_name
			        ELSE courier_service_aliases.raw_service_name
			    END,
			    canonical_service_id = coalesce(
			        EXCLUDED.canonical_service_id,
			        courier_service_aliases.canonical_service_id
			    ),
			    classification_source = CASE
			        WHEN courier_service_aliases.classification_source
			             IN ('official_public', 'official_contract')
			        THEN courier_service_aliases.classification_source
			        WHEN EXCLUDED.canonical_service_id IS NOT NULL
			        THEN EXCLUDED.classification_source
			        ELSE courier_service_aliases.classification_source
			    END,
			    source_reference = coalesce(
			        courier_service_aliases.source_reference,
			        EXCLUDED.source_reference
			    ),
			    last_seen_at = greatest(
			        courier_service_aliases.last_seen_at,
			        EXCLUDED.last_seen_at
			    ),
			    active = true,
			    updated_at = now()
		`,
			quote.ProviderCode,
			quote.CourierCode,
			quote.ServiceCode,
			quote.ServiceName,
			quote.CanonicalServiceCode,
			quote.ClassificationSource,
			quote.ClassificationReference,
			quote.FetchedAt,
		); err != nil {
			return fmt.Errorf("upsert provider service alias: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit provider quote snapshots: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ConsumeProviderHit(
	ctx context.Context,
	providerCode string,
	credentialAlias string,
	dailyLimit int64,
) error {
	if dailyLimit <= 0 {
		return ErrProviderQuotaExhausted
	}
	quotaDate, resetAt := providerQuotaWindow()
	tenantID := tenancy.TenantID(ctx)

	var usedCount int64
	err := r.pool.QueryRow(ctx, `
		INSERT INTO provider_quota_ledger (
			tenant_id,
			provider_code,
			credential_alias,
			quota_date,
			daily_limit,
			used_count,
			reset_at
		)
		VALUES ($1, $2, $3, $4::date, $5, 1, $6)
		ON CONFLICT (tenant_id, provider_code, credential_alias, quota_date) DO UPDATE
		SET daily_limit = EXCLUDED.daily_limit,
		    used_count = provider_quota_ledger.used_count + 1,
		    reset_at = EXCLUDED.reset_at,
		    updated_at = now()
		WHERE provider_quota_ledger.used_count + provider_quota_ledger.reserved_count
		      < EXCLUDED.daily_limit
		RETURNING used_count
	`, tenantID, providerCode, credentialAlias, quotaDate, dailyLimit, resetAt).Scan(&usedCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrProviderQuotaExhausted
	}
	if err != nil {
		return fmt.Errorf("consume provider quota: %w", err)
	}
	return nil
}

func (r *PostgresRepository) MarkProviderQuotaExhausted(
	ctx context.Context,
	providerCode string,
	credentialAlias string,
	dailyLimit int64,
) error {
	if dailyLimit <= 0 {
		return ErrProviderQuotaExhausted
	}
	quotaDate, resetAt := providerQuotaWindow()
	tenantID := tenancy.TenantID(ctx)

	_, err := r.pool.Exec(ctx, `
		INSERT INTO provider_quota_ledger (
			tenant_id,
			provider_code,
			credential_alias,
			quota_date,
			daily_limit,
			used_count,
			reset_at
		)
		VALUES ($1, $2, $3, $4::date, $5, $5, $6)
		ON CONFLICT (tenant_id, provider_code, credential_alias, quota_date) DO UPDATE
		SET daily_limit = EXCLUDED.daily_limit,
		    used_count = GREATEST(
				provider_quota_ledger.used_count,
				EXCLUDED.daily_limit
			),
		    reserved_count = 0,
		    reset_at = EXCLUDED.reset_at,
		    updated_at = now()
	`, tenantID, providerCode, credentialAlias, quotaDate, dailyLimit, resetAt)
	if err != nil {
		return fmt.Errorf("mark provider quota exhausted: %w", err)
	}
	return nil
}

func providerQuotaWindow() (string, time.Time) {
	jakarta := time.FixedZone("Asia/Jakarta", 7*60*60)
	now := time.Now().In(jakarta)
	quotaDate := now.Format("2006-01-02")
	resetAt := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, jakarta)
	return quotaDate, resetAt
}

func (r *PostgresRepository) RecordProviderAPICall(
	ctx context.Context,
	providerCode string,
	credentialAlias string,
	endpoint string,
	requestFingerprint string,
	httpStatus int,
	outcome string,
	duration time.Duration,
	quotaCost int,
	errorCode string,
) error {
	durationMillis := duration.Milliseconds()
	if durationMillis < 0 {
		durationMillis = 0
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO provider_api_calls (
			tenant_id,
			provider_code,
			credential_alias,
			endpoint,
			request_fingerprint,
			http_status,
			outcome,
			duration_ms,
			quota_cost,
			error_code
		)
		VALUES ($1, $2, $3, $4, $5, nullif($6, 0), $7, $8, $9, nullif($10, ''))
	`,
		tenancy.TenantID(ctx),
		providerCode,
		credentialAlias,
		endpoint,
		requestFingerprint,
		httpStatus,
		outcome,
		durationMillis,
		quotaCost,
		errorCode,
	)
	if err != nil {
		return fmt.Errorf("record provider API call: %w", err)
	}
	return nil
}

func providerRequestFingerprint(request Request) string {
	couriers := append([]string(nil), request.Couriers...)
	sort.Strings(couriers)
	payload := struct {
		TenantID          string
		IntegrationID     string
		Origin            string
		Destination       string
		Granularity       string
		PriceFilter       string
		ActualWeightGrams int64
		Couriers          []string
		Dimensions        *Dimensions
		ItemValue         int64
	}{
		TenantID:          request.TenantID,
		IntegrationID:     request.IntegrationID,
		Origin:            request.Origin,
		Destination:       request.Destination,
		Granularity:       request.Granularity,
		PriceFilter:       request.PriceFilter,
		ActualWeightGrams: request.ActualWeightGrams,
		Couriers:          couriers,
		Dimensions:        request.Dimensions,
		ItemValue:         request.ItemValue,
	}
	encoded, _ := json.Marshal(payload)
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}

func providerQuoteHash(quote ProviderQuote) string {
	encoded, _ := json.Marshal(struct {
		Provider string
		Courier  string
		Service  string
		Cost     int64
		ETDMin   *int
		ETDMax   *int
	}{
		Provider: quote.ProviderCode,
		Courier:  quote.CourierCode,
		Service:  quote.ServiceCode,
		Cost:     quote.Cost,
		ETDMin:   quote.ETDMinDays,
		ETDMax:   quote.ETDMaxDays,
	})
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}
