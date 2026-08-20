package biteship

import (
	"context"
	"net/http"
	"net/url"
)

type TrackingHistory struct {
	Note        string `json:"note"`
	ServiceType string `json:"service_type"`
	UpdatedAt   string `json:"updated_at"`
	Status      string `json:"status"`
}

type TrackingParty struct {
	ContactName string `json:"contact_name"`
	Address     string `json:"address"`
}

type PublicTracking struct {
	Success   bool   `json:"success"`
	ID        string `json:"id"`
	WaybillID string `json:"waybill_id"`
	Courier   struct {
		Company string `json:"company"`
		Name    string `json:"name"`
	} `json:"courier"`
	Origin      TrackingParty     `json:"origin"`
	Destination TrackingParty     `json:"destination"`
	History     []TrackingHistory `json:"history"`
	Link        string            `json:"link"`
	Status      string            `json:"status"`
}

func (c *Client) TrackPublic(
	ctx context.Context,
	waybill, courierCode string,
) (PublicTracking, error) {
	var result PublicTracking
	path := "v1/trackings/" + url.PathEscape(waybill) +
		"/couriers/" + url.PathEscape(courierCode)
	err := c.doJSON(ctx, http.MethodGet, path, &result)
	return result, err
}
