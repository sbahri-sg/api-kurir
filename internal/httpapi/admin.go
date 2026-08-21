package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/emisell/api-kurir/internal/admin"
	"github.com/labstack/echo/v5"
)

const maxAdminBodyBytes = 128 * 1024

var (
	validUUID = regexp.MustCompile(
		`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`,
	)
	validAdminActor = regexp.MustCompile(`^[A-Za-z0-9._@-]{1,64}$`)
)

type createRateCardRequest struct {
	OriginPublicID       string  `json:"origin_public_id"`
	DestinationPublicID  string  `json:"destination_public_id"`
	CourierCode          string  `json:"courier_code"`
	ServiceCode          string  `json:"service_code"`
	PricingModel         string  `json:"pricing_model"`
	BaseWeightGrams      int64   `json:"base_weight_grams"`
	BasePrice            int64   `json:"base_price"`
	RatePerIncrement     int64   `json:"rate_per_increment"`
	MinimumWeightGrams   int64   `json:"minimum_weight_grams"`
	MaximumWeightGrams   *int64  `json:"maximum_weight_grams"`
	WeightIncrementGrams int64   `json:"weight_increment_grams"`
	VolumetricDivisor    *int64  `json:"volumetric_divisor"`
	RoundingProfileCode  string  `json:"rounding_profile_code"`
	ETDMinDays           *int    `json:"etd_min_days"`
	ETDMaxDays           *int    `json:"etd_max_days"`
	VerificationStatus   string  `json:"verification_status"`
	SourceProvider       string  `json:"source_provider"`
	SourceReference      string  `json:"source_reference"`
	EffectiveFrom        string  `json:"effective_from"`
	ExpiresAt            *string `json:"expires_at"`
}

type upsertLocationMappingRequest struct {
	LocationPublicID     string `json:"location_public_id"`
	ProviderCode         string `json:"provider_code"`
	ProviderLocationID   string `json:"provider_location_id"`
	ProviderLocationName string `json:"provider_location_name"`
	Granularity          string `json:"granularity"`
}

func adminOverviewHandler(repository admin.Repository) echo.HandlerFunc {
	return func(c *echo.Context) error {
		result, err := repository.Overview(c.Request().Context())
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, adminResponse(c, result))
	}
}

func adminTrackingOperationListHandler(repository admin.Repository) echo.HandlerFunc {
	return func(c *echo.Context) error {
		limit, offset, err := adminPagination(c)
		if err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		result, err := repository.ListTrackingOperations(c.Request().Context(), admin.TrackingOperationFilter{
			Search: c.QueryParam("search"), CourierCode: c.QueryParam("courier"),
			ValidationStatus: c.QueryParam("validation_status"), QueueStatus: c.QueryParam("queue_status"),
			Limit: limit, Offset: offset,
		})
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, adminResponse(c, result))
	}
}

func adminTrackingOperationDeleteHandler(repository admin.Repository) echo.HandlerFunc {
	return func(c *echo.Context) error {
		id := strings.TrimSpace(c.Param("id"))
		if !validUUID.MatchString(id) {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "ID resi tidak valid.", nil)
		}
		err := repository.DeleteTrackingOperation(
			c.Request().Context(), id, adminActor(c), requestID(c),
		)
		if response := writeAdminRepositoryError(c, err); response != nil {
			return response
		}
		return c.NoContent(http.StatusNoContent)
	}
}

func adminCatalogHandler(repository admin.Repository) echo.HandlerFunc {
	return func(c *echo.Context) error {
		result, err := repository.Catalog(c.Request().Context())
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, adminResponse(c, result))
	}
}

func adminRateCardListHandler(repository admin.Repository) echo.HandlerFunc {
	return func(c *echo.Context) error {
		limit, offset, err := adminPagination(c)
		if err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		result, err := repository.ListRateCards(
			c.Request().Context(),
			c.QueryParam("search"),
			limit,
			offset,
		)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, adminListResponse(c, result, limit, offset))
	}
}

func adminRateSnapshotListHandler(repository admin.Repository) echo.HandlerFunc {
	return func(c *echo.Context) error {
		limit, offset, err := adminPagination(c)
		if err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		result, err := repository.ListRateSnapshots(
			c.Request().Context(),
			c.QueryParam("search"),
			limit,
			offset,
		)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, adminListResponse(c, result, limit, offset))
	}
}

func adminRateCardCreateHandler(repository admin.Repository) echo.HandlerFunc {
	return func(c *echo.Context) error {
		var request createRateCardRequest
		if err := decodeAdminJSON(c, &request); err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		input, err := normalizeRateCardInput(request)
		if err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		result, err := repository.CreateRateCard(
			c.Request().Context(),
			input,
			adminActor(c),
			requestID(c),
		)
		if response := writeAdminRepositoryError(c, err); response != nil {
			return response
		}
		return c.JSON(http.StatusCreated, adminResponse(c, result))
	}
}

func adminRateCardDeprecateHandler(repository admin.Repository) echo.HandlerFunc {
	return func(c *echo.Context) error {
		id := strings.TrimSpace(c.Param("id"))
		if !validUUID.MatchString(id) {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "ID rate card tidak valid.", nil)
		}
		err := repository.DeprecateRateCard(
			c.Request().Context(),
			id,
			adminActor(c),
			requestID(c),
		)
		if response := writeAdminRepositoryError(c, err); response != nil {
			return response
		}
		return c.NoContent(http.StatusNoContent)
	}
}

func adminLocationMappingListHandler(repository admin.Repository) echo.HandlerFunc {
	return func(c *echo.Context) error {
		limit, offset, err := adminPagination(c)
		if err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		result, err := repository.ListLocationMappings(
			c.Request().Context(),
			c.QueryParam("search"),
			c.QueryParam("provider"),
			limit,
			offset,
		)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, adminListResponse(c, result, limit, offset))
	}
}

func adminLocationMappingUpsertHandler(repository admin.Repository) echo.HandlerFunc {
	return func(c *echo.Context) error {
		var request upsertLocationMappingRequest
		if err := decodeAdminJSON(c, &request); err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		input, err := normalizeLocationMappingInput(request)
		if err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		result, err := repository.UpsertLocationMapping(
			c.Request().Context(),
			input,
			adminActor(c),
			requestID(c),
		)
		if response := writeAdminRepositoryError(c, err); response != nil {
			return response
		}
		return c.JSON(http.StatusOK, adminResponse(c, result))
	}
}

func adminProviderQuotaListHandler(repository admin.Repository) echo.HandlerFunc {
	return func(c *echo.Context) error {
		limit := 30
		if raw := strings.TrimSpace(c.QueryParam("limit")); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 || value > 365 {
				return writeError(
					c,
					http.StatusBadRequest,
					"INVALID_REQUEST",
					"limit harus antara 1 dan 365.",
					nil,
				)
			}
			limit = value
		}
		result, err := repository.ListProviderQuotas(c.Request().Context(), limit)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, adminResponse(c, result))
	}
}

func normalizeRateCardInput(request createRateCardRequest) (admin.RateCardInput, error) {
	request.OriginPublicID = strings.TrimSpace(request.OriginPublicID)
	request.DestinationPublicID = strings.TrimSpace(request.DestinationPublicID)
	request.CourierCode = strings.ToLower(strings.TrimSpace(request.CourierCode))
	request.ServiceCode = strings.TrimSpace(request.ServiceCode)
	request.PricingModel = strings.TrimSpace(request.PricingModel)
	request.RoundingProfileCode = strings.TrimSpace(request.RoundingProfileCode)
	request.VerificationStatus = strings.TrimSpace(request.VerificationStatus)
	request.SourceProvider = strings.ToLower(strings.TrimSpace(request.SourceProvider))
	request.SourceReference = strings.TrimSpace(request.SourceReference)

	if request.OriginPublicID == "" ||
		request.DestinationPublicID == "" ||
		request.OriginPublicID == request.DestinationPublicID {
		return admin.RateCardInput{}, errors.New("origin dan destination wajib valid dan berbeda")
	}
	if !validCourierCode.MatchString(request.CourierCode) || request.ServiceCode == "" {
		return admin.RateCardInput{}, errors.New("courier atau service tidak valid")
	}
	if _, valid := admin.ValidPricingModels[request.PricingModel]; !valid {
		return admin.RateCardInput{}, errors.New("pricing_model belum didukung mesin tarif")
	}
	if _, valid := admin.ValidVerificationStatuses[request.VerificationStatus]; !valid {
		return admin.RateCardInput{}, errors.New("verification_status tidak valid")
	}
	if request.RoundingProfileCode == "" || request.SourceProvider == "" {
		return admin.RateCardInput{}, errors.New("rounding_profile_code dan source_provider wajib diisi")
	}
	if request.BaseWeightGrams < 0 ||
		request.BasePrice < 0 ||
		request.RatePerIncrement < 0 ||
		request.MinimumWeightGrams < 0 ||
		request.WeightIncrementGrams <= 0 {
		return admin.RateCardInput{}, errors.New("nilai berat dan harga tidak boleh negatif; increment harus positif")
	}
	if request.MaximumWeightGrams != nil &&
		(*request.MaximumWeightGrams <= 0 || *request.MaximumWeightGrams < request.MinimumWeightGrams) {
		return admin.RateCardInput{}, errors.New("maximum_weight_grams tidak valid")
	}
	if request.VolumetricDivisor != nil && *request.VolumetricDivisor <= 0 {
		return admin.RateCardInput{}, errors.New("volumetric_divisor harus positif")
	}
	if request.ETDMinDays != nil && *request.ETDMinDays < 0 ||
		request.ETDMaxDays != nil && *request.ETDMaxDays < 0 ||
		request.ETDMinDays != nil && request.ETDMaxDays != nil && *request.ETDMaxDays < *request.ETDMinDays {
		return admin.RateCardInput{}, errors.New("rentang ETD tidak valid")
	}

	effectiveFrom := time.Now().UTC()
	if strings.TrimSpace(request.EffectiveFrom) != "" {
		parsed, err := time.Parse(time.RFC3339, request.EffectiveFrom)
		if err != nil {
			return admin.RateCardInput{}, errors.New("effective_from harus RFC3339")
		}
		effectiveFrom = parsed.UTC()
	}
	var expiresAt *time.Time
	if request.ExpiresAt != nil && strings.TrimSpace(*request.ExpiresAt) != "" {
		parsed, err := time.Parse(time.RFC3339, *request.ExpiresAt)
		if err != nil {
			return admin.RateCardInput{}, errors.New("expires_at harus RFC3339")
		}
		parsed = parsed.UTC()
		if !parsed.After(effectiveFrom) {
			return admin.RateCardInput{}, errors.New("expires_at harus setelah effective_from")
		}
		expiresAt = &parsed
	}

	return admin.RateCardInput{
		OriginPublicID:       request.OriginPublicID,
		DestinationPublicID:  request.DestinationPublicID,
		CourierCode:          request.CourierCode,
		ServiceCode:          request.ServiceCode,
		PricingModel:         request.PricingModel,
		BaseWeightGrams:      request.BaseWeightGrams,
		BasePrice:            request.BasePrice,
		RatePerIncrement:     request.RatePerIncrement,
		MinimumWeightGrams:   request.MinimumWeightGrams,
		MaximumWeightGrams:   request.MaximumWeightGrams,
		WeightIncrementGrams: request.WeightIncrementGrams,
		VolumetricDivisor:    request.VolumetricDivisor,
		RoundingProfileCode:  request.RoundingProfileCode,
		ETDMinDays:           request.ETDMinDays,
		ETDMaxDays:           request.ETDMaxDays,
		VerificationStatus:   request.VerificationStatus,
		SourceProvider:       request.SourceProvider,
		SourceReference:      request.SourceReference,
		EffectiveFrom:        effectiveFrom,
		ExpiresAt:            expiresAt,
	}, nil
}

func normalizeLocationMappingInput(
	request upsertLocationMappingRequest,
) (admin.LocationMappingInput, error) {
	request.LocationPublicID = strings.TrimSpace(request.LocationPublicID)
	request.ProviderCode = strings.ToLower(strings.TrimSpace(request.ProviderCode))
	request.ProviderLocationID = strings.TrimSpace(request.ProviderLocationID)
	request.ProviderLocationName = strings.TrimSpace(request.ProviderLocationName)
	request.Granularity = strings.ToLower(strings.TrimSpace(request.Granularity))

	if request.LocationPublicID == "" ||
		request.ProviderLocationID == "" ||
		!validCourierCode.MatchString(request.ProviderCode) {
		return admin.LocationMappingInput{}, errors.New("lokasi, provider, dan provider location ID wajib valid")
	}
	switch request.Granularity {
	case "province", "city", "district", "subdistrict":
	default:
		return admin.LocationMappingInput{}, errors.New("granularity tidak valid")
	}
	return admin.LocationMappingInput{
		LocationPublicID:     request.LocationPublicID,
		ProviderCode:         request.ProviderCode,
		ProviderLocationID:   request.ProviderLocationID,
		ProviderLocationName: request.ProviderLocationName,
		Granularity:          request.Granularity,
	}, nil
}

func decodeAdminJSON(c *echo.Context, target any) error {
	body := http.MaxBytesReader(c.Response(), c.Request().Body, maxAdminBodyBytes)
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("payload JSON tidak valid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("payload hanya boleh berisi satu objek JSON")
	}
	return nil
}

func adminPagination(c *echo.Context) (int, int, error) {
	limit, offset := 50, 0
	if raw := strings.TrimSpace(c.QueryParam("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 200 {
			return 0, 0, errors.New("limit harus antara 1 dan 200")
		}
		limit = value
	}
	if raw := strings.TrimSpace(c.QueryParam("offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return 0, 0, errors.New("offset tidak boleh negatif")
		}
		offset = value
	}
	return limit, offset, nil
}

func writeAdminRepositoryError(c *echo.Context, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, admin.ErrNotFound):
		return writeError(
			c,
			http.StatusNotFound,
			"ADMIN_REFERENCE_NOT_FOUND",
			"Referensi data admin tidak ditemukan atau tidak aktif.",
			nil,
		)
	case errors.Is(err, admin.ErrConflict):
		return writeError(
			c,
			http.StatusConflict,
			"ADMIN_DATA_CONFLICT",
			"Data bertabrakan dengan versi atau mapping yang sudah ada.",
			nil,
		)
	default:
		return err
	}
}

func adminActor(c *echo.Context) string {
	value := strings.TrimSpace(c.Request().Header.Get("X-Admin-Actor"))
	if !validAdminActor.MatchString(value) {
		return "dashboard"
	}
	return value
}

func adminResponse(c *echo.Context, data any) map[string]any {
	return map[string]any{
		"meta": map[string]any{"request_id": requestID(c)},
		"data": data,
	}
}

func adminListResponse(
	c *echo.Context,
	data any,
	limit, offset int,
) map[string]any {
	return map[string]any{
		"meta": map[string]any{
			"request_id": requestID(c),
			"limit":      limit,
			"offset":     offset,
		},
		"data": data,
	}
}
