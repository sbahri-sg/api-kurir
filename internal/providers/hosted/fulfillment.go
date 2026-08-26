package hosted

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

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

func (a *FulfillmentAdapter) Create(ctx context.Context, credential string, request fulfillment.CreateRequest) (fulfillment.ProviderCreateResult, error) {
	var response shipmentResponse
	if err := a.client.Do(ctx, http.MethodPost, "shipments", "x-api-key", credential, request, &response); err != nil {
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

func (a *FulfillmentAdapter) Pickup(ctx context.Context, credential string, shipment fulfillment.Shipment, request fulfillment.PickupRequest) (fulfillment.ProviderPickupResult, error) {
	input := map[string]any{
		"provider_shipment_id": shipment.ProviderShipmentID,
		"scheduled_at":         request.ScheduledAt,
		"vehicle":              request.Vehicle,
	}
	var response struct {
		Data struct {
			PickupID string `json:"pickup_id"`
			Status   string `json:"status"`
		} `json:"data"`
	}
	if err := a.client.Do(ctx, http.MethodPost, "pickups", "x-api-key", credential, input, &response); err != nil {
		return fulfillment.ProviderPickupResult{}, mapFulfillmentError(err)
	}
	return fulfillment.ProviderPickupResult{
		ProviderOperationID: response.Data.PickupID,
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
