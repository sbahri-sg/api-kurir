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
	ciphertext                []byte
	providerContextCiphertext []byte
	job                       Job
	result                    Result
	shipment                  Shipment
	failed                    bool
}

func (r *trackingRepositoryStub) Register(
	_ context.Context,
	courierCode string,
	_ string,
	waybillMasked string,
	waybillCiphertext []byte,
	providerContextCiphertext []byte,
) (Shipment, error) {
	r.ciphertext = append([]byte(nil), waybillCiphertext...)
	r.providerContextCiphertext = append([]byte(nil), providerContextCiphertext...)
	shipment := Shipment{
		ID: "shipment-1", CourierCode: courierCode, WaybillMasked: waybillMasked,
		NormalizedStatus: "unknown", RefreshQueued: true,
	}
	r.shipment = shipment
	return shipment, nil
}

func (r *trackingRepositoryStub) RegisterImmediate(
	ctx context.Context,
	courierCode string,
	waybillHash string,
	waybillMasked string,
	waybillCiphertext []byte,
	providerContextCiphertext []byte,
) (Shipment, error) {
	if r.shipment.ProviderFetchedAt != nil {
		return r.shipment, nil
	}
	shipment, err := r.Register(
		ctx,
		courierCode,
		waybillHash,
		waybillMasked,
		waybillCiphertext,
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

func TestServiceEncryptsAndMasksWaybill(t *testing.T) {
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
	if shipment.WaybillMasked != "********6789" {
		t.Fatalf("unexpected mask: %s", shipment.WaybillMasked)
	}
	if bytes.Contains(repository.ciphertext, []byte("ABC123456789")) {
		t.Fatal("ciphertext contains plaintext waybill")
	}
	decrypted, err := cipher.Decrypt(repository.ciphertext, []byte("jne"))
	if err != nil {
		t.Fatal(err)
	}
	if string(decrypted) != "ABC123456789" {
		t.Fatalf("unexpected decrypted value: %s", decrypted)
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

func TestRunnerDecryptsOnlyForAdapter(t *testing.T) {
	t.Parallel()

	cipher := testCipher(t)
	encrypted, err := cipher.Encrypt([]byte("ABC123456789"), []byte("jne"))
	if err != nil {
		t.Fatal(err)
	}
	repository := &trackingRepositoryStub{
		job: Job{
			ID: "job-1", ShipmentID: "shipment-1", CourierCode: "jne",
			WaybillCiphertext: encrypted, MaxAttempts: 3,
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
