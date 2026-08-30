package hosted

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/emisell/api-kurir/internal/fulfillment"
)

type FulfillmentAdapter struct {
	providerCode string
	client       *Client
}

func NewFulfillmentAdapter(providerCode string, client *Client) *FulfillmentAdapter {
	return &FulfillmentAdapter{providerCode: providerCode, client: client}
}

func (a *FulfillmentAdapter) Code() string { return a.providerCode }

func (a *FulfillmentAdapter) Quote(ctx context.Context, credential string, request fulfillment.QuoteRequest) ([]fulfillment.ProviderQuote, error) {
	var response struct {
		Data struct {
			Quotes []struct {
				ProviderQuoteID  string `json:"provider_quote_id"`
				CourierCode      string `json:"courier_code"`
				CourierName      string `json:"courier_name"`
				ServiceCode      string `json:"service_code"`
				ServiceName      string `json:"service_name"`
				ServiceGroup     string `json:"service_group"`
				DeliveryMode     string `json:"delivery_mode"`
				ShippingCost     int64  `json:"shipping_cost"`
				ShippingCashback int64  `json:"shipping_cashback"`
				ServiceFee       int64  `json:"service_fee"`
				AdditionalCost   int64  `json:"additional_cost"`
				GrandTotal       int64  `json:"grand_total"`
				CODValue         int64  `json:"cod_value"`
				InsuranceValue   int64  `json:"insurance_value"`
				Currency         string `json:"currency"`
				ETD              string `json:"etd"`
				ExpiresAt        string `json:"expires_at"`
			} `json:"quotes"`
		} `json:"data"`
	}
	if err := a.client.Do(ctx, http.MethodPost, "fulfillment/quotes", "x-api-key", credential, request, &response); err != nil {
		return nil, mapFulfillmentError(err)
	}
	quotes := make([]fulfillment.ProviderQuote, 0, len(response.Data.Quotes))
	for _, item := range response.Data.Quotes {
		expiresAt, _ := time.Parse(time.RFC3339, item.ExpiresAt)
		quotes = append(quotes, fulfillment.ProviderQuote{
			ProviderQuoteID: item.ProviderQuoteID, CourierCode: item.CourierCode,
			CourierName: item.CourierName, ServiceCode: item.ServiceCode,
			ServiceName: item.ServiceName, ServiceGroup: item.ServiceGroup,
			DeliveryMode: item.DeliveryMode, ShippingCost: item.ShippingCost,
			ShippingCashback: item.ShippingCashback, ServiceFee: item.ServiceFee,
			AdditionalCost: item.AdditionalCost, GrandTotal: item.GrandTotal,
			CODValue: item.CODValue, InsuranceValue: item.InsuranceValue,
			Currency: item.Currency, ETD: item.ETD, ExpiresAt: expiresAt,
		})
	}
	if len(quotes) == 0 {
		return nil, fulfillment.ErrProviderRejected
	}
	return quotes, nil
}

func (a *FulfillmentAdapter) Create(ctx context.Context, credential string, request fulfillment.CreateRequest) (fulfillment.ProviderCreateResult, error) {
	var response shipmentResponse
	if err := a.client.Do(ctx, http.MethodPost, "shipments", "x-api-key", credential, hostedCreateRequest(request), &response); err != nil {
		return fulfillment.ProviderCreateResult{}, mapFulfillmentError(err)
	}
	if response.Data.PartnerShipmentID == "" {
		return fulfillment.ProviderCreateResult{}, fulfillment.ErrProviderRejected
	}
	return fulfillment.ProviderCreateResult{
		ProviderShipmentID: response.Data.PartnerShipmentID,
		AWB:                response.Data.WaybillNumber,
		ProviderStatus:     response.Data.Status,
		Status:             fulfillment.StatusBooked,
	}, nil
}

func hostedCreateRequest(request fulfillment.CreateRequest) map[string]any {
	return map[string]any{
		"provider_code": request.ProviderCode, "merchant_reference": request.MerchantReference,
		"quote_id": request.QuoteID, "brand_name": request.BrandName,
		"courier_code": request.CourierCode, "service_code": request.ServiceCode,
		"delivery_mode": request.DeliveryMode, "fulfillment": request.Fulfillment,
		"sender": request.Sender, "recipient": request.Recipient, "package": request.Package,
		"payment": map[string]any{
			"type": request.Payment.Type, "items_subtotal": request.Payment.ItemsSubtotal,
			"order_discount": request.Payment.OrderDiscount, "tax_amount": request.Payment.TaxAmount,
			"shipping_cost":     request.Payment.ShippingCost,
			"shipping_discount": request.Payment.ShippingDiscount + request.Payment.ProviderShippingDiscount,
			"service_fee":       request.Payment.ServiceFee,
			"additional_cost":   request.Payment.AdditionalCost + request.Payment.ProviderAdditionalCost,
			"grand_total":       request.Payment.GrandTotal, "cod_value": request.Payment.CODValue,
			"insurance_value": request.Payment.InsuranceValue,
		},
		"notes": request.Notes,
	}
}

func (a *FulfillmentAdapter) Pickup(ctx context.Context, credential string, shipment fulfillment.Shipment, request fulfillment.PickupRequest) (fulfillment.ProviderPickupResult, error) {
	input := map[string]any{
		"provider_shipment_id": shipment.ProviderShipmentID,
		"mode":                 request.Mode,
		"scheduled_at":         request.ScheduledAt,
		"package_weight_grams": request.PackageWeightGrams,
	}
	var response struct {
		Data struct {
			PickupID          string `json:"pickup_id"`
			PartnerShipmentID string `json:"partner_shipment_id"`
			WaybillNumber     string `json:"waybill_number"`
			Status            string `json:"status"`
		} `json:"data"`
	}
	if err := a.client.Do(ctx, http.MethodPost, "pickups", "x-api-key", credential, input, &response); err != nil {
		return fulfillment.ProviderPickupResult{}, mapFulfillmentError(err)
	}
	return fulfillment.ProviderPickupResult{
		ProviderOperationID: response.Data.PickupID,
		AWB:                 response.Data.WaybillNumber,
		ProviderStatus:      response.Data.Status,
		Status:              fulfillment.StatusPickupRequested,
	}, nil
}

func (a *FulfillmentAdapter) Label(ctx context.Context, credential string, shipment fulfillment.Shipment, format string) (fulfillment.Label, error) {
	path := "shipments/" + url.PathEscape(shipment.ProviderShipmentID) + "/label?format=" + url.QueryEscape(format)
	var response struct {
		Data struct {
			Format      string `json:"format"`
			ContentType string `json:"content_type"`
			FileURL     string `json:"file_url"`
			Base64      string `json:"base64"`
		} `json:"data"`
	}
	if err := a.client.Do(ctx, http.MethodGet, path, "x-api-key", credential, nil, &response); err != nil {
		return fulfillment.Label{}, mapFulfillmentError(err)
	}
	if response.Data.FileURL == "" && response.Data.Base64 == "" {
		return fulfillment.Label{}, fulfillment.ErrLabelUnavailable
	}
	return fulfillment.Label{
		Format: response.Data.Format, ContentType: response.Data.ContentType,
		URL: response.Data.FileURL, Base64: response.Data.Base64,
	}, nil
}

func (a *FulfillmentAdapter) Cancel(ctx context.Context, credential string, shipment fulfillment.Shipment, _ fulfillment.CancelRequest) (fulfillment.ProviderCancelResult, error) {
	path := "shipments/" + url.PathEscape(shipment.ProviderShipmentID) + "/cancel"
	var response shipmentResponse
	if err := a.client.Do(ctx, http.MethodPost, path, "x-api-key", credential, map[string]any{}, &response); err != nil {
		return fulfillment.ProviderCancelResult{}, mapFulfillmentError(err)
	}
	return fulfillment.ProviderCancelResult{
		ProviderStatus: response.Data.Status,
		Status:         fulfillment.StatusCancelled,
	}, nil
}

func (a *FulfillmentAdapter) Detail(ctx context.Context, credential string, shipment fulfillment.Shipment) (fulfillment.ProviderDetailResult, error) {
	path := "shipments/" + url.PathEscape(shipment.ProviderShipmentID)
	var response shipmentResponse
	if err := a.client.Do(ctx, http.MethodGet, path, "x-api-key", credential, nil, &response); err != nil {
		return fulfillment.ProviderDetailResult{}, mapFulfillmentError(err)
	}
	return fulfillment.ProviderDetailResult{
		AWB: response.Data.WaybillNumber, ProviderStatus: response.Data.Status,
		Status:          normalizeFulfillmentStatus(response.Data.Status, shipment.Status),
		LiveTrackingURL: response.Data.LiveTrackingURL,
	}, nil
}

type shipmentResponse struct {
	Data struct {
		PartnerShipmentID  string `json:"partner_shipment_id"`
		ProviderShipmentID string `json:"provider_shipment_id"`
		WaybillNumber      string `json:"waybill_number"`
		Status             string `json:"status"`
		LiveTrackingURL    string `json:"live_tracking_url"`
	} `json:"data"`
}

func mapFulfillmentError(err error) error {
	var upstream *HTTPError
	if errors.As(err, &upstream) {
		switch upstream.Code {
		case "RAJAONGKIR_LABEL_AWB_PENDING", "LABEL_AWB_PENDING":
			return &fulfillment.LabelNotReadyError{Reason: "awb_pending", Retryable: true}
		case "RAJAONGKIR_LABEL_PICKUP_REQUIRED", "LABEL_PICKUP_REQUIRED":
			return &fulfillment.LabelNotReadyError{Reason: "pickup_required", Retryable: false}
		}
		switch upstream.Status {
		case http.StatusUnauthorized, http.StatusForbidden:
			return fulfillment.ErrProviderUnauthorized
		case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity:
			return fulfillment.ErrProviderRejected
		default:
			return fulfillment.ErrProviderUnavailable
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fulfillment.ErrProviderTimeout
	}
	return fulfillment.ErrProviderUnavailable
}

func normalizeFulfillmentStatus(providerStatus, current string) string {
	status := strings.ToLower(strings.TrimSpace(providerStatus))
	switch {
	case strings.Contains(status, "cancel"), strings.Contains(status, "batal"):
		return fulfillment.StatusCancelled
	case strings.Contains(status, "deliver"), strings.Contains(status, "terkirim"), strings.Contains(status, "diterima"):
		return fulfillment.StatusDelivered
	case strings.Contains(status, "transit"), strings.Contains(status, "dikirim"):
		return fulfillment.StatusInTransit
	case strings.Contains(status, "pickup"):
		return fulfillment.StatusPickupRequested
	case strings.Contains(status, "book"), strings.Contains(status, "new"), strings.Contains(status, "baru"):
		return fulfillment.StatusBooked
	default:
		return current
	}
}
