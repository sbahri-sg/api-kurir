package biteship

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	courierRatesEndpoint = "v1/rates/couriers"
	mapsAreasEndpoint    = "v1/maps/areas"
)

type RateItem struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
	Value       int64  `json:"value"`
	Quantity    int64  `json:"quantity"`
	Weight      int64  `json:"weight"`
	Height      int64  `json:"height,omitempty"`
	Length      int64  `json:"length,omitempty"`
	Width       int64  `json:"width,omitempty"`
}

type CourierRateRequest struct {
	OriginAreaID      string     `json:"origin_area_id"`
	DestinationAreaID string     `json:"destination_area_id"`
	Couriers          string     `json:"couriers"`
	Items             []RateItem `json:"items"`
}

type CourierPricing struct {
	Company               string `json:"company"`
	CourierName           string `json:"courier_name"`
	CourierCode           string `json:"courier_code"`
	CourierServiceName    string `json:"courier_service_name"`
	CourierServiceCode    string `json:"courier_service_code"`
	Description           string `json:"description"`
	Duration              string `json:"duration"`
	ShipmentDurationRange string `json:"shipment_duration_range"`
	ShipmentDurationUnit  string `json:"shipment_duration_unit"`
	ServiceType           string `json:"service_type"`
	ShippingType          string `json:"shipping_type"`
	Currency              string `json:"currency"`
	ShippingFee           int64  `json:"shipping_fee"`
	Price                 int64  `json:"price"`
}

type CourierRateResponse struct {
	Success bool             `json:"success"`
	Code    int              `json:"code"`
	Message string           `json:"message"`
	Pricing []CourierPricing `json:"pricing"`
}

type Area struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	CountryCode  string `json:"country_code"`
	ProvinceName string `json:"administrative_division_level_1_name"`
	CityName     string `json:"administrative_division_level_2_name"`
	DistrictName string `json:"administrative_division_level_3_name"`
	PostalCode   int64  `json:"postal_code"`
}

type AreaSearchResponse struct {
	Success bool   `json:"success"`
	Areas   []Area `json:"areas"`
}

func (c *Client) RetrieveCourierRates(
	ctx context.Context,
	input CourierRateRequest,
) (CourierRateResponse, error) {
	var result CourierRateResponse
	err := c.doJSONBody(
		ctx,
		http.MethodPost,
		courierRatesEndpoint,
		input,
		&result,
	)
	return result, err
}

func (c *Client) SearchAreas(
	ctx context.Context,
	input string,
) (AreaSearchResponse, error) {
	endpoint, err := url.JoinPath(c.baseURL, mapsAreasEndpoint)
	if err != nil {
		return AreaSearchResponse{}, err
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return AreaSearchResponse{}, err
	}
	query := parsed.Query()
	query.Set("countries", "ID")
	query.Set("input", strings.TrimSpace(input))
	query.Set("type", "single")
	parsed.RawQuery = query.Encode()

	var result AreaSearchResponse
	err = c.doJSONEndpoint(
		ctx,
		http.MethodGet,
		parsed.String(),
		nil,
		&result,
	)
	return result, err
}

func (a Area) PostalCodeText() string {
	if a.PostalCode <= 0 {
		return ""
	}
	return strconv.FormatInt(a.PostalCode, 10)
}
