package rajaongkir

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/emisell/api-kurir/internal/fulfillment"
)

type FulfillmentAdapter struct {
	baseURL string
	client  *http.Client
	now     func() time.Time
}

func NewFulfillmentAdapter(baseURL string, timeout time.Duration) *FulfillmentAdapter {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	return &FulfillmentAdapter{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		client:  &http.Client{Timeout: timeout},
		now:     time.Now,
	}
}

func (a *FulfillmentAdapter) Code() string { return "rajaongkir" }

func (a *FulfillmentAdapter) Create(
	ctx context.Context,
	credential string,
	request fulfillment.CreateRequest,
) (fulfillment.ProviderCreateResult, error) {
	items := make([]map[string]any, 0, len(request.Package.Items))
	for _, item := range request.Package.Items {
		length, width, height := item.LengthCM, item.WidthCM, item.HeightCM
		if length == 0 {
			length = request.Package.LengthCM
		}
		if width == 0 {
			width = request.Package.WidthCM
		}
		if height == 0 {
			height = request.Package.HeightCM
		}
		items = append(items, map[string]any{
			"product_name": item.Name, "product_variant_name": item.Variant,
			"product_price": item.UnitValue, "product_weight": item.WeightGrams,
			"product_width": width, "product_height": height,
			"product_length": length, "qty": item.Quantity,
			"subtotal": item.UnitValue * int64(item.Quantity),
		})
	}
	paymentMethod := "BANK TRANSFER"
	if request.Payment.Type == "cod" {
		paymentMethod = "COD"
	}
	payload := map[string]any{
		"order_date": requestDate(a.now()), "brand_name": request.BrandName,
		"shipper_name": request.Sender.Name, "shipper_phone": request.Sender.Phone,
		"shipper_email":          request.Sender.Email,
		"shipper_destination_id": request.Sender.DestinationID,
		"shipper_address":        joinAddress(request.Sender.Address, request.Sender.AddressNote),
		"receiver_name":          request.Recipient.Name, "receiver_phone": request.Recipient.Phone,
		"receiver_email":          request.Recipient.Email,
		"receiver_destination_id": request.Recipient.DestinationID,
		"receiver_address":        joinAddress(request.Recipient.Address, request.Recipient.AddressNote),
		"shipping":                strings.ToUpper(request.CourierCode),
		"shipping_type":           request.ServiceCode, "payment_method": paymentMethod,
		"shipping_cost":     request.Payment.ShippingCost,
		"shipping_cashback": request.Payment.ShippingCashback,
		"service_fee":       request.Payment.ServiceFee,
		"additional_cost":   request.Payment.AdditionalCost,
		"grand_total":       request.Payment.GrandTotal,
		"cod_value":         request.Payment.CODValue,
		"insurance_value":   request.Payment.InsuranceValue,
		"notes":             request.Notes, "order_details": items,
	}
	if point := pinPoint(request.Sender.Latitude, request.Sender.Longitude); point != "" {
		payload["origin_pin_point"] = point
	}
	if point := pinPoint(request.Recipient.Latitude, request.Recipient.Longitude); point != "" {
		payload["destination_pin_point"] = point
	}
	var response struct {
		Meta providerMeta `json:"meta"`
		Data struct {
			OrderID json.Number `json:"order_id"`
			OrderNo string      `json:"order_no"`
		} `json:"data"`
	}
	if err := a.doJSON(ctx, http.MethodPost, "/order/api/v1/orders/store", credential, payload, &response); err != nil {
		return fulfillment.ProviderCreateResult{}, err
	}
	if response.Data.OrderNo == "" {
		return fulfillment.ProviderCreateResult{}, fulfillment.ErrProviderRejected
	}
	return fulfillment.ProviderCreateResult{
		ProviderShipmentID: response.Data.OrderNo,
		ProviderStatus:     response.Meta.Message,
		Status:             fulfillment.StatusBooked,
	}, nil
}

func (a *FulfillmentAdapter) Pickup(
	ctx context.Context,
	credential string,
	shipment fulfillment.Shipment,
	request fulfillment.PickupRequest,
) (fulfillment.ProviderPickupResult, error) {
	payload := map[string]any{
		"pickup_date":    request.ScheduledAt.Format("2006-01-02"),
		"pickup_time":    request.ScheduledAt.Format("15:04:05"),
		"pickup_vehicle": titleVehicle(request.Vehicle),
		"orders":         []map[string]string{{"order_no": shipment.ProviderShipmentID}},
	}
	var response struct {
		Meta providerMeta `json:"meta"`
		Data []struct {
			Status  string `json:"status"`
			OrderNo string `json:"order_no"`
			AWB     string `json:"awb"`
		} `json:"data"`
	}
	if err := a.doJSON(ctx, http.MethodPost, "/order/api/v1/pickup/request", credential, payload, &response); err != nil {
		return fulfillment.ProviderPickupResult{}, err
	}
	if len(response.Data) == 0 || !strings.EqualFold(response.Data[0].Status, "success") {
		return fulfillment.ProviderPickupResult{}, fulfillment.ErrProviderRejected
	}
	return fulfillment.ProviderPickupResult{
		ProviderOperationID: response.Data[0].OrderNo,
		AWB:                 response.Data[0].AWB,
		ProviderStatus:      response.Data[0].Status,
		Status:              fulfillment.StatusPickupRequested,
	}, nil
}

func (a *FulfillmentAdapter) Label(
	ctx context.Context,
	credential string,
	shipment fulfillment.Shipment,
	format string,
) (fulfillment.Label, error) {
	path := "/order/api/v1/orders/print-label?page=" + url.QueryEscape(format) +
		"&order_no=" + url.QueryEscape(shipment.ProviderShipmentID)
	var response struct {
		Meta providerMeta `json:"meta"`
		Data struct {
			Path   string `json:"path"`
			Base64 string `json:"base_64"`
		} `json:"data"`
	}
	if err := a.doJSON(ctx, http.MethodPost, path, credential, nil, &response); err != nil {
		return fulfillment.Label{}, err
	}
	if response.Data.Base64 == "" && response.Data.Path == "" {
		return fulfillment.Label{}, fulfillment.ErrLabelUnavailable
	}
	return fulfillment.Label{
		Format: format, ContentType: "application/pdf",
		URL: response.Data.Path, Base64: response.Data.Base64,
	}, nil
}

func (a *FulfillmentAdapter) Cancel(
	ctx context.Context,
	credential string,
	shipment fulfillment.Shipment,
	_ fulfillment.CancelRequest,
) (fulfillment.ProviderCancelResult, error) {
	var response struct {
		Meta providerMeta `json:"meta"`
	}
	if err := a.doJSON(ctx, http.MethodPut, "/order/api/v1/orders/cancel", credential,
		map[string]string{"order_no": shipment.ProviderShipmentID}, &response); err != nil {
		return fulfillment.ProviderCancelResult{}, err
	}
	return fulfillment.ProviderCancelResult{
		ProviderStatus: response.Meta.Message,
		Status:         fulfillment.StatusCancelled,
	}, nil
}

func (a *FulfillmentAdapter) Detail(
	ctx context.Context,
	credential string,
	shipment fulfillment.Shipment,
) (fulfillment.ProviderDetailResult, error) {
	path := "/order/api/v1/orders/detail?order_no=" +
		url.QueryEscape(shipment.ProviderShipmentID)
	var response struct {
		Meta providerMeta `json:"meta"`
		Data struct {
			OrderNo         string `json:"order_no"`
			AWB             string `json:"awb"`
			OrderStatus     string `json:"order_status"`
			LiveTrackingURL string `json:"live_tracking_url"`
		} `json:"data"`
	}
	if err := a.doJSON(ctx, http.MethodGet, path, credential, nil, &response); err != nil {
		return fulfillment.ProviderDetailResult{}, err
	}
	if response.Data.OrderNo == "" {
		return fulfillment.ProviderDetailResult{}, fulfillment.ErrProviderRejected
	}
	return fulfillment.ProviderDetailResult{
		AWB:             strings.TrimSpace(response.Data.AWB),
		ProviderStatus:  strings.TrimSpace(response.Data.OrderStatus),
		Status:          normalizeDeliveryStatus(response.Data.OrderStatus, shipment.Status),
		LiveTrackingURL: strings.TrimSpace(response.Data.LiveTrackingURL),
	}, nil
}

type providerMeta struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
	Status  any    `json:"status"`
}

func (a *FulfillmentAdapter) doJSON(
	ctx context.Context,
	method, path, credential string,
	payload any,
	target any,
) error {
	if a.baseURL == "" || strings.TrimSpace(credential) == "" {
		return fulfillment.ErrCredentialUnavailable
	}
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, a.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("create RajaOngkir fulfillment request: %w", err)
	}
	request.Header.Set("x-api-key", credential)
	request.Header.Set("Accept", "application/json")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := a.client.Do(request)
	if err != nil {
		var netErr net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
			return fulfillment.ErrProviderTimeout
		}
		return fulfillment.ErrProviderUnavailable
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, 4<<20)
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		_, _ = io.Copy(io.Discard, limited)
		return fulfillment.ErrProviderUnauthorized
	}
	if response.StatusCode == http.StatusBadRequest || response.StatusCode == http.StatusUnprocessableEntity || response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusConflict {
		_, _ = io.Copy(io.Discard, limited)
		return fulfillment.ErrProviderRejected
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, limited)
		return fulfillment.ErrProviderUnavailable
	}
	decoder := json.NewDecoder(limited)
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return fulfillment.ErrProviderUnavailable
	}
	return nil
}

func pinPoint(latitude, longitude *float64) string {
	if latitude == nil || longitude == nil {
		return ""
	}
	return fmt.Sprintf("%.7f, %.7f", *latitude, *longitude)
}

func joinAddress(address, note string) string {
	if strings.TrimSpace(note) == "" {
		return address
	}
	return address + " (" + note + ")"
}

func requestDate(now time.Time) string {
	return now.In(time.FixedZone("WIB", 7*60*60)).Format("2006-01-02")
}

func titleVehicle(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "mobil":
		return "Mobil"
	case "truk":
		return "Truk"
	default:
		return "Motor"
	}
}

func normalizeDeliveryStatus(providerStatus, current string) string {
	status := strings.ToLower(strings.TrimSpace(providerStatus))
	switch {
	case strings.Contains(status, "cancel"), strings.Contains(status, "batal"):
		return fulfillment.StatusCancelled
	case strings.Contains(status, "deliver"), strings.Contains(status, "selesai"),
		strings.Contains(status, "terkirim"), strings.Contains(status, "diterima"):
		return fulfillment.StatusDelivered
	case strings.Contains(status, "out for delivery"), strings.Contains(status, "diantar"):
		return fulfillment.StatusOutForDelivery
	case strings.Contains(status, "transit"), strings.Contains(status, "dikirim"),
		strings.Contains(status, "manifest"):
		return fulfillment.StatusInTransit
	case strings.Contains(status, "picked"), strings.Contains(status, "terjemput"),
		strings.Contains(status, "sudah pickup"):
		return fulfillment.StatusPickedUp
	case strings.Contains(status, "pickup"), strings.Contains(status, "penjemput"):
		return fulfillment.StatusPickupRequested
	case strings.Contains(status, "aju"), strings.Contains(status, "book"),
		strings.Contains(status, "baru"), strings.Contains(status, "new"):
		return fulfillment.StatusBooked
	default:
		return current
	}
}
