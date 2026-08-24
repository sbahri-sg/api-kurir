package biteship

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/emisell/api-kurir/internal/locations"
	"github.com/emisell/api-kurir/internal/providercredentials"
	"github.com/emisell/api-kurir/internal/rates"
	"github.com/emisell/api-kurir/internal/servicecatalog"
	"github.com/emisell/api-kurir/internal/tenancy"
	"golang.org/x/sync/singleflight"
)

const biteshipRatesReference = "https://biteship.com/id/docs/api/rates/retrieve"

var durationNumberPattern = regexp.MustCompile(`\d+`)

var supportedRateCouriers = map[string]string{
	"anteraja": "anteraja",
	"ide":      "idexpress",
	"jne":      "jne",
	"jnt":      "jnt",
	"lion":     "lion",
	"ninja":    "ninja",
	"pos":      "pos",
	"rpx":      "rpx",
	"sap":      "sap",
	"sentral":  "sentralcargo",
	"sicepat":  "sicepat",
	"tiki":     "tiki",
	"wahana":   "wahana",
}

var biteshipToCanonicalCourier = func() map[string]string {
	result := make(map[string]string, len(supportedRateCouriers))
	for canonical, provider := range supportedRateCouriers {
		result[provider] = canonical
	}
	return result
}()

type canonicalService struct {
	code        string
	group       string
	serviceType string
}

var biteshipServiceAliases = map[string]map[string]canonicalService{
	"anteraja": {
		"reg": {"REG", servicecatalog.GroupRegular, servicecatalog.TypeParcel},
	},
	"ide": {
		"idtruck":       {"CARGO", servicecatalog.GroupCargo, servicecatalog.TypeCargo},
		"reg":           {"REG", servicecatalog.GroupRegular, servicecatalog.TypeParcel},
		"reg_half_kilo": {"REG", servicecatalog.GroupRegular, servicecatalog.TypeParcel},
	},
	"jne": {
		"jtr":         {"JTR", servicecatalog.GroupCargo, servicecatalog.TypeCargo},
		"jtr_150":     {"JTR", servicecatalog.GroupCargo, servicecatalog.TypeCargo},
		"jtr_150_250": {"JTR", servicecatalog.GroupCargo, servicecatalog.TypeCargo},
		"jtr_250":     {"JTR", servicecatalog.GroupCargo, servicecatalog.TypeCargo},
		"oke":         {"OKE", servicecatalog.GroupEconomy, servicecatalog.TypeParcel},
		"reg":         {"REG", servicecatalog.GroupRegular, servicecatalog.TypeParcel},
		"yes":         {"YES", servicecatalog.GroupNextDay, servicecatalog.TypeParcel},
	},
	"jnt": {
		"ez": {"EZ", servicecatalog.GroupRegular, servicecatalog.TypeParcel},
	},
	"lion": {
		"big_pack": {"BIGPACK", servicecatalog.GroupCargo, servicecatalog.TypeCargo},
		"reg_pack": {"REGPACK", servicecatalog.GroupRegular, servicecatalog.TypeParcel},
	},
	"ninja": {
		"standard": {"REG", servicecatalog.GroupRegular, servicecatalog.TypeParcel},
	},
	"pos": {
		"cargo":   {"KARGO", servicecatalog.GroupCargo, servicecatalog.TypeCargo},
		"nextday": {"NEXT_DAY", servicecatalog.GroupNextDay, servicecatalog.TypeParcel},
		"reg":     {"REGULER", servicecatalog.GroupRegular, servicecatalog.TypeParcel},
	},
	"rpx": {
		"ecp": {"ECP", servicecatalog.GroupEconomy, servicecatalog.TypeParcel},
		"hwp": {"HWP", servicecatalog.GroupCargo, servicecatalog.TypeCargo},
		"mdp": {"MDP", servicecatalog.GroupNextDay, servicecatalog.TypeParcel},
		"ndp": {"NDP", servicecatalog.GroupNextDay, servicecatalog.TypeParcel},
		"rgp": {"RGP", servicecatalog.GroupRegular, servicecatalog.TypeParcel},
	},
	"sap": {
		"cargo":         {"CARGO", servicecatalog.GroupCargo, servicecatalog.TypeCargo},
		"ods":           {"ODS", servicecatalog.GroupNextDay, servicecatalog.TypeParcel},
		"reg":           {"REG", servicecatalog.GroupRegular, servicecatalog.TypeParcel},
		"reg_half_kilo": {"REG", servicecatalog.GroupRegular, servicecatalog.TypeParcel},
	},
	"sentral": {
		"air_electronic":      {"UDARA", servicecatalog.GroupCargo, servicecatalog.TypeCargo},
		"air_non_electronic":  {"UDARA", servicecatalog.GroupCargo, servicecatalog.TypeCargo},
		"land_electronic":     {"DARAT", servicecatalog.GroupCargo, servicecatalog.TypeCargo},
		"land_non_electronic": {"DARAT", servicecatalog.GroupCargo, servicecatalog.TypeCargo},
	},
	"sicepat": {
		"best":  {"BEST", servicecatalog.GroupNextDay, servicecatalog.TypeParcel},
		"gokil": {"GOKIL", servicecatalog.GroupCargo, servicecatalog.TypeCargo},
		"reg":   {"REGULER", servicecatalog.GroupRegular, servicecatalog.TypeParcel},
	},
	"tiki": {
		"eko": {"ECO", servicecatalog.GroupEconomy, servicecatalog.TypeParcel},
		"ons": {"ONS", servicecatalog.GroupNextDay, servicecatalog.TypeParcel},
		"reg": {"REG", servicecatalog.GroupRegular, servicecatalog.TypeParcel},
		"t15": {"TRC", servicecatalog.GroupCargo, servicecatalog.TypeCargo},
		"t25": {"TRC", servicecatalog.GroupCargo, servicecatalog.TypeCargo},
		"t60": {"TRC", servicecatalog.GroupCargo, servicecatalog.TypeCargo},
		"trc": {"TRC", servicecatalog.GroupCargo, servicecatalog.TypeCargo},
	},
	"wahana": {
		"deno": {"EXPRESS", servicecatalog.GroupRegular, servicecatalog.TypeParcel},
	},
}

type RateLocationStore interface {
	ResolveProviderLocation(
		ctx context.Context,
		locationPublicID string,
		providerCode string,
	) (providerLocationID string, err error)
	FindByPublicID(ctx context.Context, publicID string) (locations.Location, error)
	SaveProviderMapping(
		ctx context.Context,
		locationPublicID string,
		providerCode string,
		providerLocationID string,
		providerLocationName string,
		sourceEndpoint string,
	) error
}

type DynamicRateProvider struct {
	resolver    CredentialResolver
	baseURL     string
	timeout     time.Duration
	mappings    RateLocationStore
	quota       rates.QuotaRepository
	recorder    ProviderCallRecorder
	snapshotTTL time.Duration
	now         func() time.Time
	mapping     singleflight.Group
	clientMu    sync.Mutex
	clients     map[string]*Client
}

func NewDynamicRateProvider(
	resolver CredentialResolver,
	baseURL string,
	timeout time.Duration,
	mappings RateLocationStore,
	quota rates.QuotaRepository,
	recorder ProviderCallRecorder,
	snapshotTTL time.Duration,
) *DynamicRateProvider {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if snapshotTTL <= 0 {
		snapshotTTL = 14 * 24 * time.Hour
	}
	return &DynamicRateProvider{
		resolver: resolver, baseURL: baseURL, timeout: timeout,
		mappings: mappings, quota: quota, recorder: recorder,
		snapshotTTL: snapshotTTL, now: time.Now,
		clients: make(map[string]*Client),
	}
}

func (p *DynamicRateProvider) Code() string { return "biteship" }

func (p *DynamicRateProvider) Quote(
	ctx context.Context,
	request rates.Request,
) ([]rates.ProviderQuote, error) {
	if p.resolver == nil || p.mappings == nil || p.quota == nil {
		return nil, rates.ErrProviderUnavailable
	}
	providerCouriers, requested := biteshipRateCouriers(request.Couriers)
	if len(providerCouriers) == 0 {
		return nil, rates.ErrRateNotAvailable
	}

	platformCtx := tenancy.WithoutIdentity(ctx)
	secret, alias, limit, err := p.resolver.ResolveProviderCredential(
		platformCtx,
		p.Code(),
	)
	if errors.Is(err, providercredentials.ErrNoActiveCredential) {
		return nil, rates.ErrRateNotAvailable
	}
	if errors.Is(err, providercredentials.ErrAllCredentialsExhausted) {
		return nil, rates.ErrProviderQuotaExhausted
	}
	if err != nil {
		return nil, rates.ErrProviderUnavailable
	}
	client := p.client(alias, secret)

	originID, err := p.ensureMapping(ctx, platformCtx, client, alias, limit, request.Origin)
	if err != nil {
		return nil, err
	}
	destinationID, err := p.ensureMapping(
		ctx,
		platformCtx,
		client,
		alias,
		limit,
		request.Destination,
	)
	if err != nil {
		return nil, err
	}

	if err := p.consume(platformCtx, alias, limit); err != nil {
		p.record(ctx, alias, courierRatesEndpoint, rateFingerprint(request), 0, "client_error", 0, 0, "LOCAL_QUOTA_EXHAUSTED")
		return nil, err
	}
	itemValue := request.ItemValue
	if itemValue <= 0 {
		itemValue = 1
	}
	item := RateItem{
		Name: "Emisell package", Description: "Emisell checkout rate fallback",
		Category: "others", Value: itemValue, Quantity: 1,
		Weight: request.ActualWeightGrams,
	}
	if request.Dimensions != nil {
		item.Length = request.Dimensions.LengthCM
		item.Width = request.Dimensions.WidthCM
		item.Height = request.Dimensions.HeightCM
	}
	startedAt := p.now()
	response, err := client.RetrieveCourierRates(ctx, CourierRateRequest{
		OriginAreaID: originID, DestinationAreaID: destinationID,
		Couriers: strings.Join(providerCouriers, ","),
		Items:    []RateItem{item},
	})
	duration := p.now().Sub(startedAt)
	if err != nil {
		mapped := mapRateError(err)
		p.recordProviderError(ctx, alias, courierRatesEndpoint, rateFingerprint(request), duration, err, mapped)
		return nil, mapped
	}
	p.record(ctx, alias, courierRatesEndpoint, rateFingerprint(request), http.StatusOK, "success", duration, 1, "")

	fetchedAt := p.now().UTC()
	quotes := make([]rates.ProviderQuote, 0, len(response.Pricing))
	for _, pricing := range response.Pricing {
		providerCourier := strings.ToLower(strings.TrimSpace(pricing.CourierCode))
		if providerCourier == "" {
			providerCourier = strings.ToLower(strings.TrimSpace(pricing.Company))
		}
		courierCode := biteshipToCanonicalCourier[providerCourier]
		if _, allowed := requested[courierCode]; !allowed {
			continue
		}
		rawService := strings.ToLower(strings.TrimSpace(pricing.CourierServiceCode))
		service, supported := biteshipServiceAliases[courierCode][rawService]
		if !supported {
			continue
		}
		cost := pricing.Price
		if cost <= 0 {
			cost = pricing.ShippingFee
		}
		if cost < 0 {
			return nil, rates.ErrProviderUnavailable
		}
		minDays, maxDays := biteshipETD(pricing)
		quotes = append(quotes, rates.ProviderQuote{
			ProviderCode: p.Code(), CourierCode: courierCode,
			CourierName:          strings.TrimSpace(pricing.CourierName),
			ServiceCode:          strings.ToUpper(rawService),
			ServiceName:          strings.TrimSpace(pricing.CourierServiceName),
			Description:          strings.TrimSpace(pricing.Description),
			CanonicalServiceCode: service.code,
			ServiceGroup:         service.group, ServiceType: service.serviceType,
			ServiceVariantCode:      strings.ToUpper(rawService),
			ClassificationSource:    "official_public",
			ClassificationReference: biteshipRatesReference,
			Cost:                    cost, ETDMinDays: minDays, ETDMaxDays: maxDays,
			VerificationStatus: "observed", FetchedAt: fetchedAt,
			ExpiresAt: fetchedAt.Add(p.snapshotTTL),
		})
	}
	if len(quotes) == 0 {
		return nil, rates.ErrRateNotAvailable
	}
	return quotes, nil
}

func (p *DynamicRateProvider) client(alias, secret string) *Client {
	p.clientMu.Lock()
	defer p.clientMu.Unlock()
	if client := p.clients[alias]; client != nil {
		return client
	}
	client := NewClient(p.baseURL, secret, p.timeout)
	p.clients[alias] = client
	return client
}

func (p *DynamicRateProvider) ensureMapping(
	requestCtx context.Context,
	platformCtx context.Context,
	client *Client,
	alias string,
	limit int64,
	publicID string,
) (string, error) {
	providerID, err := p.mappings.ResolveProviderLocation(
		requestCtx,
		publicID,
		p.Code(),
	)
	if err == nil {
		return providerID, nil
	}
	if !errors.Is(err, locations.ErrProviderMappingNotFound) {
		return "", err
	}

	value, err, _ := p.mapping.Do(publicID, func() (any, error) {
		resolved, resolveErr := p.mappings.ResolveProviderLocation(
			requestCtx,
			publicID,
			p.Code(),
		)
		if resolveErr == nil {
			return resolved, nil
		}
		if !errors.Is(resolveErr, locations.ErrProviderMappingNotFound) {
			return "", resolveErr
		}
		location, findErr := p.mappings.FindByPublicID(requestCtx, publicID)
		if findErr != nil {
			return "", rates.ErrProviderLocationMapping
		}
		if location.Level != "district" && location.Level != "subdistrict" {
			return "", rates.ErrProviderLocationMapping
		}
		if consumeErr := p.consume(platformCtx, alias, limit); consumeErr != nil {
			return "", consumeErr
		}
		search := biteshipLocationSearch(location)
		startedAt := p.now()
		response, searchErr := client.SearchAreas(requestCtx, search)
		duration := p.now().Sub(startedAt)
		fingerprint := textFingerprint(search)
		if searchErr != nil {
			mapped := mapRateError(searchErr)
			p.recordProviderError(requestCtx, alias, mapsAreasEndpoint, fingerprint, duration, searchErr, mapped)
			return "", mapped
		}
		p.record(requestCtx, alias, mapsAreasEndpoint, fingerprint, http.StatusOK, "success", duration, 1, "")
		area, selectErr := selectBiteshipArea(location, response.Areas)
		if selectErr != nil {
			return "", selectErr
		}
		if saveErr := p.mappings.SaveProviderMapping(
			requestCtx,
			publicID,
			p.Code(),
			area.ID,
			area.Name,
			mapsAreasEndpoint,
		); saveErr != nil {
			return "", saveErr
		}
		return area.ID, nil
	})
	if err != nil {
		return "", err
	}
	return value.(string), nil
}

func (p *DynamicRateProvider) consume(
	ctx context.Context,
	alias string,
	limit int64,
) error {
	if err := p.quota.ConsumeProviderHit(ctx, p.Code(), alias, limit); err != nil {
		if errors.Is(err, rates.ErrProviderQuotaExhausted) {
			return rates.ErrProviderQuotaExhausted
		}
		return rates.ErrProviderUnavailable
	}
	return nil
}

func (p *DynamicRateProvider) recordProviderError(
	ctx context.Context,
	alias, endpoint, fingerprint string,
	duration time.Duration,
	providerErr error,
	mapped error,
) {
	status := 0
	var apiError *APIError
	if errors.As(providerErr, &apiError) {
		status = apiError.StatusCode
	}
	p.record(ctx, alias, endpoint, fingerprint, status, "provider_error", duration, 1, rateErrorCode(mapped))
}

func (p *DynamicRateProvider) record(
	ctx context.Context,
	alias, endpoint, fingerprint string,
	status int,
	outcome string,
	duration time.Duration,
	quotaCost int,
	errorCode string,
) {
	if p.recorder == nil {
		return
	}
	_ = p.recorder.RecordProviderAPICall(
		ctx,
		p.Code(),
		alias,
		endpoint,
		fingerprint,
		status,
		outcome,
		duration,
		quotaCost,
		errorCode,
	)
}

func biteshipRateCouriers(input []string) ([]string, map[string]struct{}) {
	seenProvider := make(map[string]struct{})
	requested := make(map[string]struct{})
	providerCodes := make([]string, 0, len(input))
	for _, value := range input {
		canonical := strings.ToLower(strings.TrimSpace(value))
		providerCode, supported := supportedRateCouriers[canonical]
		if !supported {
			continue
		}
		requested[canonical] = struct{}{}
		if _, duplicate := seenProvider[providerCode]; duplicate {
			continue
		}
		seenProvider[providerCode] = struct{}{}
		providerCodes = append(providerCodes, providerCode)
	}
	sort.Strings(providerCodes)
	return providerCodes, requested
}

func biteshipLocationSearch(location locations.Location) string {
	parts := make([]string, 0, 4)
	if location.PostalCode != "" {
		parts = append(parts, location.PostalCode)
	}
	for _, value := range []string{location.District, location.City, location.Province} {
		if strings.TrimSpace(value) != "" {
			parts = append(parts, strings.TrimSpace(value))
		}
	}
	return strings.Join(parts, " ")
}

func selectBiteshipArea(
	location locations.Location,
	areas []Area,
) (Area, error) {
	postalCodes := make(map[string]struct{}, len(location.PostalCodes)+1)
	if location.PostalCode != "" {
		postalCodes[location.PostalCode] = struct{}{}
	}
	for _, postalCode := range location.PostalCodes {
		if postalCode = strings.TrimSpace(postalCode); postalCode != "" {
			postalCodes[postalCode] = struct{}{}
		}
	}
	matches := make([]Area, 0, len(areas))
	for _, area := range areas {
		if strings.TrimSpace(area.ID) == "" ||
			(area.CountryCode != "" && !strings.EqualFold(area.CountryCode, "ID")) {
			continue
		}
		if !sameAdministrativeName(location.Province, area.ProvinceName) ||
			!sameAdministrativeName(location.City, area.CityName) ||
			!sameAdministrativeName(location.District, area.DistrictName) {
			continue
		}
		if len(postalCodes) > 0 {
			if _, allowed := postalCodes[area.PostalCodeText()]; !allowed {
				continue
			}
		}
		matches = append(matches, area)
	}
	if len(matches) == 0 {
		return Area{}, rates.ErrProviderLocationMapping
	}
	sort.Slice(matches, func(left, right int) bool {
		if matches[left].PostalCode == matches[right].PostalCode {
			return matches[left].ID < matches[right].ID
		}
		return matches[left].PostalCode < matches[right].PostalCode
	})
	return matches[0], nil
}

func sameAdministrativeName(local, provider string) bool {
	local = normalizeAdministrativeName(local)
	provider = normalizeAdministrativeName(provider)
	return local != "" && provider != "" && local == provider
}

func normalizeAdministrativeName(value string) string {
	var token strings.Builder
	for _, character := range strings.ToUpper(strings.TrimSpace(value)) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			token.WriteRune(character)
		}
	}
	normalized := token.String()
	switch normalized {
	case "DAERAHKHUSUSIBUKOTAJAKARTA":
		normalized = "DKIJAKARTA"
	case "DAERAHISTIMEWAYOGYAKARTA":
		normalized = "DIYOGYAKARTA"
	}
	for _, prefix := range []string{
		"KOTAADMINISTRASI",
		"KABUPATEN",
		"KOTA",
		"KAB",
	} {
		if strings.HasPrefix(normalized, prefix) && len(normalized) > len(prefix) {
			return strings.TrimPrefix(normalized, prefix)
		}
	}
	return normalized
}

func biteshipETD(pricing CourierPricing) (*int, *int) {
	unit := strings.ToLower(strings.TrimSpace(pricing.ShipmentDurationUnit))
	if unit != "day" && unit != "days" && unit != "hari" {
		return nil, nil
	}
	values := durationNumberPattern.FindAllString(pricing.ShipmentDurationRange, -1)
	if len(values) == 0 {
		values = durationNumberPattern.FindAllString(pricing.Duration, -1)
	}
	if len(values) == 0 {
		return nil, nil
	}
	minimum := parsePositiveInt(values[0])
	maximum := minimum
	if len(values) > 1 {
		maximum = parsePositiveInt(values[1])
	}
	if minimum <= 0 || maximum <= 0 {
		return nil, nil
	}
	return &minimum, &maximum
}

func parsePositiveInt(value string) int {
	result := 0
	for _, character := range value {
		if character < '0' || character > '9' {
			continue
		}
		result = result*10 + int(character-'0')
	}
	return result
}

func mapRateError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return rates.ErrProviderUnavailable
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return rates.ErrProviderUnavailable
	}
	var apiError *APIError
	if !errors.As(err, &apiError) {
		return rates.ErrProviderUnavailable
	}
	message := strings.ToLower(apiError.Message)
	switch {
	case apiError.StatusCode == http.StatusUnauthorized || apiError.StatusCode == http.StatusForbidden:
		return rates.ErrProviderUnauthorized
	case apiError.StatusCode == http.StatusTooManyRequests:
		return rates.ErrProviderRateLimited
	case strings.Contains(message, "balance") || strings.Contains(message, "saldo"):
		return rates.ErrProviderQuotaExhausted
	case strings.Contains(message, "not available") ||
		strings.Contains(message, "not found") ||
		strings.Contains(message, "no rate") ||
		strings.Contains(message, "no courier"):
		return rates.ErrRateNotAvailable
	default:
		return rates.ErrProviderUnavailable
	}
}

func rateErrorCode(err error) string {
	switch {
	case errors.Is(err, rates.ErrProviderUnauthorized):
		return "PROVIDER_UNAUTHORIZED"
	case errors.Is(err, rates.ErrProviderRateLimited):
		return "PROVIDER_RATE_LIMITED"
	case errors.Is(err, rates.ErrProviderQuotaExhausted):
		return "PROVIDER_QUOTA_EXHAUSTED"
	case errors.Is(err, rates.ErrRateNotAvailable):
		return "RATE_NOT_AVAILABLE"
	case errors.Is(err, rates.ErrProviderLocationMapping):
		return "PROVIDER_LOCATION_NOT_MAPPED"
	default:
		return "PROVIDER_UNAVAILABLE"
	}
}

func rateFingerprint(request rates.Request) string {
	couriers := append([]string(nil), request.Couriers...)
	sort.Strings(couriers)
	return textFingerprint(strings.Join([]string{
		request.TenantID,
		request.Origin,
		request.Destination,
		request.Granularity,
		strings.Join(couriers, ","),
		parseInt64Text(request.ActualWeightGrams),
	}, "\x1f"))
}

func parseInt64Text(value int64) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	buffer := make([]byte, 0, 20)
	for value > 0 {
		buffer = append(buffer, byte('0'+value%10))
		value /= 10
	}
	if negative {
		buffer = append(buffer, '-')
	}
	for left, right := 0, len(buffer)-1; left < right; left, right = left+1, right-1 {
		buffer[left], buffer[right] = buffer[right], buffer[left]
	}
	return string(buffer)
}

func textFingerprint(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
