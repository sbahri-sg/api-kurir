package fulfillment

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/emisell/api-kurir/internal/tracking"
)

type WebhookDispatcher struct {
	repository   WebhookRepository
	destination  tracking.WebhookDestinationResolver
	workerID     string
	concurrency  int
	pollInterval time.Duration
	client       *http.Client
	logger       *slog.Logger
	now          func() time.Time
}

func NewWebhookDispatcher(
	repository WebhookRepository,
	destination tracking.WebhookDestinationResolver,
	workerID string,
	concurrency int,
	pollInterval, timeout time.Duration,
	logger *slog.Logger,
) *WebhookDispatcher {
	if concurrency <= 0 {
		concurrency = 1
	}
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &WebhookDispatcher{
		repository: repository, destination: destination, workerID: workerID,
		concurrency: concurrency, pollInterval: pollInterval,
		client: &http.Client{Timeout: timeout}, logger: logger, now: time.Now,
	}
}

func (d *WebhookDispatcher) Run(ctx context.Context) error {
	var workers sync.WaitGroup
	workers.Add(d.concurrency)
	for index := 0; index < d.concurrency; index++ {
		workerID := fmt.Sprintf("%s-fulfillment-webhook-%02d", d.workerID, index+1)
		go func() {
			defer workers.Done()
			d.runLoop(ctx, workerID)
		}()
	}
	workers.Wait()
	return nil
}

func (d *WebhookDispatcher) runLoop(ctx context.Context, workerID string) {
	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()
	for {
		if err := d.processOne(ctx, workerID); err != nil &&
			!errors.Is(err, ErrNoWebhookJob) &&
			!errors.Is(err, tracking.ErrWebhookDisabled) &&
			!errors.Is(err, context.Canceled) {
			d.logger.Error("fulfillment webhook delivery failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (d *WebhookDispatcher) processOne(ctx context.Context, workerID string) error {
	endpoint, secret, enabled, err := d.destination.ResolveWebhookDestination(ctx)
	if err != nil {
		return err
	}
	if !enabled || endpoint == "" || len(secret) == 0 {
		return tracking.ErrWebhookDisabled
	}
	defer clearSecret(secret)
	job, err := d.repository.ClaimFulfillmentWebhook(ctx, workerID)
	if err != nil {
		return err
	}
	envelope := map[string]any{
		"id": job.ID, "type": job.EventType, "api_version": "2026-08-26",
		"occurred_at": job.CreatedAt.UTC().Format(time.RFC3339Nano), "data": job.Data,
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return d.fail(ctx, job, 0, "encode webhook payload", false)
	}
	timestamp := strconv.FormatInt(d.now().UTC().Unix(), 10)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return d.fail(ctx, job, 0, "create webhook request", false)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Emisell-Event-ID", job.ID)
	request.Header.Set("X-Emisell-Event-Type", job.EventType)
	request.Header.Set("X-Emisell-Webhook-Timestamp", timestamp)
	request.Header.Set("X-Emisell-Webhook-Signature", fulfillmentWebhookSignature(secret, timestamp, body))
	response, err := d.client.Do(request)
	if err != nil {
		return d.fail(ctx, job, 0, "webhook network error", true)
	}
	defer response.Body.Close()
	_, _ = io.CopyN(io.Discard, response.Body, 4096)
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return d.repository.CompleteFulfillmentWebhook(ctx, job.ID, response.StatusCode)
	}
	retry := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
	return d.fail(ctx, job, response.StatusCode, "webhook receiver rejected event", retry)
}

func (d *WebhookDispatcher) fail(
	ctx context.Context,
	job WebhookJob,
	httpStatus int,
	message string,
	retry bool,
) error {
	retryAt := d.now().UTC().Add(fulfillmentWebhookRetryDelay(job.AttemptCount))
	if err := d.repository.FailFulfillmentWebhook(
		ctx, job, httpStatus, message, retryAt, retry,
	); err != nil {
		return err
	}
	return errors.New(message)
}

func fulfillmentWebhookSignature(secret []byte, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

func fulfillmentWebhookRetryDelay(attempt int) time.Duration {
	delays := []time.Duration{
		time.Minute, 5 * time.Minute, 30 * time.Minute,
		2 * time.Hour, 6 * time.Hour, 24 * time.Hour,
	}
	if attempt < 0 {
		attempt = 0
	}
	if attempt >= len(delays) {
		return delays[len(delays)-1]
	}
	return delays[attempt]
}

func clearSecret(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
