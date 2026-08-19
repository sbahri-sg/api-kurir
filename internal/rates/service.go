package rates

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/emisell/api-kurir/internal/platform/cache"
	"golang.org/x/sync/singleflight"
)

var ErrRateNotAvailable = errors.New("rate not available")

type Service struct {
	repository   Repository
	queryTimeout time.Duration
	group        singleflight.Group
	provider     QuoteProvider
	snapshots    SnapshotRepository
	locker       cache.Locker
	lockTTL      time.Duration
	resultPolicy ResultPolicy
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
		service.snapshots = snapshots
		service.locker = locker
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

func NewService(repository Repository, queryTimeout time.Duration, options ...Option) *Service {
	if queryTimeout <= 0 {
		queryTimeout = 5 * time.Second
	}
	service := &Service{
		repository:   repository,
		queryTimeout: queryTimeout,
		lockTTL:      10 * time.Second,
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) Calculate(ctx context.Context, request Request) ([]Result, error) {
	key := requestKey(request)
	resultCh := s.group.DoChan(key, func() (any, error) {
		operationCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.queryTimeout)
		defer cancel()
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

func (s *Service) calculate(ctx context.Context, request Request, key string) ([]Result, error) {
	cards, err := s.repository.FindActiveRateCards(ctx, request)
	if err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(cards))
	coveredCouriers := make(map[string]struct{}, len(cards))
	for _, card := range cards {
		result, err := Calculate(request, card)
		if errors.Is(err, ErrWeightExceeded) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("calculate %s %s: %w", card.CourierCode, card.ServiceCode, err)
		}
		coveredCouriers[card.CourierCode] = struct{}{}
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
			results = append(results, providerResults...)
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

func (s *Service) calculateProviderFallback(
	ctx context.Context,
	request Request,
	key string,
) ([]Result, error) {
	if s.provider == nil || s.snapshots == nil {
		return nil, ErrRateNotAvailable
	}

	quotes, err := s.snapshots.FindFreshProviderQuotes(ctx, request, s.provider.Code())
	if err != nil {
		return nil, err
	}
	if len(quotes) > 0 {
		return providerQuoteResults(request, quotes), nil
	}

	fetch := func(lockCtx context.Context) error {
		fresh, err := s.snapshots.FindFreshProviderQuotes(lockCtx, request, s.provider.Code())
		if err != nil {
			return err
		}
		if len(fresh) > 0 {
			quotes = fresh
			return nil
		}
		fresh, err = s.provider.Quote(lockCtx, request)
		if err != nil {
			return err
		}
		if len(fresh) == 0 {
			return ErrRateNotAvailable
		}
		if err := s.snapshots.SaveProviderQuotes(lockCtx, request, fresh); err != nil {
			return err
		}
		quotes = fresh
		return nil
	}

	if s.locker == nil {
		err = fetch(ctx)
	} else {
		err = s.locker.WithLock(ctx, "provider-rate:"+key, s.lockTTL, fetch)
	}
	if errors.Is(err, cache.ErrLockNotAcquired) {
		quotes, err = s.waitForProviderSnapshot(ctx, request)
	}
	if err != nil {
		return nil, err
	}
	return providerQuoteResults(request, quotes), nil
}

func (s *Service) waitForProviderSnapshot(
	ctx context.Context,
	request Request,
) ([]ProviderQuote, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w: waiting for provider refresh", ErrProviderUnavailable)
		case <-ticker.C:
			quotes, err := s.snapshots.FindFreshProviderQuotes(ctx, request, s.provider.Code())
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
		request.IntegrationID,
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
