package fulfillment

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fulfillmentWebhookRepositoryStub struct {
	job             WebhookJob
	claimCount      int
	completedStatus int
	failed          bool
}

func (r *fulfillmentWebhookRepositoryStub) ClaimFulfillmentWebhook(context.Context, string) (WebhookJob, error) {
	r.claimCount++
	return r.job, nil
}

func (r *fulfillmentWebhookRepositoryStub) CompleteFulfillmentWebhook(_ context.Context, _ string, status int) error {
	r.completedStatus = status
	return nil
}

func (r *fulfillmentWebhookRepositoryStub) FailFulfillmentWebhook(
	context.Context, WebhookJob, int, string, time.Time, bool,
) error {
	r.failed = true
	return nil
}

type fulfillmentWebhookDestinationStub struct {
	endpoint string
	secret   string
	enabled  bool
}

func (d *fulfillmentWebhookDestinationStub) ResolveWebhookDestination(context.Context) (string, []byte, bool, error) {
	return d.endpoint, []byte(d.secret), d.enabled, nil
}

func TestFulfillmentWebhookDoesNotClaimWhenDisabled(t *testing.T) {
	t.Parallel()
	repository := &fulfillmentWebhookRepositoryStub{}
	dispatcher := NewWebhookDispatcher(
		repository, &fulfillmentWebhookDestinationStub{}, "worker", 1,
		time.Second, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err := dispatcher.processOne(context.Background(), "worker-1"); err == nil {
		t.Fatal("expected disabled webhook error")
	}
	if repository.claimCount != 0 {
		t.Fatalf("disabled webhook claimed %d outbox jobs", repository.claimCount)
	}
}

func TestFulfillmentWebhookSignsExactRequestBody(t *testing.T) {
	t.Parallel()
	secret := "01234567890123456789012345678901"
	now := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	repository := &fulfillmentWebhookRepositoryStub{job: WebhookJob{
		ID: "event-1", EventType: "shipment.awb_created",
		Data:        map[string]any{"fulfillment_id": "shipment-1"},
		MaxAttempts: 8, CreatedAt: now,
	}}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		timestamp := request.Header.Get("X-Emisell-Webhook-Timestamp")
		want := fulfillmentWebhookSignature([]byte(secret), timestamp, body)
		if got := request.Header.Get("X-Emisell-Webhook-Signature"); got != want {
			t.Fatalf("signature=%q want=%q", got, want)
		}
		if request.Header.Get("X-Emisell-Event-Type") != "shipment.awb_created" {
			t.Fatal("missing event type header")
		}
		writer.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	dispatcher := NewWebhookDispatcher(
		repository,
		&fulfillmentWebhookDestinationStub{endpoint: server.URL, secret: secret, enabled: true},
		"worker", 1, time.Second, time.Second,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	dispatcher.now = func() time.Time { return now }
	if err := dispatcher.processOne(context.Background(), "worker-1"); err != nil {
		t.Fatal(err)
	}
	if repository.completedStatus != http.StatusAccepted || repository.failed {
		t.Fatalf("unexpected delivery state: %#v", repository)
	}
}
