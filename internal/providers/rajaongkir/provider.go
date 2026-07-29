package rajaongkir

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/emisell/api-kurir/internal/locations"
	"github.com/emisell/api-kurir/internal/rates"
	"golang.org/x/sync/singleflight"
)

type LocationMappingStore interface {
	ResolveProviderLocation(
		ctx context.Context,
		locationPublicID string,
		providerCode string,
	) (providerLocationID string, err error)
	FindByPublicID(
		ctx context.Context,
		publicID string,
	) (locations.Location, error)
	SaveProviderMapping(
		ctx context.Context,
		locationPublicID string,
		providerCode string,
		providerLocationID string,
		providerLocationName string,
		sourceEndpoint string,
	) error
}

type Provider struct {
	client          *Client
	mappings        LocationMappingStore
	quota           rates.QuotaRepository
	credentialAlias string
	dailyLimit      int64
	snapshotTTL     time.Duration
	now             func() time.Time
	mappingGroup    singleflight.Group
}

func NewProvider(
	client *Client,
	mappings LocationMappingStore,
	quota rates.QuotaRepository,
	credentialAlias string,
	dailyLimit int64,
	snapshotTTL time.Duration,
) *Provider {
	return &Provider{
		client:          client,
		mappings:        mappings,
		quota:           quota,
		credentialAlias: credentialAlias,
		dailyLimit:      dailyLimit,
		snapshotTTL:     snapshotTTL,
		now:             time.Now,
	}
}

func (p *Provider) Code() string {
	return "rajaongkir"
}

func (p *Provider) CredentialAlias() string {
	return p.credentialAlias
}

func (p *Provider) DailyLimit() int64 {
	return p.dailyLimit
}

func (p *Provider) Quote(ctx context.Context, request rates.Request) ([]rates.ProviderQuote, error) {
	if p.quota == nil {
		return nil, rates.ErrProviderUnavailable
	}
	if err := p.quota.ConsumeProviderHit(
		ctx,
		p.Code(),
		p.credentialAlias,
		p.dailyLimit,
	); err != nil {
		return nil, err
	}
	originID, err := p.ensureMapping(
		ctx,
		request.Origin,
	)
	if err != nil {
		return nil, err
	}
	destinationID, err := p.ensureMapping(ctx, request.Destination)
	if err != nil {
		return nil, err
	}

	costRequest := DomesticCostRequest{
		Origin:      originID,
		Destination: destinationID,
		WeightGrams: request.ActualWeightGrams,
		Couriers:    request.Couriers,
		PriceFilter: request.PriceFilter,
	}
	var quotes []DomesticQuote
	if request.Granularity == "district" {
		quotes, err = p.client.CalculateDistrictDomestic(ctx, costRequest)
	} else {
		quotes, err = p.client.CalculateDomestic(ctx, costRequest)
	}
	if err != nil {
		return nil, err
	}

	fetchedAt := p.now().UTC()
	expiresAt := fetchedAt.Add(p.snapshotTTL)
	requestedCouriers := make(map[string]struct{}, len(request.Couriers))
	for _, courierCode := range request.Couriers {
		requestedCouriers[strings.ToLower(strings.TrimSpace(courierCode))] = struct{}{}
	}
	result := make([]rates.ProviderQuote, 0, len(quotes))
	for _, quote := range quotes {
		if _, requested := requestedCouriers[quote.CourierCode]; !requested {
			continue
		}
		serviceName := quote.Description
		if serviceName == "" {
			serviceName = quote.ServiceCode
		}
		result = append(result, rates.ProviderQuote{
			ProviderCode:       p.Code(),
			CourierCode:        quote.CourierCode,
			CourierName:        quote.CourierName,
			ServiceCode:        quote.ServiceCode,
			ServiceName:        serviceName,
			Description:        quote.Description,
			Cost:               quote.Cost,
			ETDMinDays:         quote.ETDMinDays,
			ETDMaxDays:         quote.ETDMaxDays,
			VerificationStatus: "observed",
			FetchedAt:          fetchedAt,
			ExpiresAt:          expiresAt,
		})
	}
	if len(result) == 0 {
		return nil, rates.ErrRateNotAvailable
	}
	return result, nil
}

func (p *Provider) ensureMapping(
	ctx context.Context,
	locationPublicID string,
) (string, error) {
	providerLocationID, err := p.mappings.ResolveProviderLocation(
		ctx,
		locationPublicID,
		p.Code(),
	)
	if err == nil {
		return providerLocationID, nil
	}
	if !errors.Is(err, locations.ErrProviderMappingNotFound) {
		return "", err
	}

	result, err, _ := p.mappingGroup.Do(locationPublicID, func() (any, error) {
		resolved, resolveErr := p.mappings.ResolveProviderLocation(
			ctx,
			locationPublicID,
			p.Code(),
		)
		if resolveErr == nil {
			return resolved, nil
		}
		if !errors.Is(resolveErr, locations.ErrProviderMappingNotFound) {
			return "", resolveErr
		}

		location, findErr := p.mappings.FindByPublicID(ctx, locationPublicID)
		if findErr != nil {
			if errors.Is(findErr, locations.ErrLocationNotFound) {
				return "", rates.ErrProviderLocationMapping
			}
			return "", findErr
		}
		if p.quota == nil {
			return "", rates.ErrProviderLocationMapping
		}
		providerID, providerName, sourceEndpoint, findErr :=
			p.findProviderLocation(ctx, location)
		if findErr != nil {
			return "", findErr
		}
		if saveErr := p.mappings.SaveProviderMapping(
			ctx,
			locationPublicID,
			p.Code(),
			providerID,
			providerName,
			sourceEndpoint,
		); saveErr != nil {
			return "", saveErr
		}
		return providerID, nil
	})
	if err != nil {
		return "", err
	}
	return result.(string), nil
}

func (p *Provider) findProviderLocation(
	ctx context.Context,
	location locations.Location,
) (string, string, string, error) {
	if err := p.consumeMappingHit(ctx); err != nil {
		return "", "", "", err
	}

	if location.Level == "subdistrict" {
		search := location.PostalCode
		if search == "" {
			search = strings.Join([]string{
				location.Subdistrict,
				location.District,
				location.City,
			}, " ")
		}
		candidates, err := p.client.SearchDestinations(ctx, search, 1000, 0)
		if err != nil {
			return "", "", "", err
		}
		candidate, err := selectExactDestination(location, candidates)
		if err == nil {
			return candidate.ID, candidate.Label, "destination/domestic-destination", nil
		}

		if location.ParentPublicID == "" {
			return "", "", "", err
		}
		parentProviderID, parentErr := p.ensureMapping(
			ctx,
			location.ParentPublicID,
		)
		if parentErr != nil {
			return "", "", "", parentErr
		}
		if quotaErr := p.consumeMappingHit(ctx); quotaErr != nil {
			return "", "", "", quotaErr
		}
		hierarchyCandidates, listErr := p.client.ListSubdistricts(
			ctx,
			parentProviderID,
		)
		if listErr != nil {
			return "", "", "", listErr
		}
		hierarchyCandidate, selectErr := selectExactHierarchyLocation(
			location.Subdistrict,
			hierarchyCandidates,
		)
		if selectErr != nil {
			return "", "", "", selectErr
		}
		return hierarchyCandidate.ID,
			hierarchyCandidate.Name,
			"destination/sub-district",
			nil
	}

	parentProviderID := ""
	if location.Level != "province" {
		if location.ParentPublicID == "" {
			return "", "", "", rates.ErrProviderLocationMapping
		}
		var err error
		parentProviderID, err = p.ensureMapping(ctx, location.ParentPublicID)
		if err != nil {
			return "", "", "", err
		}
	}

	var (
		candidates []HierarchyLocation
		endpoint   string
		err        error
	)
	switch location.Level {
	case "province":
		endpoint = "destination/province"
		candidates, err = p.client.ListProvinces(ctx)
	case "city":
		endpoint = "destination/city"
		candidates, err = p.client.ListCities(ctx, parentProviderID)
	case "district":
		endpoint = "destination/district"
		candidates, err = p.client.ListDistricts(ctx, parentProviderID)
	default:
		return "", "", "", rates.ErrProviderLocationMapping
	}
	if err != nil {
		return "", "", "", err
	}

	localName := location.Province
	if location.Level == "city" {
		localName = location.City
	} else if location.Level == "district" {
		localName = location.District
	}
	candidate, err := selectExactHierarchyLocation(localName, candidates)
	if err != nil {
		return "", "", "", err
	}
	return candidate.ID, candidate.Name, endpoint, nil
}

func (p *Provider) consumeMappingHit(ctx context.Context) error {
	if p.quota == nil {
		return rates.ErrProviderLocationMapping
	}
	return p.quota.ConsumeProviderHit(
		ctx,
		p.Code(),
		p.credentialAlias,
		p.dailyLimit,
	)
}

func selectExactHierarchyLocation(
	localName string,
	candidates []HierarchyLocation,
) (HierarchyLocation, error) {
	for _, candidate := range candidates {
		if strings.EqualFold(
			strings.TrimSpace(candidate.Name),
			strings.TrimSpace(localName),
		) {
			return candidate, nil
		}
	}

	matches := make([]HierarchyLocation, 0, 1)
	for _, candidate := range candidates {
		if normalizeAdministrativeName(candidate.Name) ==
			normalizeAdministrativeName(localName) {
			matches = append(matches, candidate)
		}
	}
	if len(matches) != 1 {
		localNormalized := normalizeAdministrativeName(localName)
		bestDistance := len([]rune(localNormalized)) + 1
		var best HierarchyLocation
		tied := false
		for _, candidate := range candidates {
			distance := levenshteinDistance(
				localNormalized,
				normalizeAdministrativeName(candidate.Name),
			)
			switch {
			case distance < bestDistance:
				bestDistance = distance
				best = candidate
				tied = false
			case distance == bestDistance:
				tied = true
			}
		}
		if len([]rune(localNormalized)) >= 8 &&
			bestDistance <= 2 &&
			!tied {
			return best, nil
		}
		return HierarchyLocation{}, rates.ErrProviderLocationMapping
	}
	return matches[0], nil
}

func levenshteinDistance(left, right string) int {
	leftRunes := []rune(left)
	rightRunes := []rune(right)
	previous := make([]int, len(rightRunes)+1)
	for index := range previous {
		previous[index] = index
	}

	for leftIndex, leftRune := range leftRunes {
		current := make([]int, len(rightRunes)+1)
		current[0] = leftIndex + 1
		for rightIndex, rightRune := range rightRunes {
			cost := 0
			if leftRune != rightRune {
				cost = 1
			}
			deletion := previous[rightIndex+1] + 1
			insertion := current[rightIndex] + 1
			substitution := previous[rightIndex] + cost
			current[rightIndex+1] = min(deletion, insertion, substitution)
		}
		previous = current
	}
	return previous[len(rightRunes)]
}

func selectExactDestination(
	location locations.Location,
	candidates []Destination,
) (Destination, error) {
	matches := make([]Destination, 0, 1)
	seen := make(map[string]struct{})
	for _, candidate := range candidates {
		if location.PostalCode != "" &&
			strings.TrimSpace(candidate.ZipCode) != location.PostalCode {
			continue
		}
		if normalizeAdministrativeName(candidate.ProvinceName) !=
			normalizeAdministrativeName(location.Province) ||
			normalizeAdministrativeName(candidate.CityName) !=
				normalizeAdministrativeName(location.City) ||
			normalizeAdministrativeName(candidate.DistrictName) !=
				normalizeAdministrativeName(location.District) ||
			normalizeAdministrativeName(candidate.SubdistrictName) !=
				normalizeAdministrativeName(location.Subdistrict) {
			continue
		}
		if _, duplicate := seen[candidate.ID]; duplicate {
			continue
		}
		seen[candidate.ID] = struct{}{}
		matches = append(matches, candidate)
	}
	if len(matches) != 1 {
		return Destination{}, rates.ErrProviderLocationMapping
	}
	return matches[0], nil
}

func normalizeAdministrativeName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if open := strings.Index(value, "("); open >= 0 {
		value = value[:open]
	}
	replacer := strings.NewReplacer(
		".", " ",
		",", " ",
		"-", " ",
		"_", " ",
		"/", " ",
		"'", " ",
	)
	value = strings.Join(strings.Fields(replacer.Replace(value)), " ")
	for _, prefix := range []string{
		"provinsi ",
		"kabupaten ",
		"kab ",
		"kota administrasi ",
		"kota ",
		"kecamatan ",
		"kelurahan ",
		"desa ",
	} {
		value = strings.TrimPrefix(value, prefix)
	}
	value = strings.Join(strings.Fields(value), " ")

	// Dataset Kemendagri dan provider memakai beberapa nama resmi/singkatan
	// berbeda untuk wilayah yang sama. Canonical form ini membuat lazy mapping
	// tetap otomatis tanpa mengandalkan input mapping dari operator.
	switch value {
	case "daerah khusus ibukota jakarta", "dki jakarta":
		return "jakarta"
	case "daerah istimewa yogyakarta", "di yogyakarta", "d i yogyakarta":
		return "yogyakarta"
	case "kep bangka belitung":
		return "kepulauan bangka belitung"
	case "kep riau":
		return "kepulauan riau"
	}

	return strings.ReplaceAll(value, "sumatra", "sumatera")
}
