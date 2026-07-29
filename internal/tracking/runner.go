package tracking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"
)

type Runner struct {
	repository   Repository
	cipher       *Cipher
	adapters     map[string]Adapter
	courierCodes []string
	workerID     string
	concurrency  int
	pollInterval time.Duration
	logger       *slog.Logger
}

func NewRunner(
	repository Repository,
	cipher *Cipher,
	adapters []Adapter,
	workerID string,
	concurrency int,
	pollInterval time.Duration,
	logger *slog.Logger,
) *Runner {
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	adapterByCourier := make(map[string]Adapter)
	for _, adapter := range adapters {
		for _, courierCode := range adapter.CourierCodes() {
			adapterByCourier[courierCode] = adapter
		}
	}
	courierCodes := make([]string, 0, len(adapterByCourier))
	for courierCode := range adapterByCourier {
		courierCodes = append(courierCodes, courierCode)
	}
	sort.Strings(courierCodes)
	return &Runner{
		repository: repository, cipher: cipher, adapters: adapterByCourier,
		courierCodes: courierCodes, workerID: workerID,
		concurrency: concurrency, pollInterval: pollInterval, logger: logger,
	}
}

func (r *Runner) Run(ctx context.Context) error {
	if len(r.adapters) == 0 {
		r.logger.Warn("tracking worker has no enabled provider adapter; jobs remain pending")
		<-ctx.Done()
		return nil
	}
	var workers sync.WaitGroup
	workers.Add(r.concurrency)
	for index := 0; index < r.concurrency; index++ {
		workerID := fmt.Sprintf("%s-%02d", r.workerID, index+1)
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
		if err := r.processOneAs(ctx, workerID); err != nil &&
			!errors.Is(err, ErrNoJobAvailable) &&
			!errors.Is(err, context.Canceled) {
			r.logger.Error("tracking job failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Runner) processOne(ctx context.Context) error {
	return r.processOneAs(ctx, r.workerID)
}

func (r *Runner) processOneAs(ctx context.Context, workerID string) error {
	job, err := r.repository.Claim(ctx, workerID, r.courierCodes)
	if err != nil {
		return err
	}
	adapter := r.adapters[job.CourierCode]
	if adapter == nil {
		return r.failJob(ctx, job, "ADAPTER_UNAVAILABLE", ErrAdapterUnavailable)
	}
	waybill, err := r.cipher.Decrypt(job.WaybillCiphertext, []byte(job.CourierCode))
	if err != nil {
		return r.failJob(ctx, job, "DECRYPTION_FAILED", err)
	}
	defer zeroBytes(waybill)

	request := Request{
		CourierCode: job.CourierCode,
		Waybill:     string(waybill),
	}
	if len(job.ProviderContextCiphertext) > 0 {
		providerContext, decryptErr := r.cipher.Decrypt(
			job.ProviderContextCiphertext,
			[]byte(job.CourierCode+":provider-context"),
		)
		if decryptErr != nil {
			return r.failJob(ctx, job, "DECRYPTION_FAILED", decryptErr)
		}
		defer zeroBytes(providerContext)
		var values struct {
			LastPhoneNumber string `json:"last_phone_number"`
		}
		if unmarshalErr := json.Unmarshal(providerContext, &values); unmarshalErr != nil {
			return r.failJob(ctx, job, "DECRYPTION_FAILED", unmarshalErr)
		}
		request.LastPhoneDigits = values.LastPhoneNumber
	}

	result, err := adapter.Track(ctx, request)
	if err != nil {
		return r.failJob(ctx, job, trackingFailureCode(err), err)
	}
	if result.FetchedAt.IsZero() {
		result.FetchedAt = time.Now().UTC()
	}
	if err := r.repository.Complete(ctx, job, result); err != nil {
		return fmt.Errorf("complete tracking job: %w", err)
	}
	return nil
}

func (r *Runner) failJob(
	ctx context.Context,
	job Job,
	code string,
	_ error,
) error {
	delay := retryDelay(code, job.AttemptCount, time.Now())
	message := "tracking refresh failed"
	switch code {
	case "ADAPTER_UNAVAILABLE":
		message = "tracking adapter is not configured"
	case "DECRYPTION_FAILED":
		message = "tracking payload could not be decrypted"
	case "PROVIDER_ERROR":
		message = "tracking provider request failed"
	case "PROVIDER_UNAUTHORIZED":
		message = "tracking provider credential was rejected"
	case "PROVIDER_RATE_LIMITED":
		message = "tracking provider temporarily rate limited the request"
	case "PROVIDER_QUOTA_EXHAUSTED":
		message = "tracking provider quota is exhausted"
	case "WAYBILL_NOT_FOUND":
		message = "tracking waybill is not available yet"
	case "PHONE_VALIDATION_REQUIRED":
		message = "tracking provider requires recipient phone validation"
	}
	if err := r.repository.Fail(
		ctx,
		job,
		code,
		message,
		time.Now().UTC().Add(delay),
	); err != nil {
		return fmt.Errorf("record tracking failure: %w", err)
	}
	return fmt.Errorf("tracking job failed: %s", code)
}

func trackingFailureCode(err error) string {
	switch {
	case errors.Is(err, ErrProviderUnauthorized):
		return "PROVIDER_UNAUTHORIZED"
	case errors.Is(err, ErrProviderRateLimited):
		return "PROVIDER_RATE_LIMITED"
	case errors.Is(err, ErrProviderQuota):
		return "PROVIDER_QUOTA_EXHAUSTED"
	case errors.Is(err, ErrWaybillNotFound):
		return "WAYBILL_NOT_FOUND"
	case errors.Is(err, ErrPhoneSuffixRequired):
		return "PHONE_VALIDATION_REQUIRED"
	default:
		return "PROVIDER_ERROR"
	}
}

func retryDelay(code string, attempt int, now time.Time) time.Duration {
	switch code {
	case "PROVIDER_RATE_LIMITED":
		exponent := math.Min(float64(attempt), 4)
		delay := time.Duration(math.Pow(2, exponent)) * 30 * time.Second
		if delay > 5*time.Minute {
			return 5 * time.Minute
		}
		return delay
	case "PROVIDER_QUOTA_EXHAUSTED":
		jakarta := time.FixedZone("Asia/Jakarta", 7*60*60)
		localNow := now.In(jakarta)
		nextReset := time.Date(
			localNow.Year(),
			localNow.Month(),
			localNow.Day()+1,
			0,
			5,
			0,
			0,
			jakarta,
		)
		return nextReset.Sub(localNow)
	case "PROVIDER_UNAUTHORIZED":
		return 6 * time.Hour
	case "PHONE_VALIDATION_REQUIRED":
		return 24 * time.Hour
	case "WAYBILL_NOT_FOUND":
		exponent := math.Min(float64(attempt), 5)
		return time.Duration(math.Pow(2, exponent)) * 15 * time.Minute
	default:
		exponent := math.Min(float64(attempt), 6)
		return time.Duration(math.Pow(2, exponent)) * time.Minute
	}
}

func zeroBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
