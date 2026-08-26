package fulfillment

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/tenancy"
)

func TestCreatePinsBuiltInProviderAndReplaysIdempotently(t *testing.T) {
	t.Parallel()
	repository := &memoryRepository{}
	adapter := &stubAdapter{create: ProviderCreateResult{
		ProviderShipmentID: "KOM-100", Status: StatusBooked,
	}}
	service := NewService(repository, stubCatalog{code: "emisell"}, stubCredentials{}, adapter)
	ctx := tenancy.WithIdentity(context.Background(), tenancy.Identity{TenantID: "merchant-1"})

	first, replayed, err := service.Create(ctx, "shipment:create:100", validCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	if replayed || first.ProviderCode != "rajaongkir" || first.ProviderShipmentID != "KOM-100" {
		t.Fatalf("unexpected first create: %+v replayed=%v", first, replayed)
	}
	second, replayed, err := service.Create(ctx, "shipment:create:100", validCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	if !replayed || second.ID != first.ID || adapter.createCalls != 1 {
		t.Fatalf("idempotency failed: second=%+v calls=%d", second, adapter.createCalls)
	}
}

func TestCreateRejectsProviderOutsideActiveSelection(t *testing.T) {
	t.Parallel()
	service := NewService(&memoryRepository{}, stubCatalog{code: "rajaongkir"}, stubCredentials{}, &stubAdapter{})
	ctx := tenancy.WithIdentity(context.Background(), tenancy.Identity{TenantID: "merchant-1"})
	request := validCreateRequest()
	request.ProviderCode = "biteship"
	_, _, err := service.Create(ctx, "shipment:create:101", request)
	if !errors.Is(err, ErrProviderUnsupported) {
		t.Fatalf("expected provider rejection, got %v", err)
	}
}

func TestCreateRequiresDeliveryCredential(t *testing.T) {
	t.Parallel()
	service := NewService(&memoryRepository{}, stubCatalog{code: "rajaongkir"}, stubCredentials{err: ErrCredentialUnavailable}, &stubAdapter{})
	ctx := tenancy.WithIdentity(context.Background(), tenancy.Identity{TenantID: "merchant-1"})
	_, _, err := service.Create(ctx, "shipment:create:102", validCreateRequest())
	if !errors.Is(err, ErrCredentialUnavailable) {
		t.Fatalf("expected credential error, got %v", err)
	}
}

func validCreateRequest() CreateRequest {
	return CreateRequest{
		MerchantReference: "ORDER-100", QuoteID: "quote-100",
		CourierCode: "jne", ServiceCode: "REG", DeliveryMode: "regular",
		Fulfillment: "pickup",
		Sender:      Address{Name: "Toko", Phone: "081200000001", Address: "Jalan A", DestinationID: 5969},
		Recipient:   Address{Name: "Budi", Phone: "081200000002", Address: "Jalan B", DestinationID: 4956},
		Package: Package{
			WeightGrams: 1000, LengthCM: 20, WidthCM: 10, HeightCM: 5,
			ItemValue: 100000, Contents: "Pakaian",
			Items: []Item{{Name: "Kaos", Quantity: 1, UnitValue: 100000, WeightGrams: 1000}},
		},
		Payment: Payment{Type: "non_cod", ShippingCost: 18000, GrandTotal: 118000},
	}
}

type stubCatalog struct{ code string }

func (s stubCatalog) ActiveProviderCode(context.Context, string) (string, error) { return s.code, nil }

type stubCredentials struct{ err error }

func (s stubCredentials) ResolveProviderCredentialForCapability(context.Context, string, string) (string, string, int64, error) {
	if s.err != nil {
		return "", "", 0, s.err
	}
	return "delivery-key", "test", 100, nil
}

type stubAdapter struct {
	create      ProviderCreateResult
	createCalls int
}

func (s *stubAdapter) Code() string { return "rajaongkir" }
func (s *stubAdapter) Create(context.Context, string, CreateRequest) (ProviderCreateResult, error) {
	s.createCalls++
	return s.create, nil
}
func (s *stubAdapter) Pickup(context.Context, string, Shipment, PickupRequest) (ProviderPickupResult, error) {
	return ProviderPickupResult{}, nil
}
func (s *stubAdapter) Label(context.Context, string, Shipment, string) (Label, error) {
	return Label{}, nil
}
func (s *stubAdapter) Cancel(context.Context, string, Shipment, CancelRequest) (ProviderCancelResult, error) {
	return ProviderCancelResult{}, nil
}

type memoryRepository struct{ shipment *Shipment }

func (r *memoryRepository) ReserveCreate(_ context.Context, input ReserveCreateInput) (Shipment, bool, error) {
	if r.shipment != nil {
		return *r.shipment, false, nil
	}
	now := time.Now().UTC()
	shipment := Shipment{
		ID: input.ID, MerchantReference: input.MerchantReference,
		ProviderCode: input.ProviderCode, QuoteID: input.QuoteID,
		CourierCode: input.CourierCode, ServiceCode: input.ServiceCode,
		DeliveryMode: input.DeliveryMode, Fulfillment: input.Fulfillment,
		Status: StatusBookingPending, ShippingCost: input.ShippingCost,
		Currency: input.Currency, CreatedAt: now, UpdatedAt: now,
	}
	r.shipment = &shipment
	return shipment, true, nil
}
func (r *memoryRepository) CompleteCreate(_ context.Context, _, _ string, result ProviderCreateResult) (Shipment, error) {
	r.shipment.ProviderShipmentID = result.ProviderShipmentID
	r.shipment.Status = result.Status
	return *r.shipment, nil
}
func (*memoryRepository) FailCreate(context.Context, string, string, string) error { return nil }
func (r *memoryRepository) Get(context.Context, string, string) (Shipment, error) {
	if r.shipment == nil {
		return Shipment{}, ErrShipmentNotFound
	}
	return *r.shipment, nil
}
func (*memoryRepository) ReserveOperation(context.Context, ReserveOperationInput) (bool, error) {
	return true, nil
}
func (r *memoryRepository) CompletePickup(context.Context, string, string, string, ProviderPickupResult) (Shipment, error) {
	return *r.shipment, nil
}
func (r *memoryRepository) CompleteCancel(context.Context, string, string, string, ProviderCancelResult) (Shipment, error) {
	return *r.shipment, nil
}
func (*memoryRepository) FailOperation(context.Context, string, string, string, string, string) error {
	return nil
}
func (*memoryRepository) History(context.Context, string, string) ([]HistoryEvent, error) {
	return nil, nil
}
func (*memoryRepository) GetLabel(context.Context, string, string, string) (Label, error) {
	return Label{}, ErrLabelUnavailable
}
func (*memoryRepository) SaveLabel(context.Context, string, string, Label) error { return nil }
