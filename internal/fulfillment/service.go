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
	"time"

	"github.com/emisell/api-kurir/internal/couriers"
	"github.com/emisell/api-kurir/internal/merchantproviders"
	"github.com/emisell/api-kurir/internal/providercredentials"
	"github.com/emisell/api-kurir/internal/tenancy"
)

var idempotencyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,128}$`)
var shipmentIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-4[0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

const maxFulfillmentAmount int64 = 1_000_000_000_000_000

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
	var lockedQuote *Quote
	if strings.HasPrefix(request.QuoteID, "fq_") {
		quote, quoteErr := s.repository.GetQuote(ctx, tenantID, request.QuoteID)
		if quoteErr != nil {
			return Shipment{}, false, quoteErr
		}
		if !quote.ExpiresAt.After(time.Now().UTC()) {
			return Shipment{}, false, ErrQuoteExpired
		}
		if quote.Environment != providercredentials.ExecutionEnvironment(ctx) {
			return Shipment{}, false, ErrQuoteMismatch
		}
		if err := applyLockedQuote(&request, quote); err != nil {
			return Shipment{}, false, err
		}
		lockedQuote = &quote
	}
	if err := validateCreateRequest(request); err != nil {
		return Shipment{}, false, err
	}
	providerCode, adapter, credential, credentialAlias, err := s.resolveProvider(
		ctx, tenantID, request.ProviderCode, "shipments:write",
	)
	if err != nil {
		return Shipment{}, false, err
	}
	if lockedQuote != nil && (lockedQuote.ProviderCode != providerCode ||
		lockedQuote.CredentialAlias != credentialAlias) {
		return Shipment{}, false, ErrQuoteMismatch
	}
	request.ProviderCode = providerCode
	publicServiceCode := request.ServiceCode
	if lockedQuote != nil {
		publicServiceCode = lockedQuote.ServiceCode
		request.ServiceCode = lockedQuote.NativeServiceCode
	}
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
		CourierCode: request.CourierCode, ServiceCode: publicServiceCode,
		DeliveryMode: request.DeliveryMode, Fulfillment: request.Fulfillment,
		ShippingCost: request.Payment.ShippingCost, Currency: "IDR",
		PackageWeightGrams: request.Package.WeightGrams,
		IdempotencyKey:     idempotencyKey, RequestHash: requestHash,
		RequestCiphertext: payload,
	})
	if err != nil {
		return Shipment{}, false, err
	}
	if !created {
		if reserved.ProviderShipmentID == "" && reserved.Status == StatusBookingPending {
			return Shipment{}, false, ErrOperationInProgress
		}
		if reserved.Status == StatusBookingFailed {
			return Shipment{}, false, providerError(reserved.ProviderStatus)
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

func (s *Service) Quotes(ctx context.Context, request QuoteRequest) ([]Quote, error) {
	tenantID, err := tenant(ctx)
	if err != nil {
		return nil, err
	}
	request = normalizeQuoteRequest(request)
	if err := validateQuoteRequest(request); err != nil {
		return nil, err
	}
	providerCode, adapter, credential, credentialAlias, err := s.resolveProvider(
		ctx, tenantID, request.ProviderCode, "shipments:write",
	)
	if err != nil {
		return nil, err
	}
	quoteAdapter, ok := adapter.(QuoteAdapter)
	if !ok {
		return nil, ErrProviderUnsupported
	}
	providerQuotes, err := quoteAdapter.Quote(ctx, credential, request)
	if err != nil {
		return nil, err
	}
	_, bindingHash, err := requestPayload(quoteBindingFromQuote(request))
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	environment := providercredentials.ExecutionEnvironment(ctx)
	quotes := make([]Quote, 0, len(providerQuotes))
	for _, providerQuote := range providerQuotes {
		providerQuote = normalizeProviderQuote(providerQuote, request.Package.ItemValue, request.PaymentType, now)
		if !validProviderQuote(providerQuote, now) {
			continue
		}
		id, idErr := newQuoteID()
		if idErr != nil {
			return nil, idErr
		}
		quotes = append(quotes, Quote{
			ID: id, ProviderCode: providerCode, Environment: environment,
			CourierCode: providerQuote.CourierCode, CourierName: providerQuote.CourierName,
			ServiceCode:  canonicalQuoteServiceCode(providerCode, providerQuote),
			ServiceName:  quoteServiceName(providerQuote.ServiceGroup),
			ServiceGroup: providerQuote.ServiceGroup, DeliveryMode: providerQuote.DeliveryMode,
			ShippingCost: providerQuote.ShippingCost, ShippingCashback: providerQuote.ShippingCashback,
			ServiceFee: providerQuote.ServiceFee, AdditionalCost: providerQuote.AdditionalCost,
			GrandTotal: providerQuote.GrandTotal, CODValue: providerQuote.CODValue,
			InsuranceValue: providerQuote.InsuranceValue, Currency: providerQuote.Currency,
			ETD: providerQuote.ETD, ExpiresAt: providerQuote.ExpiresAt,
			ProviderQuoteID:   providerQuote.ProviderQuoteID,
			NativeServiceCode: providerQuote.ServiceCode,
			CredentialAlias:   credentialAlias, BindingHash: append([]byte(nil), bindingHash...),
		})
	}
	if len(quotes) == 0 {
		return nil, ErrProviderRejected
	}
	if err := s.repository.SaveQuotes(ctx, tenantID, quotes); err != nil {
		return nil, err
	}
	return quotes, nil
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
	request.Mode = strings.ToLower(strings.TrimSpace(request.Mode))
	logicalRequest := request
	now := time.Now().UTC()
	switch request.Mode {
	case "now":
		if !request.ScheduledAt.IsZero() {
			return Shipment{}, false, ErrInvalidRequest
		}
		// Provider pickup APIs still require a concrete timestamp. Keep that
		// provider detail behind the gateway and give upstream a short lead time.
		request.ScheduledAt = now.Add(15 * time.Minute).Truncate(time.Minute)
	case "scheduled":
		if request.ScheduledAt.IsZero() || !request.ScheduledAt.After(now) {
			return Shipment{}, false, ErrInvalidRequest
		}
	default:
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
	if shipment.ProviderShipmentID == "" || shipment.Fulfillment != "pickup" ||
		!oneOf(shipment.Status, StatusBooked, StatusPickupRequested) {
		return Shipment{}, false, ErrPickupNotAllowed
	}
	if shipment.Status == StatusPickupRequested || shipment.AWB != "" {
		return shipment, true, nil
	}
	request.PackageWeightGrams = shipment.PackageWeightGrams
	adapter, credential, err := s.adapterCredential(ctx, shipment.ProviderCode, "pickup:write")
	if err != nil {
		return Shipment{}, false, err
	}
	_, requestHash, err := requestPayload(logicalRequest)
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
) (string, Adapter, string, string, error) {
	activeProvider, err := s.catalog.ActiveProviderCode(ctx, tenantID)
	if err != nil {
		return "", nil, "", "", err
	}
	if activeProvider == "" {
		return "", nil, "", "", ErrShippingDisabled
	}
	requestedProvider = strings.ToLower(strings.TrimSpace(requestedProvider))
	if requestedProvider != "" && requestedProvider != activeProvider &&
		!(activeProvider == merchantproviders.EmisellProviderCode && requestedProvider == "rajaongkir") {
		return "", nil, "", "", ErrProviderUnsupported
	}
	effectiveProvider := activeProvider
	if effectiveProvider == merchantproviders.EmisellProviderCode {
		effectiveProvider = "rajaongkir"
	}
	adapter, credential, alias, err := s.adapterCredentialWithAlias(ctx, effectiveProvider, capability)
	return effectiveProvider, adapter, credential, alias, err
}

func (s *Service) adapterCredential(
	ctx context.Context,
	providerCode, capability string,
) (Adapter, string, error) {
	adapter, credential, _, err := s.adapterCredentialWithAlias(ctx, providerCode, capability)
	return adapter, credential, err
}

func (s *Service) adapterCredentialWithAlias(
	ctx context.Context,
	providerCode, capability string,
) (Adapter, string, string, error) {
	adapter, ok := s.adapters[providerCode]
	if !ok {
		return nil, "", "", ErrProviderUnsupported
	}
	credential, alias, _, err := s.credentials.ResolveProviderCredentialForCapability(
		ctx, providerCode, capability,
	)
	if errors.Is(err, providercredentials.ErrCredentialCapabilityUnavailable) ||
		errors.Is(err, providercredentials.ErrNoActiveCredential) {
		return nil, "", "", ErrCredentialUnavailable
	}
	if err != nil {
		return nil, "", "", err
	}
	return adapter, credential, alias, nil
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
	request.Payment.Type = strings.ToLower(strings.TrimSpace(request.Payment.Type))
	request.Notes = strings.TrimSpace(request.Notes)
	for index := range request.Package.Items {
		request.Package.Items[index].Name = strings.TrimSpace(request.Package.Items[index].Name)
		request.Package.Items[index].Variant = strings.TrimSpace(request.Package.Items[index].Variant)
	}
	return request
}

func normalizeQuoteRequest(request QuoteRequest) QuoteRequest {
	request.ProviderCode = strings.ToLower(strings.TrimSpace(request.ProviderCode))
	request.PaymentType = strings.ToLower(strings.TrimSpace(request.PaymentType))
	if request.PaymentType == "" {
		request.PaymentType = "non_cod"
	}
	request.CourierCodes = normalizeStringList(request.CourierCodes, true)
	request.ServiceGroups = normalizeStringList(request.ServiceGroups, false)
	if len(request.ServiceGroups) == 0 {
		request.ServiceGroups = []string{"regular", "next_day", "economy", "cargo"}
	}
	return request
}

func normalizeStringList(values []string, courierCodes bool) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if courierCodes {
			value = couriers.NormalizeCode(value)
		} else {
			value = strings.ToLower(strings.TrimSpace(value))
		}
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func validateQuoteRequest(request QuoteRequest) error {
	if request.Origin.DestinationID < 1 || request.Destination.DestinationID < 1 ||
		request.Package.WeightGrams < 1 || request.Package.WeightGrams > 1_000_000 ||
		request.Package.LengthCM < 1 || request.Package.WidthCM < 1 || request.Package.HeightCM < 1 ||
		request.Package.ItemValue < 0 || !oneOf(request.PaymentType, "non_cod", "cod") ||
		len(request.CourierCodes) > 20 || len(request.ServiceGroups) > 4 {
		return ErrInvalidRequest
	}
	for _, group := range request.ServiceGroups {
		if !oneOf(group, "regular", "next_day", "economy", "cargo") {
			return ErrInvalidRequest
		}
	}
	return nil
}

type quoteBinding struct {
	OriginDestinationID      int64  `json:"origin_destination_id"`
	DestinationDestinationID int64  `json:"destination_destination_id"`
	WeightGrams              int64  `json:"weight_grams"`
	LengthCM                 int    `json:"length_cm"`
	WidthCM                  int    `json:"width_cm"`
	HeightCM                 int    `json:"height_cm"`
	ItemValue                int64  `json:"item_value"`
	PaymentType              string `json:"payment_type"`
}

func quoteBindingFromQuote(request QuoteRequest) quoteBinding {
	return quoteBinding{
		OriginDestinationID:      request.Origin.DestinationID,
		DestinationDestinationID: request.Destination.DestinationID,
		WeightGrams:              request.Package.WeightGrams, LengthCM: request.Package.LengthCM,
		WidthCM: request.Package.WidthCM, HeightCM: request.Package.HeightCM,
		ItemValue: request.Package.ItemValue, PaymentType: request.PaymentType,
	}
}

func quoteBindingFromCreate(request CreateRequest) quoteBinding {
	return quoteBinding{
		OriginDestinationID:      request.Sender.DestinationID,
		DestinationDestinationID: request.Recipient.DestinationID,
		WeightGrams:              request.Package.WeightGrams, LengthCM: request.Package.LengthCM,
		WidthCM: request.Package.WidthCM, HeightCM: request.Package.HeightCM,
		ItemValue: merchandiseValue(request.Payment), PaymentType: request.Payment.Type,
	}
}

func applyLockedQuote(request *CreateRequest, quote Quote) error {
	for _, amount := range []int64{
		request.Payment.ItemsSubtotal, request.Payment.OrderDiscount, request.Payment.TaxAmount,
		request.Payment.ShippingCost, request.Payment.ShippingDiscount,
		request.Payment.AdditionalCost, request.Payment.GrandTotal, request.Payment.CODValue,
		request.Payment.InsuranceValue,
	} {
		if amount < 0 || amount > maxFulfillmentAmount {
			return ErrInvalidRequest
		}
	}
	if request.Payment.OrderDiscount > request.Payment.ItemsSubtotal {
		return ErrAmountMismatch
	}
	_, bindingHash, err := requestPayload(quoteBindingFromCreate(*request))
	if err != nil {
		return err
	}
	if !equalBytes(bindingHash, quote.BindingHash) ||
		(request.ProviderCode != "" && request.ProviderCode != quote.ProviderCode) ||
		(request.CourierCode != "" && request.CourierCode != quote.CourierCode) ||
		(request.ServiceCode != "" && request.ServiceCode != quote.ServiceCode) ||
		(request.DeliveryMode != "" && request.DeliveryMode != quote.DeliveryMode) ||
		(request.Payment.ShippingCost != 0 && request.Payment.ShippingCost != quote.ShippingCost) ||
		(request.Payment.GrandTotal != 0 &&
			request.Payment.GrandTotal != quote.GrandTotal-request.Payment.ShippingDiscount+request.Payment.AdditionalCost) {
		return ErrQuoteMismatch
	}
	request.ProviderCode = quote.ProviderCode
	request.CourierCode = quote.CourierCode
	request.ServiceCode = quote.ServiceCode
	request.DeliveryMode = quote.DeliveryMode
	request.Payment.ShippingCost = quote.ShippingCost
	request.Payment.ProviderShippingDiscount = quote.ShippingCashback
	request.Payment.ServiceFee = quote.ServiceFee
	request.Payment.ProviderAdditionalCost = quote.AdditionalCost
	request.Payment.GrandTotal = quote.GrandTotal - request.Payment.ShippingDiscount + request.Payment.AdditionalCost
	if request.Payment.Type == "cod" {
		request.Payment.CODValue = request.Payment.GrandTotal
	} else {
		request.Payment.CODValue = 0
	}
	request.Payment.InsuranceValue = quote.InsuranceValue
	return nil
}

func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	var difference byte
	for index := range left {
		difference |= left[index] ^ right[index]
	}
	return difference == 0
}

func normalizeProviderQuote(quote ProviderQuote, itemValue int64, paymentType string, now time.Time) ProviderQuote {
	quote.CourierCode = couriers.NormalizeCode(quote.CourierCode)
	quote.CourierName = strings.TrimSpace(quote.CourierName)
	quote.ServiceCode = strings.TrimSpace(quote.ServiceCode)
	quote.ServiceGroup = strings.ToLower(strings.TrimSpace(quote.ServiceGroup))
	quote.DeliveryMode = strings.ToLower(strings.TrimSpace(quote.DeliveryMode))
	if quote.DeliveryMode == "" {
		quote.DeliveryMode = quote.ServiceGroup
	}
	if quote.Currency == "" {
		quote.Currency = "IDR"
	}
	if quote.ExpiresAt.IsZero() {
		quote.ExpiresAt = now.Add(15 * time.Minute)
	}
	if quote.GrandTotal == 0 {
		quote.GrandTotal = itemValue + quote.ShippingCost - quote.ShippingCashback + quote.ServiceFee + quote.AdditionalCost
	}
	if paymentType == "cod" && quote.CODValue == 0 {
		quote.CODValue = quote.GrandTotal
	}
	return quote
}

func validProviderQuote(quote ProviderQuote, now time.Time) bool {
	return quote.CourierCode != "" && quote.ServiceCode != "" &&
		oneOf(quote.ServiceGroup, "regular", "next_day", "economy", "cargo") &&
		oneOf(quote.DeliveryMode, "regular", "next_day", "economy", "cargo") &&
		quote.ShippingCost >= 0 && quote.GrandTotal >= 0 && quote.ExpiresAt.After(now)
}

func canonicalQuoteServiceCode(providerCode string, quote ProviderQuote) string {
	sum := sha256.Sum256([]byte(providerCode + "\x00" + quote.CourierCode + "\x00" + quote.ServiceCode))
	return fmt.Sprintf("svc_%x", sum[:8])
}

func quoteServiceName(group string) string {
	switch group {
	case "next_day":
		return "Next Day"
	case "economy":
		return "Economy"
	case "cargo":
		return "Cargo"
	default:
		return "Regular"
	}
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
		len(request.Package.Items) == 0 || len(request.Package.Items) > 100 {
		return ErrInvalidRequest
	}
	var itemsSubtotal int64
	var itemsWeight int64
	for _, item := range request.Package.Items {
		if item.Name == "" || item.Quantity < 1 || item.UnitPrice < 0 ||
			item.UnitPrice > maxFulfillmentAmount || item.Subtotal < 0 ||
			item.Subtotal > maxFulfillmentAmount || item.WeightGrams < 1 {
			return ErrInvalidRequest
		}
		quantity := int64(item.Quantity)
		if item.UnitPrice > 0 && quantity > (1<<63-1)/item.UnitPrice {
			return ErrInvalidRequest
		}
		if item.WeightGrams > (1<<63-1)/quantity {
			return ErrInvalidRequest
		}
		if item.Subtotal > item.UnitPrice*quantity || itemsSubtotal > (1<<63-1)-item.Subtotal {
			return ErrAmountMismatch
		}
		itemsSubtotal += item.Subtotal
		itemWeight := item.WeightGrams * quantity
		if itemsWeight > (1<<63-1)-itemWeight {
			return ErrInvalidRequest
		}
		itemsWeight += itemWeight
	}
	if !oneOf(request.Payment.Type, "non_cod", "cod") ||
		request.Payment.ItemsSubtotal < 0 || request.Payment.OrderDiscount < 0 ||
		request.Payment.TaxAmount < 0 || request.Payment.ShippingCost < 0 ||
		request.Payment.ShippingDiscount < 0 || request.Payment.ServiceFee < 0 ||
		request.Payment.AdditionalCost < 0 || request.Payment.ProviderShippingDiscount < 0 ||
		request.Payment.ProviderAdditionalCost < 0 || request.Payment.GrandTotal < 0 ||
		request.Payment.CODValue < 0 || request.Payment.InsuranceValue < 0 ||
		(request.Payment.Type == "cod" && request.Payment.CODValue != request.Payment.GrandTotal) {
		return ErrInvalidRequest
	}
	for _, amount := range []int64{
		request.Payment.ItemsSubtotal, request.Payment.OrderDiscount, request.Payment.TaxAmount,
		request.Payment.ShippingCost, request.Payment.ShippingDiscount, request.Payment.ServiceFee,
		request.Payment.AdditionalCost, request.Payment.ProviderShippingDiscount,
		request.Payment.ProviderAdditionalCost, request.Payment.GrandTotal, request.Payment.CODValue,
		request.Payment.InsuranceValue,
	} {
		if amount > maxFulfillmentAmount {
			return ErrInvalidRequest
		}
	}
	if itemsWeight > request.Package.WeightGrams ||
		itemsSubtotal != request.Payment.ItemsSubtotal ||
		request.Payment.OrderDiscount > request.Payment.ItemsSubtotal ||
		request.Payment.ShippingDiscount+request.Payment.ProviderShippingDiscount > request.Payment.ShippingCost ||
		request.Payment.GrandTotal != paymentGrandTotal(request.Payment) {
		return ErrAmountMismatch
	}
	return nil
}

func merchandiseValue(payment Payment) int64 {
	return payment.ItemsSubtotal - payment.OrderDiscount + payment.TaxAmount
}

func paymentGrandTotal(payment Payment) int64 {
	return merchandiseValue(payment) + payment.ShippingCost - payment.ShippingDiscount -
		payment.ProviderShippingDiscount + payment.ServiceFee + payment.AdditionalCost +
		payment.ProviderAdditionalCost
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

func newQuoteID() (string, error) {
	id, err := newUUID()
	if err != nil {
		return "", err
	}
	return "fq_" + strings.ReplaceAll(id, "-", ""), nil
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

func providerError(status string) error {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "unauthorized":
		return ErrProviderUnauthorized
	case "rejected":
		return ErrProviderRejected
	case "timeout":
		return ErrProviderTimeout
	default:
		return ErrProviderUnavailable
	}
}
