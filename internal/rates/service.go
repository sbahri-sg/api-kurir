package rates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/emisell/api-kurir/internal/couriers"
	"github.com/emisell/api-kurir/internal/platform/cache"
	"golang.org/x/sync/singleflight"
)

var (
	ErrRateNotAvailable = errors.New("rate not available")
	ErrShippingDisabled = errors.New("shipping is not active for tenant")
)

type Service struct {
	repository   Repository
	policies     ServicePolicyRepository
	policyCache  cache.Cache
	policyTTL    time.Duration
	queryTimeout time.Duration
	group        singleflight.Group
	provider     QuoteProvider
	fallbacks    []QuoteProvider
	snapshots    SnapshotRepository
	locker       cache.Locker
	lockTTL      time.Duration
	resultPolicy ResultPolicy
	credentials  CredentialSelector
	providerGate ShippingProviderGate
	fallbackGate ProviderFallbackPolicy
}

type ShippingProviderGate interface {
	HasActiveProvider(ctx context.Context, tenantID string) (bool, error)
}

type ProviderFallbackPolicy interface {
	AllowsProviderFallback(ctx context.Context) (bool, error)
}

type CredentialSelector interface {
	SelectedCredentialID(
		ctx context.Context,
		tenantID, providerCode string,
	) (string, error)
}

type ResultPolicy interface {
	Filter(
		ctx context.Context,
		tenantID string,
		results []Result,
	) ([]Result, error)
}

type Option func(*Service)

func WithProviderFallback(
	provider QuoteProvider,
	snapshots SnapshotRepository,
	locker cache.Locker,
	lockTTL time.Duration,
) Option {
	return func(service *Service) {
		service.provider = provider
		service.fallbacks = nil
		service.snapshots = snapshots
		service.locker = locker
		if lockTTL > 0 {
			service.lockTTL = lockTTL
		}
	}
}

func WithProviderFallbackChain(
	primary QuoteProvider,
	snapshots SnapshotRepository,
	locker cache.Locker,
	lockTTL time.Duration,
	fallbackPolicy ProviderFallbackPolicy,
	fallbackProviders ...QuoteProvider,
) Option {
	return func(service *Service) {
		service.provider = primary
		service.fallbacks = append([]QuoteProvider(nil), fallbackProviders...)
		service.snapshots = snapshots
		service.locker = locker
		service.fallbackGate = fallbackPolicy
		if lockTTL > 0 {
			service.lockTTL = lockTTL
		}
	}
}

func WithResultPolicy(policy ResultPolicy) Option {
	return func(service *Service) {
		service.resultPolicy = policy
	}
}

func WithCredentialSelector(selector CredentialSelector) Option {
	return func(service *Service) {
		service.credentials = selector
	}
}

func WithShippingProviderGate(gate ShippingProviderGate) Option {
	return func(service *Service) {
		service.providerGate = gate
	}
}

func WithServicePolicyCache(policyCache cache.Cache, ttl time.Duration) Option {
	return func(service *Service) {
		service.policyCache = policyCache
		if ttl > 0 {
			service.policyTTL = ttl
		}
	}
}

func NewService(repository Repository, queryTimeout time.Duration, options ...Option) *Service {
	if queryTimeout <= 0 {
		queryTimeout = 5 * time.Second
	}
	service := &Service{
		repository:   repository,
		queryTimeout: queryTimeout,
		lockTTL:      10 * time.Second,
		policyTTL:    5 * time.Minute,
	}
	if policies, ok := repository.(ServicePolicyRepository); ok {
		service.policies = policies
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) Calculate(ctx context.Context, request Request) ([]Result, error) {
	request.Couriers = normalizeRequestedCouriers(request.Couriers)
	key := requestKey(request)
	resultCh := s.group.DoChan(key, func() (any, error) {
		operationCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.queryTimeout)
		defer cancel()
		if request.TenantID != "" && s.providerGate != nil {
			active, err := s.providerGate.HasActiveProvider(operationCtx, request.TenantID)
			if err != nil {
				return nil, err
			}
			if !active {
				return nil, ErrShippingDisabled
			}
		}
		return s.calculate(operationCtx, request, key)
	})

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		results := result.Val.([]Result)
		if request.TenantID != "" && s.resultPolicy != nil {
			filtered, err := s.resultPolicy.Filter(ctx, request.TenantID, results)
			if err != nil {
				return nil, err
			}
			if len(filtered) == 0 {
				return nil, ErrRateNotAvailable
			}
			results = filtered
		}
		return results, nil
	}
}

func normalizeRequestedCouriers(values []string) []string {
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = couriers.NormalizeCode(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}

func (s *Service) calculate(ctx context.Context, request Request, key string) ([]Result, error) {
	policies, err := s.loadServicePolicies(ctx, request.Couriers)
	if err != nil {
		return nil, err
	}
	courierPresentations, err := s.loadCourierPresentations(ctx, request.Couriers)
	if err != nil {
		return nil, err
	}

	cards, err := s.repository.FindActiveRateCards(ctx, request)
	if err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(cards))
	coveredCouriers := make(map[string]struct{}, len(cards))
	for _, card := range cards {
		card = applyCourierPresentation(card, courierPresentations[card.CourierCode])
		var evaluation *PolicyEvaluation
		if policy, ok := policies.match(card); ok {
			checked := evaluateServicePolicy(request, policy)
			if !checked.Eligible {
				continue
			}
			evaluation = &checked
			card = applyPolicyToRateCard(card, policy)
		}
		result, err := Calculate(request, card)
		if errors.Is(err, ErrWeightExceeded) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("calculate %s %s: %w", card.CourierCode, card.ServiceCode, err)
		}
		coveredCouriers[card.CourierCode] = struct{}{}
		if evaluation != nil {
			result = attachPolicyEvaluation(result, *evaluation)
		}
		results = append(results, result)
	}

	missingCouriers := make([]string, 0, len(request.Couriers))
	for _, courier := range request.Couriers {
		if _, covered := coveredCouriers[courier]; !covered {
			missingCouriers = append(missingCouriers, courier)
		}
	}
	if len(missingCouriers) > 0 {
		fallbackRequest := request
		fallbackRequest.Couriers = missingCouriers
		providerResults, providerErr := s.calculateProviderFallback(ctx, fallbackRequest, key)
		if providerErr == nil {
			for _, result := range providerResults {
				result.Card = applyCourierPresentation(
					result.Card,
					courierPresentations[result.Card.CourierCode],
				)
				if policy, ok := policies.match(result.Card); ok {
					evaluation := evaluateServicePolicy(request, policy)
					if !evaluation.Eligible {
						continue
					}
					result = attachPolicyEvaluation(result, evaluation)
				}
				results = append(results, result)
			}
		} else if len(results) == 0 || !errors.Is(providerErr, ErrRateNotAvailable) {
			return nil, providerErr
		}
	}
	if len(results) == 0 {
		return nil, ErrRateNotAvailable
	}
	sortResults(results)
	if request.PriceFilter == "highest" {
		for left, right := 0, len(results)-1; left < right; left, right = left+1, right-1 {
			results[left], results[right] = results[right], results[left]
		}
	}
	return results, nil
}

func (s *Service) loadCourierPresentations(
	ctx context.Context,
	courierCodes []string,
) (map[string]CourierPresentation, error) {
	if repository, ok := s.repository.(CourierPresentationRepository); ok {
		return repository.FindActiveCourierPresentations(ctx, courierCodes)
	}
	legacy, ok := s.repository.(CourierLogoRepository)
	if !ok {
		return nil, nil
	}
	logos, err := legacy.FindActiveCourierLogos(ctx, courierCodes)
	if err != nil {
		return nil, err
	}
	presentations := make(map[string]CourierPresentation, len(logos))
	for courierCode, logo := range logos {
		presentations[courierCode] = CourierPresentation{Logo: logo}
	}
	return presentations, nil
}

func applyCourierPresentation(
	card RateCard,
	presentation CourierPresentation,
) RateCard {
	if presentation.Name != "" {
		card.CourierName = presentation.Name
	}
	if presentation.Logo != "" {
		card.CourierLogo = presentation.Logo
	}
	serviceCode := strings.ToUpper(strings.TrimSpace(card.CanonicalServiceCode))
	if serviceCode == "" {
		serviceCode = strings.ToUpper(strings.TrimSpace(card.ServiceCode))
	}
	serviceName := strings.TrimSpace(card.ServiceName)
	if masterName := presentation.ServiceNames[serviceCode]; masterName != "" {
		serviceName = masterName
	}
	card.ServiceName = conciseServiceName(
		card.CourierCode,
		card.CourierName,
		serviceCode,
		serviceName,
	)
	return card
}

func conciseServiceName(
	courierCode,
	courierName,
	serviceCode,
	serviceName string,
) string {
	courierCode = strings.ToUpper(strings.TrimSpace(courierCode))
	courierName = strings.TrimSpace(courierName)
	serviceCode = strings.ToUpper(strings.TrimSpace(serviceCode))
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		return serviceDisplayFromCode(courierCode, serviceCode)
	}

	prefixes := []string{courierName, courierCode}
	if open := strings.LastIndex(courierName, "("); open >= 0 && strings.HasSuffix(courierName, ")") {
		prefixes = append(prefixes, strings.TrimSpace(courierName[open+1:len(courierName)-1]))
	}
	for _, suffix := range []string{" Express", " Xpress", " Parcel"} {
		if len(courierName) > len(suffix) && strings.HasSuffix(strings.ToLower(courierName), strings.ToLower(suffix)) {
			prefixes = append(prefixes, strings.TrimSpace(courierName[:len(courierName)-len(suffix)]))
		}
	}
	for _, prefix := range prefixes {
		if prefix == "" || len(serviceName) < len(prefix) || !strings.EqualFold(serviceName[:len(prefix)], prefix) {
			continue
		}
		serviceName = strings.TrimLeft(serviceName[len(prefix):], " \t-–—:|/")
		break
	}

	serviceName = strings.TrimSpace(serviceName)
	if len(serviceName) >= len("Layanan ") && strings.EqualFold(serviceName[:len("Layanan ")], "Layanan ") {
		serviceName = strings.TrimSpace(serviceName[len("Layanan "):])
	}
	if serviceName == "" || strings.EqualFold(serviceName, courierName) {
		return serviceDisplayFromCode(courierCode, serviceCode)
	}
	switch strings.ToUpper(serviceName) {
	case "REGULAR", "REGULER", "REGULAR SERVICE", "REGULER SERVICE", "NORMAL":
		return "Regular"
	case "ECONOMY", "ECONOMY SERVICE", "EKONOMI", "EKONOMIS":
		return "Economy"
	case "NEXT DAY", "NEXTDAY":
		return "Next Day"
	case "MINI CARGO", "MINI KARGO":
		return "Mini Cargo"
	default:
		return serviceName
	}
}

func serviceDisplayFromCode(courierCode, serviceCode string) string {
	courierCode = strings.ToUpper(strings.TrimSpace(courierCode))
	serviceCode = strings.ToUpper(strings.TrimSpace(serviceCode))
	if courierCode == "WAHANA" && serviceCode == "EXPRESS" {
		return "Regular"
	}
	if strings.HasPrefix(serviceCode, "JTR") {
		return "Trucking"
	}
	switch serviceCode {
	case "REG", "REGULER", "NORMAL":
		return "Regular"
	case "DOK", "DOC":
		return "Document"
	case "ECO", "EKONOMI":
		return "Economy"
	case "MIC":
		return "Mini Cargo"
	case "ND", "NEXT_DAY", "NEXTDAY":
		return "Next Day"
	default:
		return serviceCode
	}
}

func (s *Service) loadServicePolicies(
	ctx context.Context,
	courierCodes []string,
) (policySet, error) {
	if s.policies == nil {
		return newPolicySet(nil), nil
	}
	cacheKey := "service-weight-policies:" + strings.Join(courierCodes, ":")
	if s.policyCache != nil {
		payload, found, err := s.policyCache.Get(ctx, cacheKey)
		if err == nil && found {
			var items []ServicePolicy
			if json.Unmarshal(payload, &items) == nil {
				return newPolicySet(items), nil
			}
		}
	}

	items, err := s.policies.FindActiveServicePolicies(ctx, courierCodes)
	if err != nil {
		return policySet{}, err
	}
	if s.policyCache != nil {
		if payload, marshalErr := json.Marshal(items); marshalErr == nil {
			_ = s.policyCache.Set(ctx, cacheKey, payload, s.policyTTL)
		}
	}
	return newPolicySet(items), nil
}

func (s *Service) calculateProviderFallback(
	ctx context.Context,
	request Request,
	_ string,
) ([]Result, error) {
	if s.provider == nil || s.snapshots == nil {
		return nil, ErrRateNotAvailable
	}
	if request.TenantID != "" && s.credentials != nil {
		credentialID, err := s.credentials.SelectedCredentialID(
			ctx,
			request.TenantID,
			s.provider.Code(),
		)
		if err != nil {
			return nil, err
		}
		// An empty credential ID represents the built-in Emisell provider.
		// The provider resolver authorizes that tenant and selects the shared
		// platform pool. BYOK integrations still return their pinned ID here.
		if credentialID != "" {
			request.ProviderCredentialID = credentialID
		}
	}

	primaryQuotes, primaryErr := s.fetchProviderQuotes(ctx, request, s.provider)
	if primaryErr != nil && !allowsRateProviderFallback(primaryErr) {
		return nil, primaryErr
	}
	allQuotes := append([]ProviderQuote(nil), primaryQuotes...)
	missingCouriers := missingProviderQuoteCouriers(request.Couriers, allQuotes)
	if primaryErr == nil && len(missingCouriers) == 0 {
		return providerQuoteResults(request, allQuotes), nil
	}

	allowed, err := s.fallbackAllowed(ctx)
	if err != nil {
		return nil, err
	}
	if !allowed || len(s.fallbacks) == 0 {
		if primaryErr != nil {
			return nil, primaryErr
		}
		if len(allQuotes) == 0 {
			return nil, ErrRateNotAvailable
		}
		return providerQuoteResults(request, allQuotes), nil
	}

	if len(missingCouriers) == 0 {
		missingCouriers = append([]string(nil), request.Couriers...)
	}
	var fallbackErr error
	for _, fallback := range s.fallbacks {
		if fallback == nil || len(missingCouriers) == 0 {
			continue
		}
		fallbackRequest := request
		// Biteship is a shared platform fallback for Emisell Kurir. Its quote
		// snapshot is reusable across merchants; tenant authorization has already
		// been enforced above and audit calls still retain the context identity.
		fallbackRequest.TenantID = ""
		fallbackRequest.ProviderCredentialID = ""
		fallbackRequest.Couriers = append([]string(nil), missingCouriers...)
		quotes, quoteErr := s.fetchProviderQuotes(ctx, fallbackRequest, fallback)
		if quoteErr != nil {
			fallbackErr = quoteErr
			continue
		}
		allQuotes = append(allQuotes, quotes...)
		missingCouriers = missingProviderQuoteCouriers(
			missingCouriers,
			quotes,
		)
	}
	if len(allQuotes) > 0 {
		return providerQuoteResults(request, allQuotes), nil
	}
	if primaryErr != nil {
		return nil, primaryErr
	}
	if fallbackErr != nil {
		return nil, fallbackErr
	}
	return nil, ErrRateNotAvailable
}

func (s *Service) fetchProviderQuotes(
	ctx context.Context,
	request Request,
	provider QuoteProvider,
) ([]ProviderQuote, error) {
	if provider == nil {
		return nil, ErrRateNotAvailable
	}
	providerCode := provider.Code()
	quotes, err := s.snapshots.FindFreshProviderQuotes(ctx, request, providerCode)
	if err != nil {
		return nil, err
	}
	if len(quotes) > 0 {
		return quotes, nil
	}

	fetch := func(lockCtx context.Context) error {
		fresh, err := s.snapshots.FindFreshProviderQuotes(lockCtx, request, providerCode)
		if err != nil {
			return err
		}
		if len(fresh) > 0 {
			quotes = fresh
			return nil
		}
		fresh, err = provider.Quote(lockCtx, request)
		if err != nil {
			return err
		}
		if len(fresh) == 0 {
			return ErrRateNotAvailable
		}
		for index := range fresh {
			if fresh[index].ProviderCode == "" {
				fresh[index].ProviderCode = providerCode
			}
		}
		if err := s.snapshots.SaveProviderQuotes(lockCtx, request, fresh); err != nil {
			return err
		}
		quotes = fresh
		return nil
	}

	lockKey := "provider-rate:" + providerCode + ":" + requestKey(request)
	if s.locker == nil {
		err = fetch(ctx)
	} else {
		err = s.locker.WithLock(ctx, lockKey, s.lockTTL, fetch)
	}
	if errors.Is(err, cache.ErrLockNotAcquired) {
		quotes, err = s.waitForProviderSnapshot(ctx, request, providerCode)
	}
	return quotes, err
}

func (s *Service) fallbackAllowed(ctx context.Context) (bool, error) {
	if s.fallbackGate == nil {
		return true, nil
	}
	return s.fallbackGate.AllowsProviderFallback(ctx)
}

func allowsRateProviderFallback(err error) bool {
	return errors.Is(err, ErrRateNotAvailable) ||
		errors.Is(err, ErrProviderUnavailable) ||
		errors.Is(err, ErrProviderQuotaExhausted) ||
		errors.Is(err, ErrProviderRateLimited) ||
		errors.Is(err, ErrProviderUnauthorized) ||
		errors.Is(err, ErrProviderLocationMapping)
}

func missingProviderQuoteCouriers(
	requested []string,
	quotes []ProviderQuote,
) []string {
	covered := make(map[string]struct{}, len(quotes))
	for _, quote := range quotes {
		covered[couriers.NormalizeCode(quote.CourierCode)] = struct{}{}
	}
	missing := make([]string, 0, len(requested))
	for _, courier := range requested {
		courier = couriers.NormalizeCode(courier)
		if _, exists := covered[courier]; !exists && courier != "" {
			missing = append(missing, courier)
		}
	}
	return missing
}

func (s *Service) waitForProviderSnapshot(
	ctx context.Context,
	request Request,
	providerCode string,
) ([]ProviderQuote, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w: waiting for provider refresh", ErrProviderUnavailable)
		case <-ticker.C:
			quotes, err := s.snapshots.FindFreshProviderQuotes(ctx, request, providerCode)
			if err != nil {
				return nil, err
			}
			if len(quotes) > 0 {
				return quotes, nil
			}
		}
	}
}

func providerQuoteResults(request Request, quotes []ProviderQuote) []Result {
	results := make([]Result, 0, len(quotes))
	for _, quote := range quotes {
		results = append(results, resultFromProviderQuote(request, quote))
	}
	return results
}

func sortResults(results []Result) {
	sort.Slice(results, func(i, j int) bool {
		if results[i].Cost.Total == results[j].Cost.Total {
			return results[i].Card.ServiceCode < results[j].Card.ServiceCode
		}
		return results[i].Cost.Total < results[j].Cost.Total
	})
}

func requestKey(request Request) string {
	couriers := append([]string(nil), request.Couriers...)
	sort.Strings(couriers)

	dimensions := "none"
	if request.Dimensions != nil {
		dimensions = fmt.Sprintf(
			"%dx%dx%d",
			request.Dimensions.LengthCM,
			request.Dimensions.WidthCM,
			request.Dimensions.HeightCM,
		)
	}
	return fmt.Sprintf(
		"%s:%s:%s:%s:%s:%s:%d:%s:%s:%d:%t",
		request.TenantID,
		request.ProviderCredentialID,
		request.Origin,
		request.Destination,
		request.Granularity,
		request.PriceFilter,
		request.ActualWeightGrams,
		strings.Join(couriers, ":"),
		dimensions,
		request.ItemValue,
		request.IncludeUnverified,
	)
}
