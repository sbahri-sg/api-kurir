package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/emisell/api-kurir/internal/config"
	"github.com/emisell/api-kurir/internal/database"
	"github.com/emisell/api-kurir/internal/locations"
	"github.com/emisell/api-kurir/internal/providers/rajaongkir"
	"github.com/emisell/api-kurir/internal/rates"
)

const providerCode = "rajaongkir"

type commandOptions struct {
	mode            string
	search          string
	limit           int
	offset          int
	maxHits         int
	concurrency     int
	requestInterval time.Duration
	refresh         bool
	continuous      bool
}

type hitBudget struct {
	mu   sync.Mutex
	max  int
	used int
}

func (b *hitBudget) take() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.used >= b.max {
		return false
	}
	b.used++
	return true
}

func (b *hitBudget) usage() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used
}

type syncResult struct {
	children  []locations.ImportedProviderLocation
	exhausted bool
	err       error
}

type hierarchyFetcher func(
	context.Context,
	string,
) ([]rajaongkir.HierarchyLocation, error)

type hierarchyBuilder func(
	locations.ImportedProviderLocation,
	rajaongkir.HierarchyLocation,
) locations.ImportedProviderLocation

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("location sync failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	options, err := parseOptions()
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.RajaOngkir.APIKey == "" {
		return errors.New("RAJAONGKIR_API_KEY is required")
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	pool, err := database.Open(
		ctx,
		cfg.DatabaseURL,
		0,
		int32(options.concurrency+4),
	)
	if err != nil {
		return err
	}
	defer pool.Close()

	providerHTTPClient := rajaongkir.NewPacedHTTPClient(
		cfg.RajaOngkir.Timeout,
		options.requestInterval,
	)
	client, err := rajaongkir.NewClient(
		cfg.RajaOngkir.BaseURL,
		cfg.RajaOngkir.APIKey,
		providerHTTPClient,
	)
	if err != nil {
		return err
	}
	locationRepository := locations.NewPostgresRepository(pool)
	rateRepository := rates.NewPostgresRepository(pool)

	if options.mode == "search" {
		return runSearch(
			ctx,
			logger,
			cfg,
			options,
			client,
			locationRepository,
			rateRepository,
		)
	}
	if options.continuous {
		return runContinuousFullSync(
			ctx,
			logger,
			cfg,
			options,
			client,
			locationRepository,
			rateRepository,
		)
	}
	return runFullSync(
		ctx,
		logger,
		cfg,
		options,
		client,
		locationRepository,
		rateRepository,
	)
}

func parseOptions() (commandOptions, error) {
	mode := flag.String("mode", "search", "sync mode: search or full")
	search := flag.String(
		"search",
		"",
		"city, district, subdistrict, or postal code",
	)
	limit := flag.Int("limit", 100, "maximum search results, 1-1000")
	offset := flag.Int("offset", 0, "search result offset")
	maxHits := flag.Int(
		"max-hits",
		500,
		"maximum RajaOngkir calls for one full-sync run",
	)
	concurrency := flag.Int(
		"concurrency",
		4,
		"parallel provider calls for full sync, 1-16",
	)
	requestInterval := flag.Duration(
		"request-interval",
		250*time.Millisecond,
		"minimum interval between RajaOngkir requests",
	)
	refresh := flag.Bool(
		"refresh",
		false,
		"ignore completed checkpoints and fetch provider data again",
	)
	continuous := flag.Bool(
		"continuous",
		false,
		"wait for quota reset and resume full sync automatically",
	)
	flag.Parse()

	options := commandOptions{
		mode:            strings.ToLower(strings.TrimSpace(*mode)),
		search:          strings.TrimSpace(*search),
		limit:           *limit,
		offset:          *offset,
		maxHits:         *maxHits,
		concurrency:     *concurrency,
		requestInterval: *requestInterval,
		refresh:         *refresh,
		continuous:      *continuous,
	}
	if options.mode != "search" && options.mode != "full" {
		return commandOptions{}, errors.New("-mode must be search or full")
	}
	if options.mode == "search" && len(options.search) < 2 {
		return commandOptions{}, errors.New(
			"-search with at least 2 characters is required in search mode",
		)
	}
	if options.continuous && options.mode != "full" {
		return commandOptions{}, errors.New(
			"-continuous can only be used with -mode full",
		)
	}
	if options.limit < 1 || options.limit > 1000 || options.offset < 0 {
		return commandOptions{}, errors.New("invalid search pagination")
	}
	if options.maxHits < 1 {
		return commandOptions{}, errors.New("-max-hits must be positive")
	}
	if options.concurrency < 1 || options.concurrency > 16 {
		return commandOptions{}, errors.New("-concurrency must be between 1 and 16")
	}
	if options.requestInterval < 10*time.Millisecond {
		return commandOptions{}, errors.New(
			"-request-interval must be at least 10ms",
		)
	}
	return options, nil
}

func runContinuousFullSync(
	ctx context.Context,
	logger *slog.Logger,
	cfg config.Config,
	options commandOptions,
	client *rajaongkir.Client,
	locationRepository *locations.PostgresRepository,
	rateRepository *rates.PostgresRepository,
) error {
	for {
		err := runFullSync(
			ctx,
			logger,
			cfg,
			options,
			client,
			locationRepository,
			rateRepository,
		)
		if err == nil {
			if err := waitForNextRun(
				ctx,
				logger,
				24*time.Hour,
				"checkpoint verification",
			); err != nil {
				return err
			}
			continue
		}
		if errors.Is(err, rates.ErrProviderRateLimited) {
			if err := waitForNextRun(
				ctx,
				logger,
				time.Minute,
				"RajaOngkir rate-limit cooldown",
			); err != nil {
				return err
			}
			continue
		}
		if !errors.Is(err, rates.ErrProviderQuotaExhausted) {
			return err
		}
		delay := durationUntilJakartaMidnight(time.Now()) + 30*time.Second
		if err := waitForNextRun(
			ctx,
			logger,
			delay,
			"RajaOngkir quota reset",
		); err != nil {
			return err
		}
	}
}

func waitForNextRun(
	ctx context.Context,
	logger *slog.Logger,
	delay time.Duration,
	reason string,
) error {
	if delay < time.Second {
		delay = time.Second
	}
	logger.Info(
		"location sync waiting",
		"reason",
		reason,
		"resume_at",
		time.Now().Add(delay).Format(time.RFC3339),
	)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func durationUntilJakartaMidnight(now time.Time) time.Duration {
	jakarta := time.FixedZone("Asia/Jakarta", 7*60*60)
	localNow := now.In(jakarta)
	nextMidnight := time.Date(
		localNow.Year(),
		localNow.Month(),
		localNow.Day()+1,
		0,
		0,
		0,
		0,
		jakarta,
	)
	return nextMidnight.Sub(localNow)
}

func runSearch(
	ctx context.Context,
	logger *slog.Logger,
	cfg config.Config,
	options commandOptions,
	client *rajaongkir.Client,
	locationRepository *locations.PostgresRepository,
	rateRepository *rates.PostgresRepository,
) error {
	if err := rateRepository.ConsumeProviderHit(
		ctx,
		providerCode,
		cfg.RajaOngkir.CredentialAlias,
		cfg.RajaOngkir.DailyLimit,
	); err != nil {
		return err
	}

	startedAt := time.Now()
	found, err := client.SearchDestinations(
		ctx,
		options.search,
		options.limit,
		options.offset,
	)
	recordProviderCall(
		ctx,
		rateRepository,
		cfg,
		"destination/domestic-destination",
		hashStrings(options.search, strconv.Itoa(options.offset)),
		startedAt,
		err,
	)
	if err != nil {
		return err
	}

	retrievedAt := time.Now()
	items := make([]locations.ImportedProviderLocation, 0, len(found))
	for _, destination := range found {
		items = append(items, locations.ImportedProviderLocation{
			ProviderLocationID: destination.ID,
			ProviderLabel:      destination.Label,
			Province:           destination.ProvinceName,
			City:               destination.CityName,
			District:           destination.DistrictName,
			Subdistrict:        destination.SubdistrictName,
			PostalCode:         destination.ZipCode,
			SourceEndpoint:     "destination/domestic-destination",
			RetrievedAt:        retrievedAt,
		})
	}
	count, err := locationRepository.UpsertProviderLocations(
		ctx,
		providerCode,
		items,
	)
	if err != nil {
		return err
	}
	logger.Info(
		"location search sync completed",
		"search",
		options.search,
		"imported",
		count,
	)
	return nil
}

func runFullSync(
	ctx context.Context,
	logger *slog.Logger,
	cfg config.Config,
	options commandOptions,
	client *rajaongkir.Client,
	locationRepository *locations.PostgresRepository,
	rateRepository *rates.PostgresRepository,
) error {
	budget := &hitBudget{max: options.maxHits}
	root := []locations.ImportedProviderLocation{{}}

	provinces, exhausted, err := syncStage(
		ctx,
		options.concurrency,
		root,
		"province",
		"destination/province",
		cfg,
		options.refresh,
		budget,
		locationRepository,
		rateRepository,
		func(callCtx context.Context, _ string) ([]rajaongkir.HierarchyLocation, error) {
			return client.ListProvinces(callCtx)
		},
		func(
			_ locations.ImportedProviderLocation,
			item rajaongkir.HierarchyLocation,
		) locations.ImportedProviderLocation {
			return locations.ImportedProviderLocation{
				ProviderLocationID: item.ID,
				ProviderLabel:      item.Name,
				Province:           item.Name,
				PostalCode:         item.ZipCode,
			}
		},
	)
	if err != nil {
		return err
	}
	if exhausted {
		return logBudgetStop(logger, budget, "province")
	}

	cities, exhausted, err := syncStage(
		ctx,
		options.concurrency,
		provinces,
		"city",
		"destination/city",
		cfg,
		options.refresh,
		budget,
		locationRepository,
		rateRepository,
		client.ListCities,
		func(
			parent locations.ImportedProviderLocation,
			item rajaongkir.HierarchyLocation,
		) locations.ImportedProviderLocation {
			return locations.ImportedProviderLocation{
				ProviderLocationID:       item.ID,
				ProviderLabel:            item.Name,
				Province:                 parent.Province,
				City:                     item.Name,
				PostalCode:               item.ZipCode,
				ParentProviderLocationID: parent.ProviderLocationID,
			}
		},
	)
	if err != nil {
		return err
	}
	if exhausted {
		return logBudgetStop(logger, budget, "city")
	}

	districts, exhausted, err := syncStage(
		ctx,
		options.concurrency,
		cities,
		"district",
		"destination/district",
		cfg,
		options.refresh,
		budget,
		locationRepository,
		rateRepository,
		client.ListDistricts,
		func(
			parent locations.ImportedProviderLocation,
			item rajaongkir.HierarchyLocation,
		) locations.ImportedProviderLocation {
			return locations.ImportedProviderLocation{
				ProviderLocationID:       item.ID,
				ProviderLabel:            item.Name,
				Province:                 parent.Province,
				City:                     parent.City,
				District:                 item.Name,
				PostalCode:               item.ZipCode,
				ParentProviderLocationID: parent.ProviderLocationID,
			}
		},
	)
	if err != nil {
		return err
	}
	if exhausted {
		return logBudgetStop(logger, budget, "district")
	}

	subdistricts, exhausted, err := syncStage(
		ctx,
		options.concurrency,
		districts,
		"subdistrict",
		"destination/sub-district",
		cfg,
		options.refresh,
		budget,
		locationRepository,
		rateRepository,
		client.ListSubdistricts,
		func(
			parent locations.ImportedProviderLocation,
			item rajaongkir.HierarchyLocation,
		) locations.ImportedProviderLocation {
			return locations.ImportedProviderLocation{
				ProviderLocationID:       item.ID,
				ProviderLabel:            item.Name,
				Province:                 parent.Province,
				City:                     parent.City,
				District:                 parent.District,
				Subdistrict:              item.Name,
				PostalCode:               item.ZipCode,
				ParentProviderLocationID: parent.ProviderLocationID,
			}
		},
	)
	if err != nil {
		return err
	}
	if exhausted {
		return logBudgetStop(logger, budget, "subdistrict")
	}

	logger.Info(
		"full location sync completed",
		"provider_hits",
		budget.usage(),
		"provinces",
		len(provinces),
		"cities",
		len(cities),
		"districts",
		len(districts),
		"subdistricts",
		len(subdistricts),
	)
	return nil
}

func syncStage(
	ctx context.Context,
	concurrency int,
	parents []locations.ImportedProviderLocation,
	endpointLevel string,
	endpointBase string,
	cfg config.Config,
	refresh bool,
	budget *hitBudget,
	locationRepository *locations.PostgresRepository,
	rateRepository *rates.PostgresRepository,
	fetch hierarchyFetcher,
	build hierarchyBuilder,
) ([]locations.ImportedProviderLocation, bool, error) {
	stageCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan locations.ImportedProviderLocation)
	results := make(chan syncResult, len(parents))
	var workers sync.WaitGroup

	worker := func() {
		defer workers.Done()
		for {
			var parent locations.ImportedProviderLocation
			var open bool
			select {
			case <-stageCtx.Done():
				return
			case parent, open = <-jobs:
				if !open {
					return
				}
			}
			children, exhausted, err := syncParent(
				ctx,
				parent,
				endpointLevel,
				endpointBase,
				cfg,
				refresh,
				budget,
				locationRepository,
				rateRepository,
				fetch,
				build,
			)
			results <- syncResult{
				children:  children,
				exhausted: exhausted,
				err:       err,
			}
			if err != nil || exhausted {
				cancel()
				return
			}
		}
	}

	workerCount := concurrency
	if len(parents) < workerCount {
		workerCount = len(parents)
	}
	workers.Add(workerCount)
	for range workerCount {
		go worker()
	}
	go func() {
		defer close(jobs)
		for _, parent := range parents {
			select {
			case jobs <- parent:
			case <-stageCtx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()

	children := make([]locations.ImportedProviderLocation, 0)
	exhausted := false
	var firstErr error
	for result := range results {
		children = append(children, result.children...)
		exhausted = exhausted || result.exhausted
		if result.err != nil {
			switch {
			case firstErr == nil:
				firstErr = result.err
			case errors.Is(firstErr, context.Canceled) &&
				!errors.Is(result.err, context.Canceled):
				firstErr = result.err
			}
		}
	}
	if firstErr != nil {
		return nil, exhausted, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, exhausted, err
	}
	return children, exhausted, nil
}

func syncParent(
	ctx context.Context,
	parent locations.ImportedProviderLocation,
	endpointLevel string,
	endpointBase string,
	cfg config.Config,
	refresh bool,
	budget *hitBudget,
	locationRepository *locations.PostgresRepository,
	rateRepository *rates.PostgresRepository,
	fetch hierarchyFetcher,
	build hierarchyBuilder,
) ([]locations.ImportedProviderLocation, bool, error) {
	parentID := parent.ProviderLocationID
	if !refresh {
		expectedCount, completed, err := locationRepository.SyncCheckpointItemCount(
			ctx,
			providerCode,
			cfg.RajaOngkir.CredentialAlias,
			endpointLevel,
			parentID,
		)
		if err != nil {
			return nil, false, err
		}
		if completed {
			children, err := locationRepository.ListProviderChildren(
				ctx,
				providerCode,
				parentID,
				endpointLevel,
			)
			if err != nil {
				return nil, false, err
			}
			if len(children) == expectedCount {
				return children, false, nil
			}
		}
	}

	if !budget.take() {
		return nil, true, nil
	}
	if err := rateRepository.ConsumeProviderHit(
		ctx,
		providerCode,
		cfg.RajaOngkir.CredentialAlias,
		cfg.RajaOngkir.DailyLimit,
	); err != nil {
		return nil, false, err
	}

	endpoint := endpointBase
	if parentID != "" {
		endpoint += "/" + parentID
	}
	startedAt := time.Now()
	found, err := fetch(ctx, parentID)
	recordProviderCall(
		ctx,
		rateRepository,
		cfg,
		endpoint,
		hashStrings(endpointLevel, parentID),
		startedAt,
		err,
	)
	if err != nil {
		if errors.Is(err, rates.ErrProviderQuotaExhausted) {
			_ = rateRepository.MarkProviderQuotaExhausted(
				context.WithoutCancel(ctx),
				providerCode,
				cfg.RajaOngkir.CredentialAlias,
				cfg.RajaOngkir.DailyLimit,
			)
		}
		return nil, false, err
	}

	retrievedAt := time.Now()
	children := make([]locations.ImportedProviderLocation, 0, len(found))
	for _, item := range found {
		child := build(parent, item)
		child.SourceEndpoint = endpoint
		child.RetrievedAt = retrievedAt
		children = append(children, child)
	}
	if _, err := locationRepository.UpsertProviderLocations(
		ctx,
		providerCode,
		children,
	); err != nil {
		return nil, false, err
	}
	activeProviderLocationIDs := make([]string, 0, len(children))
	for _, child := range children {
		activeProviderLocationIDs = append(
			activeProviderLocationIDs,
			child.ProviderLocationID,
		)
	}
	if err := locationRepository.DeactivateMissingProviderChildren(
		ctx,
		providerCode,
		parentID,
		endpointLevel,
		activeProviderLocationIDs,
	); err != nil {
		return nil, false, err
	}

	responseHash := hashHierarchy(found)
	if err := locationRepository.MarkSyncCheckpointCompleted(
		ctx,
		providerCode,
		cfg.RajaOngkir.CredentialAlias,
		endpointLevel,
		parentID,
		len(children),
		responseHash,
	); err != nil {
		return nil, false, err
	}
	return children, false, nil
}

func recordProviderCall(
	ctx context.Context,
	repository *rates.PostgresRepository,
	cfg config.Config,
	endpoint string,
	fingerprint string,
	startedAt time.Time,
	callErr error,
) {
	status := 200
	outcome := "success"
	errorCode := ""
	if callErr != nil {
		status = 0
		outcome = "provider_error"
		errorCode = "PROVIDER_ERROR"
		switch {
		case errors.Is(callErr, rates.ErrProviderRateLimited):
			status = 429
			errorCode = "PROVIDER_RATE_LIMITED"
		case errors.Is(callErr, rates.ErrProviderQuotaExhausted):
			status = 429
			errorCode = "PROVIDER_QUOTA_EXHAUSTED"
		case errors.Is(callErr, rates.ErrProviderUnauthorized):
			status = 401
			errorCode = "PROVIDER_UNAUTHORIZED"
		case errors.Is(callErr, context.DeadlineExceeded),
			errors.Is(callErr, context.Canceled):
			outcome = "timeout"
			errorCode = "PROVIDER_TIMEOUT"
		}
	}
	_ = repository.RecordProviderAPICall(
		context.WithoutCancel(ctx),
		providerCode,
		cfg.RajaOngkir.CredentialAlias,
		endpoint,
		fingerprint,
		status,
		outcome,
		time.Since(startedAt),
		1,
		errorCode,
	)
}

func hashHierarchy(items []rajaongkir.HierarchyLocation) string {
	values := make([]string, 0, len(items)*3)
	for _, item := range items {
		values = append(values, item.ID, item.Name, item.ZipCode)
	}
	return hashStrings(values...)
}

func hashStrings(values ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(values, "\x1f")))
	return hex.EncodeToString(hash[:])
}

func logBudgetStop(
	logger *slog.Logger,
	budget *hitBudget,
	level string,
) error {
	logger.Info(
		"location sync paused at configured hit budget; run the command again to resume",
		"provider_hits",
		budget.usage(),
		"max_hits",
		budget.max,
		"level",
		level,
	)
	return nil
}
