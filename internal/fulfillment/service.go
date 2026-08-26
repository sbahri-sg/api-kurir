package fulfillment

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/emisell/api-kurir/internal/couriers"
	"github.com/emisell/api-kurir/internal/merchantproviders"
	"github.com/emisell/api-kurir/internal/providercredentials"
	"github.com/emisell/api-kurir/internal/tenancy"
)

var idempotencyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,128}$`)
var shipmentIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-4[0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

type Service struct {
	repository  Repository
	catalog     ProviderCatalog
	credentials CredentialResolver
	adapters    map[string]Adapter
}

func NewService(
	repository Repository,
	catalog ProviderCatalog,
	credentials CredentialResolver,
	adapters ...Adapter,
) *Service {
	registry := make(map[string]Adapter, len(adapters))
	for _, adapter := range adapters {
		if adapter != nil {
			registry[strings.ToLower(strings.TrimSpace(adapter.Code()))] = adapter
		}
	}
	return &Service{
		repository: repository, catalog: catalog,
		credentials: credentials, adapters: registry,
	}
}

func (s *Service) Create(
	ctx context.Context,
	idempotencyKey string,
	request CreateRequest,
) (Shipment, bool, error) {
	tenantID, err := tenant(ctx)
	if err != nil {
		return Shipment{}, false, err
	}
	idempotencyKey, err = normalizeIdempotencyKey(idempotencyKey)
	if err != nil {
		return Shipment{}, false, err
	}
	request = normalizeCreateRequest(request)
	if err := validateCreateRequest(request); err != nil {
		return Shipment{}, false, err
	}
	providerCode, adapter, credential, err := s.resolveProvider(
		ctx, tenantID, request.ProviderCode, "shipments:write",
	)
	if err != nil {
		return Shipment{}, false, err
	}
	request.ProviderCode = providerCode
	payload, requestHash, err := requestPayload(request)
	if err != nil {
		return Shipment{}, false, err
	}
	shipmentID, err := newUUID()
	if err != nil {
		return Shipment{}, false, err
	}
	reserved, created, err := s.repository.ReserveCreate(ctx, ReserveCreateInput{
		ID: shipmentID, TenantID: tenantID, ProviderCode: providerCode,
		MerchantReference: request.MerchantReference, QuoteID: request.QuoteID,
		CourierCode: request.CourierCode, ServiceCode: request.ServiceCode,
		DeliveryMode: request.DeliveryMode, Fulfillment: request.Fulfillment,
		ShippingCost: request.Payment.ShippingCost, Currency: "IDR",
		IdempotencyKey: idempotencyKey, RequestHash: requestHash,
		RequestCiphertext: payload,
	})
	if err != nil {
		return Shipment{}, false, err
	}
	if !created {
		if reserved.ProviderShipmentID == "" && reserved.Status == StatusBookingPending {
			return Shipment{}, false, ErrOperationInProgress
		}
		return reserved, true, nil
	}
	result, err := adapter.Create(ctx, credential, request)
	if err != nil {
		_ = s.repository.FailCreate(ctx, tenantID, reserved.ID, providerErrorStatus(err))
		return Shipment{}, false, err
	}
	completed, err := s.repository.CompleteCreate(ctx, tenantID, reserved.ID, result)
	return completed, false, err
}

func (s *Service) Get(ctx context.Context, shipmentID string) (Shipment, error) {
	tenantID, err := tenant(ctx)
	if err != nil {
		return Shipment{}, err
	}
	shipmentID, err = normalizeShipmentID(shipmentID)
	if err != nil {
		return Shipment{}, err
	}
	return s.repository.Get(ctx, tenantID, shipmentID)
}

func (s *Service) Pickup(
	ctx context.Context,
	shipmentID, idempotencyKey string,
	request PickupRequest,
) (Shipment, bool, error) {
	tenantID, err := tenant(ctx)
	if err != nil {
		return Shipment{}, false, err
	}
	idempotencyKey, err = normalizeIdempotencyKey(idempotencyKey)
	if err != nil {
		return Shipment{}, false, err
	}
	request.Vehicle = strings.ToLower(strings.TrimSpace(request.Vehicle))
	if request.ScheduledAt.IsZero() || !oneOf(request.Vehicle, "motor", "mobil", "truk") {
		return Shipment{}, false, ErrInvalidRequest
	}
	shipmentID, err = normalizeShipmentID(shipmentID)
	if err != nil {
		return Shipment{}, false, err
	}
	shipment, err := s.repository.Get(ctx, tenantID, shipmentID)
	if err != nil {
		return Shipment{}, false, err
	}
	if finalStatus(shipment.Status) {
		return Shipment{}, false, ErrShipmentFinal
	}
	adapter, credential, err := s.adapterCredential(ctx, shipment.ProviderCode, "pickup:write")
	if err != nil {
		return Shipment{}, false, err
	}
	_, requestHash, err := requestPayload(request)
	if err != nil {
		return Shipment{}, false, err
	}
	operationID, err := newUUID()
	if err != nil {
		return Shipment{}, false, err
	}
	created, err := s.repository.ReserveOperation(ctx, ReserveOperationInput{
		ID: operationID, TenantID: tenantID, ShipmentID: shipment.ID,
		OperationType: "pickup", IdempotencyKey: idempotencyKey,
		RequestHash: requestHash,
	})
	if err != nil {
		return Shipment{}, false, err
	}
	if !created {
		current, getErr := s.repository.Get(ctx, tenantID, shipment.ID)
		if getErr != nil {
			return Shipment{}, false, getErr
		}
		if current.Status == StatusPickupRequested || current.AWB != "" {
			return current, true, nil
		}
		return Shipment{}, false, ErrOperationInProgress
	}
	result, err := adapter.Pickup(ctx, credential, shipment, request)
	if err != nil {
		_ = s.repository.FailOperation(ctx, tenantID, shipment.ID, "pickup", idempotencyKey, providerErrorStatus(err))
		return Shipment{}, false, err
	}
	completed, err := s.repository.CompletePickup(ctx, tenantID, shipment.ID, idempotencyKey, result)
	return completed, false, err
}

func (s *Service) Cancel(
	ctx context.Context,
	shipmentID, idempotencyKey string,
	request CancelRequest,
) (Shipment, bool, error) {
	tenantID, err := tenant(ctx)
	if err != nil {
		return Shipment{}, false, err
	}
	idempotencyKey, err = normalizeIdempotencyKey(idempotencyKey)
	if err != nil {
		return Shipment{}, false, err
	}
	request.ReasonCode = strings.ToLower(strings.TrimSpace(request.ReasonCode))
	request.Reason = strings.TrimSpace(request.Reason)
	if request.ReasonCode == "" || len(request.ReasonCode) > 64 || len(request.Reason) > 500 {
		return Shipment{}, false, ErrInvalidRequest
	}
	shipmentID, err = normalizeShipmentID(shipmentID)
	if err != nil {
		return Shipment{}, false, err
	}
	shipment, err := s.repository.Get(ctx, tenantID, shipmentID)
	if err != nil {
		return Shipment{}, false, err
	}
	if finalStatus(shipment.Status) {
		if shipment.Status == StatusCancelled {
			return shipment, true, nil
		}
		return Shipment{}, false, ErrShipmentFinal
	}
	adapter, credential, err := s.adapterCredential(ctx, shipment.ProviderCode, "shipments:cancel")
	if err != nil {
		return Shipment{}, false, err
	}
	_, requestHash, err := requestPayload(request)
	if err != nil {
		return Shipment{}, false, err
	}
	operationID, err := newUUID()
	if err != nil {
		return Shipment{}, false, err
	}
	created, err := s.repository.ReserveOperation(ctx, ReserveOperationInput{
		ID: operationID, TenantID: tenantID, ShipmentID: shipment.ID,
		OperationType: "cancel", IdempotencyKey: idempotencyKey,
		RequestHash: requestHash,
	})
	if err != nil {
		return Shipment{}, false, err
	}
	if !created {
		current, getErr := s.repository.Get(ctx, tenantID, shipment.ID)
		if getErr != nil {
			return Shipment{}, false, getErr
		}
		if current.Status == StatusCancelled || current.Status == StatusCancellationPending {
			return current, true, nil
		}
		return Shipment{}, false, ErrOperationInProgress
	}
	result, err := adapter.Cancel(ctx, credential, shipment, request)
	if err != nil {
		_ = s.repository.FailOperation(ctx, tenantID, shipment.ID, "cancel", idempotencyKey, providerErrorStatus(err))
		return Shipment{}, false, err
	}
	completed, err := s.repository.CompleteCancel(ctx, tenantID, shipment.ID, idempotencyKey, result)
	return completed, false, err
}

func (s *Service) Label(ctx context.Context, shipmentID, format string) (Label, error) {
	tenantID, err := tenant(ctx)
	if err != nil {
		return Label{}, err
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "page_5"
	}
	if !oneOf(format, "page_1", "page_2", "page_4", "page_5", "page_6") {
		return Label{}, ErrInvalidRequest
	}
	shipmentID, err = normalizeShipmentID(shipmentID)
	if err != nil {
		return Label{}, err
	}
	shipment, err := s.repository.Get(ctx, tenantID, shipmentID)
	if err != nil {
		return Label{}, err
	}
	if cached, cacheErr := s.repository.GetLabel(ctx, tenantID, shipment.ID, format); cacheErr == nil {
		return cached, nil
	} else if !errors.Is(cacheErr, ErrLabelUnavailable) {
		return Label{}, cacheErr
	}
	adapter, credential, err := s.adapterCredential(ctx, shipment.ProviderCode, "labels:read")
	if err != nil {
		return Label{}, err
	}
	label, err := adapter.Label(ctx, credential, shipment, format)
	if err != nil {
		return Label{}, err
	}
	if err := s.repository.SaveLabel(ctx, tenantID, shipment.ID, label); err != nil {
		return Label{}, err
	}
	return label, nil
}

func (s *Service) History(ctx context.Context, shipmentID string) ([]HistoryEvent, error) {
	tenantID, err := tenant(ctx)
	if err != nil {
		return nil, err
	}
	shipmentID, err = normalizeShipmentID(shipmentID)
	if err != nil {
		return nil, err
	}
	return s.repository.History(ctx, tenantID, shipmentID)
}

func (s *Service) ReconcileNow(ctx context.Context, shipmentID string) error {
	shipmentID, err := normalizeShipmentID(shipmentID)
	if err != nil {
		return err
	}
	repository, ok := s.repository.(LifecycleRepository)
	if !ok {
		return ErrProviderUnsupported
	}
	return repository.EnqueueReconciliation(ctx, shipmentID)
}

func (s *Service) resolveProvider(
	ctx context.Context,
	tenantID, requestedProvider, capability string,
) (string, Adapter, string, error) {
	activeProvider, err := s.catalog.ActiveProviderCode(ctx, tenantID)
	if err != nil {
		return "", nil, "", err
	}
	if activeProvider == "" {
		return "", nil, "", ErrShippingDisabled
	}
	requestedProvider = strings.ToLower(strings.TrimSpace(requestedProvider))
	if requestedProvider != "" && requestedProvider != activeProvider &&
		!(activeProvider == merchantproviders.EmisellProviderCode && requestedProvider == "rajaongkir") {
		return "", nil, "", ErrProviderUnsupported
	}
	effectiveProvider := activeProvider
	if effectiveProvider == merchantproviders.EmisellProviderCode {
		effectiveProvider = "rajaongkir"
	}
	adapter, credential, err := s.adapterCredential(ctx, effectiveProvider, capability)
	return effectiveProvider, adapter, credential, err
}

func (s *Service) adapterCredential(
	ctx context.Context,
	providerCode, capability string,
) (Adapter, string, error) {
	adapter, ok := s.adapters[providerCode]
	if !ok {
		return nil, "", ErrProviderUnsupported
	}
	credential, _, _, err := s.credentials.ResolveProviderCredentialForCapability(
		ctx, providerCode, capability,
	)
	if errors.Is(err, providercredentials.ErrCredentialCapabilityUnavailable) ||
		errors.Is(err, providercredentials.ErrNoActiveCredential) {
		return nil, "", ErrCredentialUnavailable
	}
	if err != nil {
		return nil, "", err
	}
	return adapter, credential, nil
}

func normalizeCreateRequest(request CreateRequest) CreateRequest {
	request.ProviderCode = strings.ToLower(strings.TrimSpace(request.ProviderCode))
	request.MerchantReference = strings.TrimSpace(request.MerchantReference)
	request.QuoteID = strings.TrimSpace(request.QuoteID)
	request.BrandName = strings.TrimSpace(request.BrandName)
	if request.BrandName == "" {
		request.BrandName = "Emisell"
	}
	request.CourierCode = couriers.NormalizeCode(request.CourierCode)
	request.ServiceCode = strings.TrimSpace(request.ServiceCode)
	request.DeliveryMode = strings.ToLower(strings.TrimSpace(request.DeliveryMode))
	request.Fulfillment = strings.ToLower(strings.TrimSpace(request.Fulfillment))
	request.Sender = normalizeAddress(request.Sender)
	request.Recipient = normalizeAddress(request.Recipient)
	request.Package.Contents = strings.TrimSpace(request.Package.Contents)
	request.Payment.Type = strings.ToLower(strings.TrimSpace(request.Payment.Type))
	request.Payment.FundingSource = strings.ToLower(strings.TrimSpace(request.Payment.FundingSource))
	request.Notes = strings.TrimSpace(request.Notes)
	for index := range request.Package.Items {
		request.Package.Items[index].Name = strings.TrimSpace(request.Package.Items[index].Name)
		request.Package.Items[index].SKU = strings.TrimSpace(request.Package.Items[index].SKU)
		request.Package.Items[index].Variant = strings.TrimSpace(request.Package.Items[index].Variant)
	}
	return request
}

func normalizeAddress(address Address) Address {
	address.Name = strings.TrimSpace(address.Name)
	address.Phone = strings.TrimSpace(address.Phone)
	address.Email = strings.TrimSpace(address.Email)
	address.Address = strings.TrimSpace(address.Address)
	address.AddressNote = strings.TrimSpace(address.AddressNote)
	address.OfficialCode = strings.TrimSpace(address.OfficialCode)
	address.PostalCode = strings.TrimSpace(address.PostalCode)
	return address
}

func validateCreateRequest(request CreateRequest) error {
	if request.MerchantReference == "" || len(request.MerchantReference) > 128 ||
		request.QuoteID == "" || len(request.QuoteID) > 128 ||
		request.CourierCode == "" || len(request.CourierCode) > 32 ||
		request.ServiceCode == "" || len(request.ServiceCode) > 64 ||
		!oneOf(request.DeliveryMode, "regular", "next_day", "economy", "cargo") ||
		!oneOf(request.Fulfillment, "pickup", "dropoff") {
		return ErrInvalidRequest
	}
	if !validAddress(request.Sender) || !validAddress(request.Recipient) {
		return ErrInvalidRequest
	}
	if request.Package.WeightGrams < 1 || request.Package.WeightGrams > 1_000_000 ||
		request.Package.LengthCM < 1 || request.Package.WidthCM < 1 || request.Package.HeightCM < 1 ||
		request.Package.ItemValue < 0 || request.Package.Contents == "" ||
		len(request.Package.Items) == 0 || len(request.Package.Items) > 100 {
		return ErrInvalidRequest
	}
	for _, item := range request.Package.Items {
		if item.Name == "" || item.Quantity < 1 || item.UnitValue < 0 || item.WeightGrams < 1 {
			return ErrInvalidRequest
		}
	}
	if !oneOf(request.Payment.Type, "non_cod", "cod") ||
		request.Payment.ShippingCost < 0 || request.Payment.GrandTotal < 0 ||
		(request.Payment.Type == "cod" && request.Payment.CODValue != request.Payment.GrandTotal) {
		return ErrInvalidRequest
	}
	return nil
}

func validAddress(address Address) bool {
	return address.Name != "" && len(address.Name) <= 128 &&
		address.Phone != "" && len(address.Phone) <= 32 &&
		address.Address != "" && len(address.Address) <= 1000 &&
		address.DestinationID > 0
}

func tenant(ctx context.Context) (string, error) {
	identity, ok := tenancy.FromContext(ctx)
	if !ok || strings.TrimSpace(identity.TenantID) == "" {
		return "", ErrInvalidRequest
	}
	return identity.TenantID, nil
}

func normalizeIdempotencyKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !idempotencyPattern.MatchString(value) {
		return "", ErrInvalidIdempotency
	}
	return value, nil
}

func normalizeShipmentID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !shipmentIDPattern.MatchString(value) {
		return "", ErrInvalidRequest
	}
	return strings.ToLower(value), nil
}

func requestPayload(value any) ([]byte, []byte, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal fulfillment request: %w", err)
	}
	hash := sha256.Sum256(payload)
	return payload, hash[:], nil
}

func newUUID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate fulfillment ID: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func finalStatus(status string) bool {
	return status == StatusDelivered || status == StatusCancelled
}

func providerErrorStatus(err error) string {
	switch {
	case errors.Is(err, ErrProviderUnauthorized):
		return "unauthorized"
	case errors.Is(err, ErrProviderRejected):
		return "rejected"
	case errors.Is(err, ErrProviderTimeout):
		return "timeout"
	default:
		return "unavailable"
	}
}
