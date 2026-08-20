package tracking

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type webhookRepositoryStub struct {
	job             WebhookJob
	completedStatus int
	failed          bool
	claimCount      int
}

func (r *webhookRepositoryStub) ClaimWebhook(context.Context, string) (WebhookJob, error) {
	r.claimCount++
	return r.job, nil
}

func (r *webhookRepositoryStub) CompleteWebhook(_ context.Context, _ string, status int) error {
	r.completedStatus = status
	return nil
}

type webhookDestinationStub struct {
	endpoint string
	secret   string
	enabled  bool
}

func (r *webhookDestinationStub) ResolveWebhookDestination(
	context.Context,
) (string, []byte, bool, error) {
	return r.endpoint, []byte(r.secret), r.enabled, nil
}

func (r *webhookRepositoryStub) FailWebhook(
	context.Context,
	WebhookJob,
	int,
	string,
	time.Time,
	bool,
) error {
	r.failed = true
	return nil
}

func TestWebhookDispatcherDoesNotClaimOutboxWhileDashboardSettingDisabled(t *testing.T) {
	t.Parallel()
	repository := &webhookRepositoryStub{}
	destination := &webhookDestinationStub{}
	dispatcher := NewResolvingWebhookDispatcher(
		repository,
		destination,
		"worker",
		1,
		time.Second,
		time.Second,
		discardLogger(),
	)
	if err := dispatcher.processOne(context.Background(), "worker-1"); err != ErrWebhookDisabled {
		t.Fatalf("expected disabled webhook, got %v", err)
	}
	if repository.claimCount != 0 {
		t.Fatalf("disabled webhook claimed %d outbox jobs", repository.claimCount)
	}
}

func TestWebhookDispatcherSignsExactRequestBody(t *testing.T) {
	t.Parallel()
	secret := "01234567890123456789012345678901"
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	repository := &webhookRepositoryStub{job: WebhookJob{
		ID: "event-1", EventType: "tracking.delivered",
		Data:        map[string]any{"fulfillment_id": "fulfillment-1"},
		MaxAttempts: 10, CreatedAt: now,
	}}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		timestamp := request.Header.Get("X-Emisell-Webhook-Timestamp")
		want := webhookSignature([]byte(secret), timestamp, body)
		if got := request.Header.Get("X-Emisell-Webhook-Signature"); got != want {
			t.Fatalf("signature=%q want=%q", got, want)
		}
		if request.Header.Get("X-Emisell-Event-ID") != "event-1" {
			t.Fatalf("missing event id header")
		}
		writer.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	dispatcher := NewWebhookDispatcher(
		repository, server.URL, secret, "worker", 1,
		time.Second, time.Second, discardLogger(),
	)
	dispatcher.now = func() time.Time { return now }
	if err := dispatcher.processOne(context.Background(), "worker-1"); err != nil {
		t.Fatal(err)
	}
	if repository.completedStatus != http.StatusAccepted || repository.failed {
		t.Fatalf("unexpected delivery state: %#v", repository)
	}
}
