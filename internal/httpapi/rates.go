package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emisell/api-kurir/internal/locations"
	"github.com/emisell/api-kurir/internal/rates"
	"github.com/emisell/api-kurir/internal/tenancy"
	"github.com/labstack/echo/v5"
)

const maxCalculateBodyBytes = 64 * 1024

var validCourierCode = regexp.MustCompile(`^[a-z0-9_-]{2,32}$`)

var (
	errUnsupportedRajaOngkirRateMediaType = errors.New("unsupported RajaOngkir rate media type")
	errInvalidRajaOngkirRateForm          = errors.New("invalid RajaOngkir rate form")
)

type calculateRequest struct {
	Origin      string              `json:"origin"`
	Destination string              `json:"destination"`
	Weight      int64               `json:"weight"`
	Courier     string              `json:"courier"`
	Price       string              `json:"price,omitempty"`
	Dimensions  *calculateDimension `json:"dimensions,omitempty"`
	ItemValue   int64               `json:"item_value,omitempty"`
	Options     calculateOptions    `json:"options,omitempty"`
}

type calculateDimension struct {
	Length int64  `json:"length"`
	Width  int64  `json:"width"`
	Height int64  `json:"height"`
	Unit   string `json:"unit"`
}

type calculateOptions struct {
	IncludeInsurance  bool `json:"include_insurance,omitempty"`
	IncludeUnverified bool `json:"include_unverified,omitempty"`
}

func calculateRateHandler(service *rates.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if err := requireTenantScopeIfPresent(c, "shipping:read"); err != nil {
			return err
		}
		var input calculateRequest
		body := http.MaxBytesReader(c.Response(), c.Request().Body, maxCalculateBodyBytes)
		decoder := json.NewDecoder(body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			return writeError(
				c,
				http.StatusBadRequest,
				"INVALID_REQUEST",
				"Payload JSON tidak valid.",
				map[string]any{"reason": err.Error()},
			)
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return writeError(
				c,
				http.StatusBadRequest,
				"INVALID_REQUEST",
				"Payload hanya boleh berisi satu objek JSON.",
				nil,
			)
		}

		request, err := normalizeCalculateRequest(input)
		if err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		attachTenantRateContext(c.Request().Context(), &request)

		results, err := service.Calculate(c.Request().Context(), request)
		switch {
		case errors.Is(err, rates.ErrRateNotAvailable):
			return writeError(
				c,
				http.StatusUnprocessableEntity,
				"RATE_NOT_AVAILABLE",
				"Tarif belum tersedia untuk rute dan kurir ini.",
				map[string]any{"courier": request.Couriers},
			)
		case errors.Is(err, rates.ErrProviderLocationMapping):
			return writeError(
				c,
				http.StatusUnprocessableEntity,
				"PROVIDER_LOCATION_NOT_MAPPED",
				"Origin atau destination belum memiliki mapping provider.",
				nil,
			)
		case errors.Is(err, rates.ErrProviderQuotaExhausted):
			return writeError(
				c,
				http.StatusServiceUnavailable,
				"PROVIDER_QUOTA_EXHAUSTED",
				"Kuota provider untuk hari ini telah habis.",
				nil,
			)
		case errors.Is(err, rates.ErrProviderUnauthorized):
			return writeError(
				c,
				http.StatusBadGateway,
				"PROVIDER_AUTHENTICATION_FAILED",
				"Autentikasi ke provider gagal.",
				nil,
			)
		case errors.Is(err, rates.ErrProviderUnavailable):
			return writeError(
				c,
				http.StatusBadGateway,
				"PROVIDER_ERROR",
				"Provider belum dapat memberikan tarif.",
				nil,
			)
		case errors.Is(err, rates.ErrInvalidShipment):
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		case err != nil:
			return err
		}

		calculatedAt := time.Now().UTC()
		data := make([]map[string]any, 0, len(results))
		warnings := make([]string, 0)
		hasProviderQuote := false
		for _, result := range results {
			if result.Card.VerificationStatus == "observed" ||
				result.Card.VerificationStatus == "needs_contract_confirmation" {
				warnings = append(
					warnings,
					fmt.Sprintf("%s %s belum berstatus official_contract", result.Card.CourierCode, result.Card.ServiceCode),
				)
			}
			if result.SourceType == "provider_quote" {
				hasProviderQuote = true
				warnings = append(
					warnings,
					fmt.Sprintf(
						"%s %s adalah exact provider quote; aturan pembulatan provider tidak diinferensikan",
						result.Card.CourierCode,
						result.Card.ServiceCode,
					),
				)
			}
			if result.Card.ServiceGroup == "unknown" {
				warnings = append(
					warnings,
					fmt.Sprintf(
						"%s %s belum dikenali katalog canonical; kode mentah tetap dikembalikan",
						result.Card.CourierCode,
						result.Card.ServiceCode,
					),
				)
			}
			data = append(data, rateResponse(result, calculatedAt))
		}
		if hasProviderQuote && request.Dimensions != nil {
			warnings = append(
				warnings,
				"dimensi disimpan pada fingerprint tetapi tidak dikirim ke endpoint rate RajaOngkir V2; weight harus sudah aman sebagai berat provider",
			)
		}

		return c.JSON(http.StatusOK, map[string]any{
			"meta": map[string]any{
				"request_id":    requestID(c),
				"calculated_at": calculatedAt.Format(time.RFC3339),
				"currency":      "IDR",
			},
			"data":     data,
			"warnings": warnings,
		})
	}
}

func calculatePublicRateHandler(
	service *rates.Service,
	locationRepository locations.Repository,
	granularity string,
) echo.HandlerFunc {
	legacyHandler := calculateRateHandler(service)
	return func(c *echo.Context) error {
		if !rajaOngkirV2Compatibility(c) {
			return legacyHandler(c)
		}
		return calculateRajaOngkirV2Rate(
			c,
			service,
			locationRepository,
			granularity,
		)
	}
}

func calculateRajaOngkirV2Rate(
	c *echo.Context,
	service *rates.Service,
	locationRepository locations.Repository,
	granularity string,
) error {
	if err := requireTenantScopeIfPresent(c, "shipping:read"); err != nil {
		return err
	}
	c.Request().Body = http.MaxBytesReader(
		c.Response(),
		c.Request().Body,
		maxCalculateBodyBytes,
	)
	if err := parseRajaOngkirRateForm(c); err != nil {
		if errors.Is(err, errUnsupportedRajaOngkirRateMediaType) {
			return writeError(
				c,
				http.StatusUnsupportedMediaType,
				"UNSUPPORTED_MEDIA_TYPE",
				"Content-Type harus application/x-www-form-urlencoded atau multipart/form-data.",
				nil,
			)
		}
		return writeError(
			c,
			http.StatusBadRequest,
			"INVALID_REQUEST",
			"Payload form tidak valid.",
			nil,
		)
	}

	weight, err := strconv.ParseInt(
		strings.TrimSpace(c.Request().FormValue("weight")),
		10,
		64,
	)
	if err != nil {
		return writeError(
			c,
			http.StatusBadRequest,
			"INVALID_REQUEST",
			"weight wajib berupa integer gram.",
			nil,
		)
	}
	priceFilter := strings.ToLower(strings.TrimSpace(c.Request().FormValue("price")))
	if priceFilter != "" && priceFilter != "lowest" && priceFilter != "highest" {
		return writeError(
			c,
			http.StatusBadRequest,
			"INVALID_REQUEST",
			"price harus lowest atau highest.",
			nil,
		)
	}
	courierValue := strings.TrimSpace(c.Request().FormValue("courier"))
	if courierValue == "" {
		return writeError(
			c,
			http.StatusBadRequest,
			"INVALID_REQUEST",
			"courier wajib diisi.",
			nil,
		)
	}
	if _, err := normalizeCourierCodes(courierValue); err != nil {
		return writeError(
			c,
			http.StatusUnprocessableEntity,
			"INVALID_COURIER",
			"Courier tidak valid.",
			nil,
		)
	}

	origin, err := resolveRajaOngkirLocation(
		c.Request().Context(),
		locationRepository,
		c.Request().FormValue("origin"),
		granularity,
	)
	if err != nil {
		return writeError(
			c,
			http.StatusBadRequest,
			"INVALID_REQUEST",
			"origin tidak valid.",
			nil,
		)
	}
	destination, err := resolveRajaOngkirLocation(
		c.Request().Context(),
		locationRepository,
		c.Request().FormValue("destination"),
		granularity,
	)
	if err != nil {
		return writeError(
			c,
			http.StatusBadRequest,
			"INVALID_REQUEST",
			"destination tidak valid.",
			nil,
		)
	}

	request, err := normalizeCalculateRequest(calculateRequest{
		Origin:      origin,
		Destination: destination,
		Weight:      weight,
		Courier:     courierValue,
		Price:       priceFilter,
	})
	if err != nil {
		return writeError(
			c,
			http.StatusBadRequest,
			"INVALID_REQUEST",
			err.Error(),
			nil,
		)
	}
	attachTenantRateContext(c.Request().Context(), &request)
	request.Granularity = granularity
	request.PriceFilter = priceFilter

	results, err := service.Calculate(c.Request().Context(), request)
	if err != nil {
		return writeRajaOngkirRateError(c, request, err)
	}

	data := make([]map[string]any, 0, len(results))
	for _, result := range results {
		data = append(data, map[string]any{
			"name":        result.Card.CourierName,
			"code":        result.Card.CourierCode,
			"service":     result.Card.ServiceCode,
			"description": result.Card.ServiceName,
			"cost":        result.Cost.Total,
			"etd":         rajaOngkirETD(result),
		})
	}
	return c.JSON(http.StatusOK, map[string]any{
		"meta": map[string]any{
			"message": "Success Calculate Domestic Shipping cost",
			"code":    http.StatusOK,
			"status":  "success",
		},
		"data": data,
	})
}

func parseRajaOngkirRateForm(c *echo.Context) error {
	mediaType, _, err := mime.ParseMediaType(
		strings.TrimSpace(c.Request().Header.Get("Content-Type")),
	)
	if err != nil {
		return errUnsupportedRajaOngkirRateMediaType
	}

	switch mediaType {
	case "application/x-www-form-urlencoded":
		err = c.Request().ParseForm()
	case "multipart/form-data":
		err = c.Request().ParseMultipartForm(maxCalculateBodyBytes)
	default:
		return errUnsupportedRajaOngkirRateMediaType
	}
	if err != nil {
		return fmt.Errorf("%w: %v", errInvalidRajaOngkirRateForm, err)
	}
	return nil
}

func resolveRajaOngkirLocation(
	ctx context.Context,
	repository locations.Repository,
	identifier string,
	granularity string,
) (string, error) {
	identifier = strings.TrimSpace(identifier)
	if strings.HasPrefix(identifier, "loc_") {
		return repository.ResolvePublicID(ctx, identifier, granularity)
	}
	if legacyRepository, ok := repository.(locations.LegacyRepository); ok {
		return legacyRepository.ResolveLegacyPublicID(
			ctx,
			legacyRegionProvider,
			granularity,
			identifier,
		)
	}
	return repository.ResolvePublicID(ctx, identifier, granularity)
}

func writeRajaOngkirRateError(
	c *echo.Context,
	request rates.Request,
	err error,
) error {
	switch {
	case errors.Is(err, rates.ErrRateNotAvailable):
		return writeError(
			c,
			http.StatusBadRequest,
			"RATE_NOT_AVAILABLE",
			"Calculate Domestic Shipping Cost not found.",
			map[string]any{"courier": request.Couriers},
		)
	case errors.Is(err, rates.ErrProviderLocationMapping):
		return writeError(
			c,
			http.StatusBadRequest,
			"INVALID_LOCATION",
			"Origin atau destination tidak valid.",
			nil,
		)
	case errors.Is(err, rates.ErrProviderQuotaExhausted):
		return writeError(
			c,
			http.StatusServiceUnavailable,
			"PROVIDER_QUOTA_EXHAUSTED",
			"Kuota provider untuk hari ini telah habis.",
			nil,
		)
	case errors.Is(err, rates.ErrProviderUnauthorized):
		return writeError(
			c,
			http.StatusBadGateway,
			"PROVIDER_AUTHENTICATION_FAILED",
			"Autentikasi ke provider gagal.",
			nil,
		)
	case errors.Is(err, rates.ErrProviderUnavailable):
		return writeError(
			c,
			http.StatusBadGateway,
			"PROVIDER_ERROR",
			"Provider belum dapat memberikan tarif.",
			nil,
		)
	case errors.Is(err, rates.ErrInvalidShipment):
		return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
	default:
		return err
	}
}

func rajaOngkirETD(result rates.Result) string {
	switch {
	case result.Card.ETDMinDays != nil && result.Card.ETDMaxDays != nil:
		if *result.Card.ETDMinDays == *result.Card.ETDMaxDays {
			return strconv.Itoa(*result.Card.ETDMinDays)
		}
		return fmt.Sprintf(
			"%d-%d",
			*result.Card.ETDMinDays,
			*result.Card.ETDMaxDays,
		)
	case result.Card.ETDMinDays != nil:
		return strconv.Itoa(*result.Card.ETDMinDays)
	case result.Card.ETDMaxDays != nil:
		return strconv.Itoa(*result.Card.ETDMaxDays)
	default:
		return ""
	}
}

func normalizeCalculateRequest(input calculateRequest) (rates.Request, error) {
	input.Origin = strings.TrimSpace(input.Origin)
	input.Destination = strings.TrimSpace(input.Destination)
	if input.Origin == "" || input.Destination == "" {
		return rates.Request{}, errors.New("origin dan destination wajib diisi")
	}
	if input.Origin == input.Destination {
		return rates.Request{}, errors.New("origin dan destination tidak boleh sama")
	}
	if input.Weight <= 0 || input.Weight > 100_000_000 {
		return rates.Request{}, errors.New("weight harus antara 1 dan 100.000.000 gram")
	}
	if input.ItemValue < 0 {
		return rates.Request{}, errors.New("item_value tidak boleh negatif")
	}
	input.Price = strings.ToLower(strings.TrimSpace(input.Price))
	if input.Price != "" && input.Price != "lowest" && input.Price != "highest" {
		return rates.Request{}, errors.New("price harus lowest atau highest")
	}

	couriers, err := normalizeCourierCodes(input.Courier)
	if err != nil {
		return rates.Request{}, err
	}

	var dimensions *rates.Dimensions
	if input.Dimensions != nil {
		if strings.ToLower(strings.TrimSpace(input.Dimensions.Unit)) != "cm" {
			return rates.Request{}, errors.New("unit dimensi harus cm")
		}
		if input.Dimensions.Length <= 0 || input.Dimensions.Width <= 0 || input.Dimensions.Height <= 0 {
			return rates.Request{}, errors.New("panjang, lebar, dan tinggi harus lebih dari nol")
		}
		if input.Dimensions.Length > 10_000 ||
			input.Dimensions.Width > 10_000 ||
			input.Dimensions.Height > 10_000 {
			return rates.Request{}, errors.New("setiap dimensi maksimal 10.000 cm")
		}
		dimensions = &rates.Dimensions{
			LengthCM: input.Dimensions.Length,
			WidthCM:  input.Dimensions.Width,
			HeightCM: input.Dimensions.Height,
		}
	}

	return rates.Request{
		Origin:            input.Origin,
		Destination:       input.Destination,
		Granularity:       "subdistrict",
		PriceFilter:       input.Price,
		ActualWeightGrams: input.Weight,
		Couriers:          couriers,
		Dimensions:        dimensions,
		ItemValue:         input.ItemValue,
		IncludeUnverified: input.Options.IncludeUnverified,
	}, nil
}

func attachTenantRateContext(ctx context.Context, request *rates.Request) {
	identity, ok := tenancy.FromContext(ctx)
	if !ok {
		return
	}
	request.TenantID = identity.TenantID
	request.IntegrationID = identity.IntegrationID
}

func normalizeCourierCodes(value string) ([]string, error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(value)), ":")
	unique := make(map[string]struct{}, len(parts))
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		code := strings.TrimSpace(part)
		if !validCourierCode.MatchString(code) {
			return nil, errors.New("courier berisi kode yang tidak valid")
		}
		if _, exists := unique[code]; exists {
			continue
		}
		unique[code] = struct{}{}
		result = append(result, code)
	}
	if len(result) == 0 || len(result) > 20 {
		return nil, errors.New("courier harus berisi 1 sampai 20 kode")
	}
	sort.Strings(result)
	return result, nil
}

func rateResponse(result rates.Result, calculatedAt time.Time) map[string]any {
	etdText := ""
	if result.Card.ETDMinDays != nil && result.Card.ETDMaxDays != nil {
		etdText = fmt.Sprintf("%d-%d hari", *result.Card.ETDMinDays, *result.Card.ETDMaxDays)
	}
	isStale := result.Card.ExpiresAt != nil && !calculatedAt.Before(*result.Card.ExpiresAt)
	canonicalServiceCode := result.Card.CanonicalServiceCode
	if canonicalServiceCode == "" {
		canonicalServiceCode = result.Card.ServiceCode
	}
	serviceGroup := result.Card.ServiceGroup
	if serviceGroup == "" {
		serviceGroup = "unknown"
	}

	return map[string]any{
		"courier": map[string]any{
			"code": result.Card.CourierCode,
			"name": result.Card.CourierName,
		},
		"service": map[string]any{
			"code":           result.Card.ServiceCode,
			"name":           result.Card.ServiceName,
			"canonical_code": canonicalServiceCode,
			"group":          serviceGroup,
			"type":           result.Card.ServiceType,
			"variant_code":   result.Card.ServiceVariantCode,
		},
		"cost": result.Cost.Total,
		"etd": map[string]any{
			"min_days": result.Card.ETDMinDays,
			"max_days": result.Card.ETDMaxDays,
			"text":     etdText,
		},
		"weight": map[string]any{
			"actual_grams":     result.Weight.ActualGrams,
			"volumetric_grams": result.Weight.VolumetricGrams,
			"chargeable_grams": result.Weight.ChargeableGrams,
			"rounded_grams":    result.Weight.RoundedGrams,
			"billing_grams":    result.Weight.BillingGrams,
			"minimum_grams":    result.Weight.MinimumGrams,
			"rounding_profile": result.Weight.RoundingProfile,
		},
		"breakdown": map[string]any{
			"shipping":  result.Cost.Shipping,
			"surcharge": result.Cost.Surcharge,
			"insurance": result.Cost.Insurance,
			"tax":       result.Cost.Tax,
			"total":     result.Cost.Total,
		},
		"source": map[string]any{
			"type":                result.SourceType,
			"provider":            result.Card.SourceProvider,
			"verification_status": result.Card.VerificationStatus,
			"effective_from":      result.Card.EffectiveFrom.Format("2006-01-02"),
			"fetched_at":          result.Card.FetchedAt.Format(time.RFC3339),
			"is_stale":            isStale,
		},
	}
}
