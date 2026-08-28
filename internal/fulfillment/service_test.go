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

func TestCreateReplayPreservesFailedProviderResult(t *testing.T) {
	t.Parallel()
	repository := &memoryRepository{shipment: &Shipment{
		ID: "2d0bc09b-28e3-48e2-b513-d481605ea963", ProviderCode: "rajaongkir",
		Status: StatusBookingFailed, ProviderStatus: "unauthorized",
	}}
	adapter := &stubAdapter{}
	service := NewService(repository, stubCatalog{code: "rajaongkir"}, stubCredentials{}, adapter)
	ctx := tenancy.WithIdentity(context.Background(), tenancy.Identity{TenantID: "merchant-1"})

	_, replayed, err := service.Create(ctx, "shipment:create:failed", validCreateRequest())
	if !errors.Is(err, ErrProviderUnauthorized) {
		t.Fatalf("expected original provider error, got %v", err)
	}
	if replayed || adapter.createCalls != 0 {
		t.Fatalf("failed booking must not be replayed as success: replayed=%v calls=%d", replayed, adapter.createCalls)
	}
}

func TestCanonicalQuoteOverridesProviderNativeValues(t *testing.T) {
	t.Parallel()
	repository := &memoryRepository{}
	adapter := &stubAdapter{quotes: []ProviderQuote{{
		CourierCode: "jne", CourierName: "JNE", ServiceCode: "JNEFlat",
		ServiceGroup: "regular", DeliveryMode: "regular", ShippingCost: 10500,
		GrandTotal: 110500, Currency: "IDR", ExpiresAt: time.Now().Add(time.Hour),
	}}}
	service := NewService(repository, stubCatalog{code: "rajaongkir"}, stubCredentials{}, adapter)
	ctx := tenancy.WithIdentity(context.Background(), tenancy.Identity{TenantID: "merchant-1"})
	quotes, err := service.Quotes(ctx, validQuoteRequest())
	if err != nil || len(quotes) != 1 {
		t.Fatalf("quote failed: quotes=%+v err=%v", quotes, err)
	}
	request := validCreateRequest()
	request.QuoteID = quotes[0].ID
	request.ServiceCode = quotes[0].ServiceCode
	request.Payment.ShippingCost = quotes[0].ShippingCost
	request.Payment.ShippingDiscount = 500
	request.Payment.GrandTotal = quotes[0].GrandTotal - 500
	shipment, _, err := service.Create(ctx, "shipment:create:canonical", request)
	if err != nil {
		t.Fatal(err)
	}
	if adapter.lastCreate.ServiceCode != "JNEFlat" {
		t.Fatalf("provider must receive native code, got %q", adapter.lastCreate.ServiceCode)
	}
	if adapter.lastCreate.Payment.ShippingDiscount != 500 || adapter.lastCreate.Payment.GrandTotal != 110000 {
		t.Fatalf("merchant discount must survive quote lock: %+v", adapter.lastCreate.Payment)
	}
	if shipment.ServiceCode != quotes[0].ServiceCode || shipment.ShippingCost != 10500 {
		t.Fatalf("public shipment must retain canonical quote: %+v", shipment)
	}
}

func TestCanonicalQuoteRejectsManipulatedPrice(t *testing.T) {
	t.Parallel()
	repository := &memoryRepository{}
	adapter := &stubAdapter{quotes: []ProviderQuote{{
		CourierCode: "jne", ServiceCode: "JNEFlat", ServiceGroup: "regular",
		DeliveryMode: "regular", ShippingCost: 10500, GrandTotal: 110500,
		Currency: "IDR", ExpiresAt: time.Now().Add(time.Hour),
	}}}
	service := NewService(repository, stubCatalog{code: "rajaongkir"}, stubCredentials{}, adapter)
	ctx := tenancy.WithIdentity(context.Background(), tenancy.Identity{TenantID: "merchant-1"})
	quotes, err := service.Quotes(ctx, validQuoteRequest())
	if err != nil {
		t.Fatal(err)
	}
	request := validCreateRequest()
	request.QuoteID = quotes[0].ID
	request.ServiceCode = quotes[0].ServiceCode
	request.Payment.ShippingCost = 999
	_, _, err = service.Create(ctx, "shipment:create:manipulated", request)
	if !errors.Is(err, ErrQuoteMismatch) {
		t.Fatalf("expected quote mismatch, got %v", err)
	}
}

func TestCreateAcceptsDiscountedItemTotals(t *testing.T) {
	t.Parallel()
	repository := &memoryRepository{}
	adapter := &stubAdapter{create: ProviderCreateResult{ProviderShipmentID: "KOM-200", Status: StatusBooked}}
	service := NewService(repository, stubCatalog{code: "rajaongkir"}, stubCredentials{}, adapter)
	ctx := tenancy.WithIdentity(context.Background(), tenancy.Identity{TenantID: "merchant-1"})
	request := validCreateRequest()
	request.Package.WeightGrams = 1350
	request.Package.Items[0].UnitPrice = 150000
	request.Package.Items[0].Subtotal = 130000
	request.Payment = Payment{
		Type: "non_cod", ItemsSubtotal: 130000, OrderDiscount: 0, TaxAmount: 0,
		ShippingCost: 18000, ShippingDiscount: 5000, AdditionalCost: 0,
		GrandTotal: 143000,
	}
	if _, _, err := service.Create(ctx, "shipment:create:discount", request); err != nil {
		t.Fatal(err)
	}
	if adapter.createCalls != 1 || adapter.lastCreate.Payment.ShippingDiscount != 5000 {
		t.Fatalf("discounted payment was not forwarded: %+v", adapter.lastCreate.Payment)
	}
}

func TestCreateRejectsInconsistentAmountsBeforeProviderCall(t *testing.T) {
	t.Parallel()
	adapter := &stubAdapter{}
	service := NewService(&memoryRepository{}, stubCatalog{code: "rajaongkir"}, stubCredentials{}, adapter)
	ctx := tenancy.WithIdentity(context.Background(), tenancy.Identity{TenantID: "merchant-1"})
	request := validCreateRequest()
	request.Payment.ItemsSubtotal = 99999
	_, _, err := service.Create(ctx, "shipment:create:amount-mismatch", request)
	if !errors.Is(err, ErrAmountMismatch) || adapter.createCalls != 0 {
		t.Fatalf("expected local amount rejection, err=%v calls=%d", err, adapter.createCalls)
	}
}

func TestPickupRejectsShipmentWithoutSuccessfulBooking(t *testing.T) {
	t.Parallel()
	repository := &memoryRepository{shipment: &Shipment{
		ID: "2d0bc09b-28e3-48e2-b513-d481605ea963", ProviderCode: "rajaongkir",
		Fulfillment: "pickup", Status: StatusBookingFailed, ProviderStatus: "unauthorized",
	}}
	adapter := &stubAdapter{}
	service := NewService(repository, stubCatalog{code: "rajaongkir"}, stubCredentials{}, adapter)
	ctx := tenancy.WithIdentity(context.Background(), tenancy.Identity{TenantID: "merchant-1"})

	_, replayed, err := service.Pickup(ctx, repository.shipment.ID, "shipment:pickup:failed", PickupRequest{
		Mode: "scheduled", ScheduledAt: time.Now().Add(time.Hour),
	})
	if !errors.Is(err, ErrPickupNotAllowed) {
		t.Fatalf("expected pickup guard, got %v", err)
	}
	if replayed || adapter.pickupCalls != 0 {
		t.Fatalf("provider must not be called: replayed=%v calls=%d", replayed, adapter.pickupCalls)
	}
}

func TestPickupRejectsBookedShipmentWithoutProviderID(t *testing.T) {
	t.Parallel()
	repository := &memoryRepository{shipment: &Shipment{
		ID: "05fc3385-7555-4aa2-8f0b-da00ba96bbd0", ProviderCode: "rajaongkir",
		Fulfillment: "pickup", Status: StatusBooked,
	}}
	adapter := &stubAdapter{}
	service := NewService(repository, stubCatalog{code: "rajaongkir"}, stubCredentials{}, adapter)
	ctx := tenancy.WithIdentity(context.Background(), tenancy.Identity{TenantID: "merchant-1"})

	_, _, err := service.Pickup(ctx, repository.shipment.ID, "shipment:pickup:missing-provider-id", PickupRequest{
		Mode: "scheduled", ScheduledAt: time.Now().Add(time.Hour),
	})
	if !errors.Is(err, ErrPickupNotAllowed) || adapter.pickupCalls != 0 {
		t.Fatalf("expected local pickup rejection without provider call, err=%v calls=%d", err, adapter.pickupCalls)
	}
}

func TestPickupRequestedShipmentReturnsReplayWithoutProviderCall(t *testing.T) {
	t.Parallel()
	repository := &memoryRepository{shipment: &Shipment{
		ID: "87e045ae-e419-437a-b2f9-b100131c35e5", ProviderCode: "rajaongkir",
		ProviderShipmentID: "KOM-101", Fulfillment: "pickup", Status: StatusPickupRequested,
	}}
	adapter := &stubAdapter{}
	service := NewService(repository, stubCatalog{code: "rajaongkir"}, stubCredentials{}, adapter)
	ctx := tenancy.WithIdentity(context.Background(), tenancy.Identity{TenantID: "merchant-1"})

	shipment, replayed, err := service.Pickup(ctx, repository.shipment.ID, "shipment:pickup:replay", PickupRequest{
		Mode: "scheduled", ScheduledAt: time.Now().Add(time.Hour),
	})
	if err != nil || !replayed || shipment.ID != repository.shipment.ID || adapter.pickupCalls != 0 {
		t.Fatalf("expected safe replay without provider call, shipment=%+v replayed=%v err=%v calls=%d", shipment, replayed, err, adapter.pickupCalls)
	}
}

func TestPickupNowHidesProviderContextFromMerchantRequest(t *testing.T) {
	t.Parallel()
	repository := &memoryRepository{shipment: &Shipment{
		ID: "47d5c048-c6a0-4c99-a1bf-5eef477a8684", ProviderCode: "rajaongkir",
		ProviderShipmentID: "KOM-102", Fulfillment: "pickup", Status: StatusBooked,
		PackageWeightGrams: 12_000,
	}}
	adapter := &stubAdapter{}
	service := NewService(repository, stubCatalog{code: "rajaongkir"}, stubCredentials{}, adapter)
	ctx := tenancy.WithIdentity(context.Background(), tenancy.Identity{TenantID: "merchant-1"})

	before := time.Now().UTC()
	_, replayed, err := service.Pickup(ctx, repository.shipment.ID, "shipment:pickup:now", PickupRequest{Mode: "now"})
	if err != nil || replayed || adapter.pickupCalls != 1 {
		t.Fatalf("pickup now failed: replayed=%v err=%v calls=%d", replayed, err, adapter.pickupCalls)
	}
	if adapter.lastPickup.PackageWeightGrams != 12_000 ||
		adapter.lastPickup.ScheduledAt.Before(before.Add(14*time.Minute)) {
		t.Fatalf("provider context not derived by gateway: %+v", adapter.lastPickup)
	}
}

func TestPickupScheduledRejectsPastTimestamp(t *testing.T) {
	t.Parallel()
	service := NewService(&memoryRepository{}, stubCatalog{code: "rajaongkir"}, stubCredentials{}, &stubAdapter{})
	ctx := tenancy.WithIdentity(context.Background(), tenancy.Identity{TenantID: "merchant-1"})
	_, _, err := service.Pickup(ctx, "47d5c048-c6a0-4c99-a1bf-5eef477a8684", "shipment:pickup:past", PickupRequest{
		Mode: "scheduled", ScheduledAt: time.Now().Add(-time.Minute),
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected invalid scheduled pickup, got %v", err)
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
			Items: []Item{{Name: "Kaos", Quantity: 1, UnitPrice: 100000, Subtotal: 100000, WeightGrams: 1000}},
		},
		Payment: Payment{
			Type: "non_cod", ItemsSubtotal: 100000, ShippingCost: 18000,
			GrandTotal: 118000,
		},
	}
}

func validQuoteRequest() QuoteRequest {
	return QuoteRequest{
		Origin:      QuoteLocation{DestinationID: 5969},
		Destination: QuoteLocation{DestinationID: 4956},
		Package:     QuotePackage{WeightGrams: 1000, LengthCM: 20, WidthCM: 10, HeightCM: 5, ItemValue: 100000},
		PaymentType: "non_cod", CourierCodes: []string{"jne"},
		ServiceGroups: []string{"regular"},
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
	quotes      []ProviderQuote
	createCalls int
	pickupCalls int
	lastCreate  CreateRequest
	lastPickup  PickupRequest
}

func (s *stubAdapter) Code() string { return "rajaongkir" }
func (s *stubAdapter) Create(_ context.Context, _ string, request CreateRequest) (ProviderCreateResult, error) {
	s.lastCreate = request
	s.createCalls++
	return s.create, nil
}
func (s *stubAdapter) Quote(context.Context, string, QuoteRequest) ([]ProviderQuote, error) {
	return s.quotes, nil
}
func (s *stubAdapter) Pickup(_ context.Context, _ string, _ Shipment, request PickupRequest) (ProviderPickupResult, error) {
	s.pickupCalls++
	s.lastPickup = request
	return ProviderPickupResult{}, nil
}
func (s *stubAdapter) Label(context.Context, string, Shipment, string) (Label, error) {
	return Label{}, nil
}
func (s *stubAdapter) Cancel(context.Context, string, Shipment, CancelRequest) (ProviderCancelResult, error) {
	return ProviderCancelResult{}, nil
}

type memoryRepository struct {
	shipment *Shipment
	quotes   map[string]Quote
}

func (r *memoryRepository) SaveQuotes(_ context.Context, _ string, quotes []Quote) error {
	if r.quotes == nil {
		r.quotes = make(map[string]Quote)
	}
	for _, quote := range quotes {
		r.quotes[quote.ID] = quote
	}
	return nil
}

func (r *memoryRepository) GetQuote(_ context.Context, _ string, quoteID string) (Quote, error) {
	quote, ok := r.quotes[quoteID]
	if !ok {
		return Quote{}, ErrQuoteNotFound
	}
	return quote, nil
}

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
		Currency: input.Currency, PackageWeightGrams: input.PackageWeightGrams,
		CreatedAt: now, UpdatedAt: now,
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
