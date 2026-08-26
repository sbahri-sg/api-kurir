package fulfillment

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestLifecycleRunnerRegistersPickupAWBInTracking(t *testing.T) {
	t.Parallel()
	repository := &lifecycleRepositoryStub{
		job:      LifecycleJob{ID: "job-1", TenantID: "merchant-1", ShipmentID: "shipment-1", JobType: "register_tracking", MaxAttempts: 3},
		shipment: Shipment{ID: "shipment-1", MerchantReference: "ORDER-1", CourierCode: "jne", AWB: "JY100200300"},
	}
	registrar := &trackingRegistrarStub{trackingShipmentID: "tracking-1"}
	runner := NewRunner(repository, nil, registrar, "test", 1, time.Second, slog.Default())
	if err := runner.processOne(context.Background(), "worker-1"); err != nil {
		t.Fatal(err)
	}
	if registrar.input.FulfillmentReference != "shipment-1" ||
		repository.completedTrackingID != "tracking-1" {
		t.Fatalf("tracking bridge did not complete: input=%+v id=%q", registrar.input, repository.completedTrackingID)
	}
}

func TestLifecycleRunnerReconcilesProviderDetail(t *testing.T) {
	t.Parallel()
	repository := &lifecycleRepositoryStub{
		job:      LifecycleJob{ID: "job-2", TenantID: "merchant-1", ShipmentID: "shipment-2", JobType: "reconcile", MaxAttempts: 3},
		shipment: Shipment{ID: "shipment-2", ProviderCode: "rajaongkir", ProviderShipmentID: "KOM-2", Status: StatusBooked},
	}
	adapter := &detailAdapterStub{stubAdapter: stubAdapter{}, result: ProviderDetailResult{
		AWB: "JY200300400", Status: StatusPickupRequested, ProviderStatus: "Pickup Requested",
	}}
	service := NewService(&memoryRepository{}, stubCatalog{code: "rajaongkir"}, stubCredentials{}, adapter)
	runner := NewRunner(repository, service, &trackingRegistrarStub{}, "test", 1, time.Second, slog.Default())
	if err := runner.processOne(context.Background(), "worker-1"); err != nil {
		t.Fatal(err)
	}
	if repository.reconciled.AWB != "JY200300400" || repository.nextRefresh != nil {
		t.Fatalf("unexpected reconciliation: result=%+v next=%v", repository.reconciled, repository.nextRefresh)
	}
}

type lifecycleRepositoryStub struct {
	job                 LifecycleJob
	shipment            Shipment
	completedTrackingID string
	reconciled          ProviderDetailResult
	nextRefresh         *time.Time
}

func (r *lifecycleRepositoryStub) ClaimLifecycleJob(context.Context, string) (LifecycleJob, error) {
	return r.job, nil
}
func (r *lifecycleRepositoryStub) GetLifecycleShipment(context.Context, string, string) (Shipment, error) {
	return r.shipment, nil
}
func (r *lifecycleRepositoryStub) CompleteTrackingRegistration(_ context.Context, _ LifecycleJob, trackingShipmentID string) error {
	r.completedTrackingID = trackingShipmentID
	return nil
}
func (r *lifecycleRepositoryStub) CompleteReconciliation(_ context.Context, _ LifecycleJob, result ProviderDetailResult, next *time.Time) error {
	r.reconciled = result
	r.nextRefresh = next
	return nil
}
func (*lifecycleRepositoryStub) FailLifecycleJob(context.Context, LifecycleJob, string, string, time.Time) error {
	return nil
}
func (*lifecycleRepositoryStub) EnqueueReconciliation(context.Context, string) error { return nil }

type trackingRegistrarStub struct {
	input              TrackingRegistration
	trackingShipmentID string
}

func (r *trackingRegistrarStub) RegisterFulfillmentTracking(_ context.Context, input TrackingRegistration) (string, error) {
	r.input = input
	return r.trackingShipmentID, nil
}

type detailAdapterStub struct {
	stubAdapter
	result ProviderDetailResult
}

func (a *detailAdapterStub) Detail(context.Context, string, Shipment) (ProviderDetailResult, error) {
	return a.result, nil
}
