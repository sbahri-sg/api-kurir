package tracking

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

type trackingRepositoryStub struct {
	waybill                   string
	waybillMasked             string
	providerContextCiphertext []byte
	job                       Job
	result                    Result
	shipment                  Shipment
	failed                    bool
	removedFulfillment        string
}

func (r *trackingRepositoryStub) Register(
	_ context.Context,
	courierCode string,
	_ string,
	waybill string,
	waybillMasked string,
	providerContextCiphertext []byte,
) (Shipment, error) {
	r.waybill = waybill
	r.waybillMasked = waybillMasked
	r.providerContextCiphertext = append([]byte(nil), providerContextCiphertext...)
	shipment := Shipment{
		ID: "shipment-1", CourierCode: courierCode, WaybillMasked: waybill,
		NormalizedStatus: "unknown", ValidationStatus: "unverified",
		ProviderHitLimit: DefaultProviderHitLimit, RefreshQueued: true,
	}
	r.shipment = shipment
	return shipment, nil
}

func (r *trackingRepositoryStub) RegisterImmediate(
	ctx context.Context,
	courierCode string,
	waybillHash string,
	waybill string,
	waybillMasked string,
	providerContextCiphertext []byte,
) (Shipment, error) {
	if r.shipment.ID != "" {
		return r.shipment, nil
	}
	shipment, err := r.Register(
		ctx,
		courierCode,
		waybillHash,
		waybill,
		waybillMasked,
		providerContextCiphertext,
	)
	if err == nil {
		shipment.RefreshQueued = false
		r.shipment = shipment
	}
	return shipment, err
}

func (r *trackingRepositoryStub) Claim(
	context.Context,
	string,
	[]string,
) (Job, error) {
	return r.job, nil
}

func (r *trackingRepositoryStub) Complete(_ context.Context, _ Job, result Result) error {
	r.result = result
	return nil
}

func (r *trackingRepositoryStub) CompleteImmediate(
	_ context.Context,
	_ string,
	result Result,
) error {
	r.result = result
	fetchedAt := result.FetchedAt
	r.shipment.NormalizedStatus = result.NormalizedStatus
	r.shipment.StatusLabel = result.StatusLabel
	r.shipment.Summary = result.Summary
	r.shipment.Events = result.Events
	r.shipment.ProviderCode = result.ProviderCode
	r.shipment.ProviderFetchedAt = &fetchedAt
	r.shipment.NextRefreshAt = result.NextRefreshAt
	r.shipment.IsFinal = result.IsFinal
	return nil
}

func (r *trackingRepositoryStub) RecordNotFound(
	_ context.Context,
	_ Job,
	fetchedAt time.Time,
	nextRefreshAt *time.Time,
	invalid bool,
) error {
	return r.recordNotFound(fetchedAt, nextRefreshAt, invalid)
}

func (r *trackingRepositoryStub) RecordNotFoundImmediate(
	_ context.Context,
	_ string,
	fetchedAt time.Time,
	nextRefreshAt *time.Time,
	invalid bool,
) error {
	return r.recordNotFound(fetchedAt, nextRefreshAt, invalid)
}

func (r *trackingRepositoryStub) RecordImmediateFailure(
	_ context.Context,
	_ string,
	errorCode string,
	retryAt time.Time,
	countProviderHit bool,
) error {
	r.shipment.LastErrorCode = errorCode
	r.shipment.NextRefreshAt = &retryAt
	if countProviderHit {
		r.shipment.ProviderHitCount++
	}
	return nil
}

func (r *trackingRepositoryStub) recordNotFound(
	fetchedAt time.Time,
	nextRefreshAt *time.Time,
	invalid bool,
) error {
	r.shipment.ProviderFetchedAt = &fetchedAt
	r.shipment.NextRefreshAt = nextRefreshAt
	r.shipment.NotFoundCount++
	r.shipment.ProviderHitCount++
	r.shipment.ValidationStatus = "not_found"
	if invalid {
		r.shipment.ValidationStatus = "invalid"
	}
	return nil
}

func (r *trackingRepositoryStub) UpsertSubscription(
	_ context.Context,
	_ string,
	orderReference, fulfillmentReference string,
	_ int,
) (Subscription, error) {
	return Subscription{
		ID: "subscription-1", OrderReference: orderReference,
		FulfillmentReference: fulfillmentReference, Active: true,
		Shipment: r.shipment,
	}, nil
}

func (r *trackingRepositoryStub) GetSubscription(
	_ context.Context,
	fulfillmentReference string,
) (Subscription, error) {
	return Subscription{
		ID: "subscription-1", FulfillmentReference: fulfillmentReference,
		Active: true, Shipment: r.shipment,
	}, nil
}

func (r *trackingRepositoryStub) DeactivateSubscription(
	_ context.Context,
	fulfillmentReference string,
) (SubscriptionRemoval, error) {
	r.removedFulfillment = fulfillmentReference
	return SubscriptionRemoval{
		ID:                   "subscription-1",
		FulfillmentReference: fulfillmentReference,
		Revision:             1,
		Status:               "removed",
		PollingStopped:       true,
		SnapshotRetained:     true,
	}, nil
}

func (r *trackingRepositoryStub) Fail(
	context.Context,
	Job,
	string,
	string,
	time.Time,
) error {
	r.failed = true
	return nil
}

func testCipher(t *testing.T) *Cipher {
	t.Helper()
	key := bytes.Repeat([]byte{7}, 32)
	cipher, err := NewCipher(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	return cipher
}

func TestServiceStoresPlaintextWaybillAndEncryptsProviderContext(t *testing.T) {
	t.Parallel()

	repository := &trackingRepositoryStub{}
	cipher := testCipher(t)
	service := NewService(repository, cipher)
	shipment, err := service.Register(
		context.Background(),
		"jne",
		"ABC123456789",
		"54321",
	)
	if err != nil {
		t.Fatal(err)
	}
	if shipment.WaybillMasked != "ABC123456789" {
		t.Fatalf("unexpected response waybill: %s", shipment.WaybillMasked)
	}
	if repository.waybill != "ABC123456789" {
		t.Fatalf("unexpected stored waybill: %s", repository.waybill)
	}
	if repository.waybillMasked != "********6789" {
		t.Fatalf("unexpected internal mask: %s", repository.waybillMasked)
	}
	decryptedContext, err := cipher.Decrypt(
		repository.providerContextCiphertext,
		[]byte("jne:provider-context"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(decryptedContext, []byte(`"last_phone_number":"54321"`)) {
		t.Fatalf("unexpected provider context: %s", decryptedContext)
	}
}

type trackingAdapterStub struct{}

func (trackingAdapterStub) Code() string { return "test" }
func (trackingAdapterStub) CourierCodes() []string {
	return []string{"jne"}
}
func (trackingAdapterStub) Track(
	_ context.Context,
	request Request,
) (Result, error) {
	if request.CourierCode != "jne" || request.Waybill != "ABC123456789" {
		return Result{}, ErrInvalidWaybill
	}
	return Result{
		NormalizedStatus: "in_transit",
		StatusLabel:      "Dalam perjalanan",
		ProviderCode:     "test",
		FetchedAt:        time.Now().UTC(),
	}, nil
}

func TestServiceDoesNotRequirePhoneSuffixForJNE(t *testing.T) {
	t.Parallel()

	repository := &trackingRepositoryStub{}
	service := NewService(repository, testCipher(t), "jne")
	if _, err := service.Register(
		context.Background(),
		"jne",
		"ABC123456789",
		"",
	); err != nil {
		t.Fatalf("phone suffix should be optional, got %v", err)
	}
	if len(repository.providerContextCiphertext) != 0 {
		t.Fatal("provider context should stay empty when no phone suffix is supplied")
	}
}

func TestServiceRejectsUnsupportedCourier(t *testing.T) {
	t.Parallel()

	service := NewService(&trackingRepositoryStub{}, testCipher(t), "jne")
	_, err := service.Register(context.Background(), "sicepat", "ABC123456789", "")
	if !errors.Is(err, ErrUnsupportedCourier) {
		t.Fatalf("expected unsupported courier, got %v", err)
	}
}

type countingTrackingAdapterStub struct {
	calls  int
	result Result
	err    error
}

func (a *countingTrackingAdapterStub) Code() string { return "test" }
func (a *countingTrackingAdapterStub) CourierCodes() []string {
	return []string{"jne"}
}
func (a *countingTrackingAdapterStub) Track(
	context.Context,
	Request,
) (Result, error) {
	a.calls++
	return a.result, a.err
}

func TestTrackNowReusesFreshPersistentSnapshot(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC)
	nextRefreshAt := now.Add(time.Hour)
	repository := &trackingRepositoryStub{}
	service := NewService(repository, testCipher(t), "jne")
	service.now = func() time.Time { return now }
	adapter := &countingTrackingAdapterStub{result: Result{
		NormalizedStatus: "in_transit",
		StatusLabel:      "Dalam perjalanan",
		Summary:          map[string]any{"status": "IN TRANSIT"},
		ProviderCode:     "rajaongkir",
		FetchedAt:        now,
		NextRefreshAt:    &nextRefreshAt,
	}}

	first, err := service.TrackNow(
		context.Background(),
		adapter,
		"jne",
		"ABC123456789",
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.TrackNow(
		context.Background(),
		adapter,
		"jne",
		"ABC123456789",
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if adapter.calls != 1 {
		t.Fatalf("provider calls: got %d want 1", adapter.calls)
	}
	if first.NormalizedStatus != "in_transit" ||
		second.NormalizedStatus != "in_transit" {
		t.Fatalf("unexpected results: first=%#v second=%#v", first, second)
	}
}

func TestTrackNowNegativeCachesUnknownWaybill(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	repository := &trackingRepositoryStub{}
	service := NewService(repository, testCipher(t), "jne")
	service.now = func() time.Time { return now }
	adapter := &countingTrackingAdapterStub{err: ErrWaybillNotFound}

	for attempt := 0; attempt < 2; attempt++ {
		_, err := service.TrackNow(context.Background(), adapter, "jne", "RANDOM123456", "")
		if !errors.Is(err, ErrWaybillNotFound) {
			t.Fatalf("attempt %d: expected not found, got %v", attempt+1, err)
		}
	}
	if adapter.calls != 1 {
		t.Fatalf("negative cache must suppress repeat provider call, got %d", adapter.calls)
	}
	if repository.shipment.NextRefreshAt == nil ||
		repository.shipment.NextRefreshAt.Sub(now) != 12*time.Hour {
		t.Fatalf("unexpected negative cache checkpoint: %#v", repository.shipment.NextRefreshAt)
	}
}

func TestTrackNowReturnsLastSnapshotAfterHitBudgetStops(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	repository := &trackingRepositoryStub{shipment: Shipment{
		ID: "shipment-1", CourierCode: "jne", WaybillMasked: "********6789",
		NormalizedStatus: "in_transit", StatusLabel: "Dalam perjalanan",
		ValidationStatus: "valid", ProviderFetchedAt: &now,
		ProviderHitCount: 10, ProviderHitLimit: 10, PollingStopped: true,
	}}
	service := NewService(repository, testCipher(t), "jne")
	service.now = func() time.Time { return now.Add(24 * time.Hour) }
	adapter := &countingTrackingAdapterStub{}
	result, err := service.TrackNow(context.Background(), adapter, "jne", "ABC123456789", "")
	if err != nil {
		t.Fatal(err)
	}
	if adapter.calls != 0 || result.NormalizedStatus != "in_transit" {
		t.Fatalf("budget stop must return snapshot without provider: result=%#v calls=%d", result, adapter.calls)
	}
}

func TestTrackNowCachesProviderFailureUntilRetryCheckpoint(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	repository := &trackingRepositoryStub{}
	service := NewService(repository, testCipher(t), "jne")
	service.now = func() time.Time { return now }
	adapter := &countingTrackingAdapterStub{err: ErrProviderTimeout}
	for attempt := 0; attempt < 2; attempt++ {
		_, err := service.TrackNow(context.Background(), adapter, "jne", "TIMEOUT123456", "")
		if !errors.Is(err, ErrProviderTimeout) {
			t.Fatalf("attempt %d: expected timeout, got %v", attempt+1, err)
		}
	}
	if adapter.calls != 1 {
		t.Fatalf("provider error cache must suppress repeat call, got %d", adapter.calls)
	}
	if repository.shipment.NextRefreshAt == nil ||
		repository.shipment.NextRefreshAt.Sub(now) != time.Hour {
		t.Fatalf("unexpected provider retry checkpoint: %#v", repository.shipment.NextRefreshAt)
	}
}

func TestRunnerUsesPlaintextWaybillForAdapter(t *testing.T) {
	t.Parallel()

	cipher := testCipher(t)
	repository := &trackingRepositoryStub{
		job: Job{
			ID: "job-1", ShipmentID: "shipment-1", CourierCode: "jne",
			Waybill: "ABC123456789", MaxAttempts: 3,
		},
	}
	runner := NewRunner(
		repository,
		cipher,
		[]Adapter{trackingAdapterStub{}},
		"worker-test",
		1,
		time.Millisecond,
		discardLogger(),
	)
	if err := runner.processOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repository.result.NormalizedStatus != "in_transit" || repository.failed {
		t.Fatalf("unexpected runner result: %#v", repository.result)
	}
}
