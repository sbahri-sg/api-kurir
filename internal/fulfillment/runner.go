package fulfillment

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/emisell/api-kurir/internal/tenancy"
)

type Runner struct {
	repository   LifecycleRepository
	service      *Service
	tracking     TrackingRegistrar
	workerID     string
	concurrency  int
	pollInterval time.Duration
	logger       *slog.Logger
	now          func() time.Time
}

func NewRunner(
	repository LifecycleRepository,
	service *Service,
	trackingRegistrar TrackingRegistrar,
	workerID string,
	concurrency int,
	pollInterval time.Duration,
	logger *slog.Logger,
) *Runner {
	if concurrency <= 0 {
		concurrency = 1
	}
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	return &Runner{
		repository: repository, service: service, tracking: trackingRegistrar,
		workerID: workerID, concurrency: concurrency,
		pollInterval: pollInterval, logger: logger, now: time.Now,
	}
}

func (r *Runner) Run(ctx context.Context) error {
	var workers sync.WaitGroup
	workers.Add(r.concurrency)
	for index := 0; index < r.concurrency; index++ {
		workerID := fmt.Sprintf("%s-fulfillment-%02d", r.workerID, index+1)
		go func() {
			defer workers.Done()
			r.runLoop(ctx, workerID)
		}()
	}
	workers.Wait()
	return nil
}

func (r *Runner) runLoop(ctx context.Context, workerID string) {
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()
	for {
		if err := r.processOne(ctx, workerID); err != nil &&
			!errors.Is(err, ErrNoLifecycleJob) &&
			!errors.Is(err, context.Canceled) {
			r.logger.Error("fulfillment lifecycle job failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Runner) processOne(ctx context.Context, workerID string) error {
	job, err := r.repository.ClaimLifecycleJob(ctx, workerID)
	if err != nil {
		return err
	}
	jobCtx := tenancy.WithIdentity(ctx, tenancy.Identity{TenantID: job.TenantID})
	shipment, err := r.repository.GetLifecycleShipment(jobCtx, job.TenantID, job.ShipmentID)
	if err != nil {
		return r.fail(jobCtx, job, "SHIPMENT_NOT_FOUND", err)
	}
	switch job.JobType {
	case "register_tracking":
		if shipment.AWB == "" {
			return r.fail(jobCtx, job, "AWB_NOT_READY", errors.New("shipment AWB is empty"))
		}
		trackingShipmentID, err := r.tracking.RegisterFulfillmentTracking(
			jobCtx,
			TrackingRegistration{
				TenantID: job.TenantID, OrderReference: shipment.MerchantReference,
				FulfillmentReference: shipment.ID, CourierCode: shipment.CourierCode,
				Waybill: shipment.AWB,
			},
		)
		if err != nil {
			return r.fail(jobCtx, job, "TRACKING_REGISTRATION_FAILED", err)
		}
		return r.repository.CompleteTrackingRegistration(jobCtx, job, trackingShipmentID)
	case "reconcile":
		adapter, credential, err := r.service.adapterCredential(
			jobCtx, shipment.ProviderCode, "shipments:read",
		)
		if err != nil {
			return r.fail(jobCtx, job, lifecycleFailureCode(err), err)
		}
		detailAdapter, ok := adapter.(DetailAdapter)
		if !ok {
			return r.fail(jobCtx, job, "DETAIL_UNSUPPORTED", ErrProviderUnsupported)
		}
		result, err := detailAdapter.Detail(jobCtx, credential, shipment)
		if err != nil {
			return r.fail(jobCtx, job, lifecycleFailureCode(err), err)
		}
		return r.repository.CompleteReconciliation(
			jobCtx, job, result, nextReconciliationAt(r.now().UTC(), result),
		)
	default:
		return r.fail(jobCtx, job, "JOB_TYPE_UNSUPPORTED", ErrInvalidRequest)
	}
}

func (r *Runner) fail(
	ctx context.Context,
	job LifecycleJob,
	code string,
	err error,
) error {
	retryAt := r.now().UTC().Add(lifecycleRetryDelay(code, job.AttemptCount))
	if recordErr := r.repository.FailLifecycleJob(
		ctx, job, code, err.Error(), retryAt,
	); recordErr != nil {
		return recordErr
	}
	return fmt.Errorf("%s: %w", code, err)
}

func nextReconciliationAt(now time.Time, result ProviderDetailResult) *time.Time {
	if result.AWB != "" || result.Status == StatusDelivered || result.Status == StatusCancelled {
		return nil
	}
	delay := 6 * time.Hour
	if result.Status == StatusPickupRequested || result.Status == StatusPickedUp {
		delay = time.Hour
	}
	next := now.Add(delay)
	return &next
}

func lifecycleRetryDelay(code string, attempt int) time.Duration {
	switch code {
	case "PROVIDER_UNAUTHORIZED", "DETAIL_UNSUPPORTED":
		return 24 * time.Hour
	case "AWB_NOT_READY":
		return time.Hour
	case "TRACKING_REGISTRATION_FAILED":
		if attempt == 0 {
			return 5 * time.Minute
		}
		return time.Hour
	default:
		if attempt == 0 {
			return time.Hour
		}
		if attempt == 1 {
			return 6 * time.Hour
		}
		return 24 * time.Hour
	}
}

func lifecycleFailureCode(err error) string {
	switch {
	case errors.Is(err, ErrProviderUnauthorized):
		return "PROVIDER_UNAUTHORIZED"
	case errors.Is(err, ErrProviderTimeout):
		return "PROVIDER_TIMEOUT"
	case errors.Is(err, ErrProviderRejected):
		return "PROVIDER_REJECTED"
	case errors.Is(err, ErrCredentialUnavailable):
		return "DELIVERY_CREDENTIAL_REQUIRED"
	default:
		return "PROVIDER_UNAVAILABLE"
	}
}
