package rajaongkir

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/emisell/api-kurir/internal/rates"
	"github.com/emisell/api-kurir/internal/tracking"
)

const maxResponseBytes = 2 * 1024 * 1024

var etdNumberPattern = regexp.MustCompile(`\d+`)

type Client struct {
	baseURL    *url.URL
	apiKey     string
	httpClient *http.Client
}

type pacedRoundTripper struct {
	base     http.RoundTripper
	interval time.Duration
	mu       sync.Mutex
	next     time.Time
}

func (t *pacedRoundTripper) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	t.mu.Lock()
	now := time.Now()
	startAt := now
	if t.next.After(now) {
		startAt = t.next
	}
	t.next = startAt.Add(t.interval)
	delay := startAt.Sub(now)
	t.mu.Unlock()

	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-request.Context().Done():
			return nil, request.Context().Err()
		case <-timer.C:
		}
	}
	return t.base.RoundTrip(request)
}

type DomesticCostRequest struct {
	Origin      string
	Destination string
	WeightGrams int64
	Couriers    []string
	PriceFilter string
}

type DomesticQuote struct {
	CourierName string
	CourierCode string
	ServiceCode string
	Description string
	Cost        int64
	ETDMinDays  *int
	ETDMaxDays  *int
}

type WaybillRequest struct {
	AWB             string
	Courier         string
	LastPhoneNumber string
}

type WaybillTracking struct {
	Delivered bool
	Summary   WaybillSummary
	Details   WaybillDetails
	Delivery  WaybillDelivery
	Manifest  []WaybillManifest
}

type WaybillSummary struct {
	CourierCode   string
	CourierName   string
	WaybillNumber string
	ServiceCode   string
	WaybillDate   string
	ShipperName   string
	ReceiverName  string
	Origin        string
	Destination   string
	Status        string
}

type WaybillDetails struct {
	WaybillNumber    string
	WaybillDate      string
	WaybillTime      string
	Weight           string
	Origin           string
	Destination      string
	ShipperName      string
	ShipperAddress1  string
	ShipperAddress2  string
	ShipperAddress3  string
	ShipperCity      string
	ReceiverName     string
	ReceiverAddress1 string
	ReceiverAddress2 string
	ReceiverAddress3 string
	ReceiverCity     string
}

type WaybillDelivery struct {
	Status      string
	PODReceiver string
	PODDate     string
	PODTime     string
}

type WaybillManifest struct {
	Code        string
	Description string
	Date        string
	Time        string
	City        string
}

type Destination struct {
	ID              string
	Label           string
	ProvinceName    string
	CityName        string
	DistrictName    string
	SubdistrictName string
	ZipCode         string
}

type HierarchyLocation struct {
	ID      string
	Name    string
	ZipCode string
}

type responseMeta struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

type calculateResponse struct {
	Meta responseMeta `json:"meta"`
	Data []struct {
		Name        string        `json:"name"`
		Code        string        `json:"code"`
		Service     string        `json:"service"`
		Description string        `json:"description"`
		Cost        flexibleInt64 `json:"cost"`
		ETD         string        `json:"etd"`
	} `json:"data"`
}

type destinationResponse struct {
	Meta responseMeta `json:"meta"`
	Data []struct {
		ID              flexibleString `json:"id"`
		Label           string         `json:"label"`
		ProvinceName    string         `json:"province_name"`
		CityName        string         `json:"city_name"`
		DistrictName    string         `json:"district_name"`
		SubdistrictName string         `json:"subdistrict_name"`
		ZipCode         flexibleString `json:"zip_code"`
	} `json:"data"`
}

type hierarchyResponse struct {
	Meta responseMeta `json:"meta"`
	Data []struct {
		ID      flexibleString `json:"id"`
		Name    string         `json:"name"`
		ZipCode flexibleString `json:"zip_code"`
	} `json:"data"`
}

type waybillResponse struct {
	Meta responseMeta `json:"meta"`
	Data *struct {
		Delivered bool `json:"delivered"`
		Summary   struct {
			CourierCode   string `json:"courier_code"`
			CourierName   string `json:"courier_name"`
			WaybillNumber string `json:"waybill_number"`
			ServiceCode   string `json:"service_code"`
			WaybillDate   string `json:"waybill_date"`
			ShipperName   string `json:"shipper_name"`
			ReceiverName  string `json:"receiver_name"`
			Origin        string `json:"origin"`
			Destination   string `json:"destination"`
			Status        string `json:"status"`
		} `json:"summary"`
		Details struct {
			WaybillNumber    string         `json:"waybill_number"`
			WaybillDate      string         `json:"waybill_date"`
			WaybillTime      string         `json:"waybill_time"`
			Weight           flexibleString `json:"weight"`
			Origin           string         `json:"origin"`
			Destination      string         `json:"destination"`
			ShipperName      string         `json:"shipper_name"`
			ShipperAddress1  string         `json:"shipper_address1"`
			ShipperAddress2  string         `json:"shipper_address2"`
			ShipperAddress3  string         `json:"shipper_address3"`
			ShipperCity      string         `json:"shipper_city"`
			ReceiverName     string         `json:"receiver_name"`
			ReceiverAddress1 string         `json:"receiver_address1"`
			ReceiverAddress2 string         `json:"receiver_address2"`
			ReceiverAddress3 string         `json:"receiver_address3"`
			ReceiverCity     string         `json:"receiver_city"`
		} `json:"details"`
		DeliveryStatus struct {
			Status      string `json:"status"`
			PODReceiver string `json:"pod_receiver"`
			PODDate     string `json:"pod_date"`
			PODTime     string `json:"pod_time"`
		} `json:"delivery_status"`
		Manifest []struct {
			Code        string `json:"manifest_code"`
			Description string `json:"manifest_description"`
			Date        string `json:"manifest_date"`
			Time        string `json:"manifest_time"`
			City        string `json:"city_name"`
		} `json:"manifest"`
	} `json:"data"`
}

type providerHTTPError struct {
	status int
	err    error
}

func (e *providerHTTPError) Error() string {
	return e.err.Error()
}

func (e *providerHTTPError) Unwrap() error {
	return e.err
}

type flexibleString string

func (s *flexibleString) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*s = ""
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err == nil {
		*s = flexibleString(value)
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return errors.New("value must be a string or number")
	}
	*s = flexibleString(number.String())
	return nil
}

type flexibleInt64 int64

func (i *flexibleInt64) UnmarshalJSON(data []byte) error {
	var number json.Number
	if err := json.Unmarshal(data, &number); err == nil {
		value, err := strconv.ParseInt(number.String(), 10, 64)
		if err == nil {
			*i = flexibleInt64(value)
			return nil
		}
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return errors.New("value must be an integer or numeric string")
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fmt.Errorf("parse integer: %w", err)
	}
	*i = flexibleInt64(parsed)
	return nil
}

func NewClient(baseURL, apiKey string, httpClient *http.Client) (*Client, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse RajaOngkir base URL: %w", err)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, errors.New("RajaOngkir base URL must be absolute")
	}
	if !strings.HasSuffix(parsedURL.Path, "/") {
		parsedURL.Path += "/"
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("RajaOngkir API key is required")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 4 * time.Second}
	}
	return &Client{
		baseURL:    parsedURL,
		apiKey:     strings.TrimSpace(apiKey),
		httpClient: httpClient,
	}, nil
}

func NewHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 4 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 50
	transport.MaxIdleConnsPerHost = 20
	transport.MaxConnsPerHost = 20
	transport.IdleConnTimeout = 90 * time.Second
	transport.ResponseHeaderTimeout = timeout
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
}

func NewPacedHTTPClient(
	timeout time.Duration,
	minimumRequestInterval time.Duration,
) *http.Client {
	client := NewHTTPClient(timeout)
	if minimumRequestInterval <= 0 {
		minimumRequestInterval = 250 * time.Millisecond
	}
	client.Transport = &pacedRoundTripper{
		base:     client.Transport,
		interval: minimumRequestInterval,
	}
	return client
}

func (c *Client) CalculateDomestic(
	ctx context.Context,
	input DomesticCostRequest,
) ([]DomesticQuote, error) {
	return c.calculateDomestic(
		ctx,
		"calculate/domestic-cost",
		input,
	)
}

func (c *Client) CalculateDistrictDomestic(
	ctx context.Context,
	input DomesticCostRequest,
) ([]DomesticQuote, error) {
	return c.calculateDomestic(
		ctx,
		"calculate/district/domestic-cost",
		input,
	)
}

func (c *Client) calculateDomestic(
	ctx context.Context,
	path string,
	input DomesticCostRequest,
) ([]DomesticQuote, error) {
	if input.Origin == "" || input.Destination == "" || input.WeightGrams <= 0 || len(input.Couriers) == 0 {
		return nil, errors.New("invalid RajaOngkir domestic cost request")
	}

	form := url.Values{}
	form.Set("origin", input.Origin)
	form.Set("destination", input.Destination)
	providerWeight := input.WeightGrams
	if providerWeight < rates.MinimumProviderBillableWeightGrams {
		providerWeight = rates.MinimumProviderBillableWeightGrams
	}
	form.Set("weight", strconv.FormatInt(providerWeight, 10))
	form.Set("courier", strings.Join(input.Couriers, ":"))
	if input.PriceFilter != "" {
		form.Set("price", input.PriceFilter)
	}

	request, err := c.newRequest(
		ctx,
		http.MethodPost,
		path,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var response calculateResponse
	if err := c.doJSON(request, &response); err != nil {
		return nil, err
	}
	if len(response.Data) == 0 {
		return nil, rates.ErrRateNotAvailable
	}

	quotes := make([]DomesticQuote, 0, len(response.Data))
	for _, item := range response.Data {
		courierCode := strings.ToLower(strings.TrimSpace(item.Code))
		if int64(item.Cost) < 0 ||
			strings.TrimSpace(item.Service) == "" ||
			courierCode == "" {
			return nil, fmt.Errorf("%w: invalid quote response", rates.ErrProviderUnavailable)
		}
		minDays, maxDays := parseETD(item.ETD)
		quotes = append(quotes, DomesticQuote{
			CourierName: strings.TrimSpace(item.Name),
			CourierCode: courierCode,
			ServiceCode: strings.TrimSpace(item.Service),
			Description: strings.TrimSpace(item.Description),
			Cost:        int64(item.Cost),
			ETDMinDays:  minDays,
			ETDMaxDays:  maxDays,
		})
	}
	return quotes, nil
}

func (c *Client) SearchDestinations(
	ctx context.Context,
	search string,
	limit int,
	offset int,
) ([]Destination, error) {
	search = strings.TrimSpace(search)
	if len(search) < 2 {
		return nil, errors.New("destination search must contain at least 2 characters")
	}
	if limit < 1 || limit > 1000 || offset < 0 {
		return nil, errors.New("invalid destination pagination")
	}

	endpoint := c.resolve("destination/domestic-destination")
	query := endpoint.Query()
	query.Set("search", search)
	query.Set("limit", strconv.Itoa(limit))
	query.Set("offset", strconv.Itoa(offset))
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create RajaOngkir destination request: %w", err)
	}
	c.setHeaders(request)

	var response destinationResponse
	if err := c.doJSON(request, &response); err != nil {
		return nil, err
	}

	destinations := make([]Destination, 0, len(response.Data))
	for _, item := range response.Data {
		if strings.TrimSpace(string(item.ID)) == "" {
			continue
		}
		destinations = append(destinations, Destination{
			ID:              string(item.ID),
			Label:           strings.TrimSpace(item.Label),
			ProvinceName:    strings.TrimSpace(item.ProvinceName),
			CityName:        strings.TrimSpace(item.CityName),
			DistrictName:    strings.TrimSpace(item.DistrictName),
			SubdistrictName: strings.TrimSpace(item.SubdistrictName),
			ZipCode:         strings.TrimSpace(string(item.ZipCode)),
		})
	}
	return destinations, nil
}

func (c *Client) ListProvinces(ctx context.Context) ([]HierarchyLocation, error) {
	return c.listHierarchy(ctx, "destination/province")
}

func (c *Client) ListCities(
	ctx context.Context,
	provinceID string,
) ([]HierarchyLocation, error) {
	return c.listHierarchyChild(ctx, "destination/city", provinceID)
}

func (c *Client) ListDistricts(
	ctx context.Context,
	cityID string,
) ([]HierarchyLocation, error) {
	return c.listHierarchyChild(ctx, "destination/district", cityID)
}

func (c *Client) ListSubdistricts(
	ctx context.Context,
	districtID string,
) ([]HierarchyLocation, error) {
	return c.listHierarchyChild(ctx, "destination/sub-district", districtID)
}

func (c *Client) listHierarchyChild(
	ctx context.Context,
	path string,
	parentID string,
) ([]HierarchyLocation, error) {
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return nil, errors.New("parent destination ID is required")
	}
	return c.listHierarchy(ctx, path+"/"+url.PathEscape(parentID))
}

func (c *Client) listHierarchy(
	ctx context.Context,
	path string,
) ([]HierarchyLocation, error) {
	endpoint := c.resolve(path)
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		endpoint.String(),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("create RajaOngkir hierarchy request: %w", err)
	}
	c.setHeaders(request)

	var response hierarchyResponse
	if err := c.doJSON(request, &response); err != nil {
		return nil, err
	}

	result := make([]HierarchyLocation, 0, len(response.Data))
	for _, item := range response.Data {
		id := strings.TrimSpace(string(item.ID))
		name := strings.TrimSpace(item.Name)
		if id == "" || name == "" {
			continue
		}
		result = append(result, HierarchyLocation{
			ID:      id,
			Name:    name,
			ZipCode: strings.TrimSpace(string(item.ZipCode)),
		})
	}
	return result, nil
}

func (c *Client) TrackWaybill(
	ctx context.Context,
	input WaybillRequest,
) (WaybillTracking, error) {
	input.AWB = strings.TrimSpace(input.AWB)
	input.Courier = strings.ToLower(strings.TrimSpace(input.Courier))
	input.LastPhoneNumber = strings.TrimSpace(input.LastPhoneNumber)
	if input.AWB == "" || input.Courier == "" {
		return WaybillTracking{}, tracking.ErrInvalidWaybill
	}

	form := url.Values{}
	form.Set("awb", input.AWB)
	form.Set("courier", input.Courier)
	if input.LastPhoneNumber != "" {
		form.Set("last_phone_number", input.LastPhoneNumber)
	}
	request, err := c.newRequest(
		ctx,
		http.MethodPost,
		"track/waybill",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return WaybillTracking{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var response waybillResponse
	if err := c.doTrackingJSON(request, &response); err != nil {
		return WaybillTracking{}, err
	}
	if response.Data == nil {
		return WaybillTracking{}, tracking.ErrWaybillNotFound
	}
	result := WaybillTracking{
		Delivered: response.Data.Delivered,
		Summary: WaybillSummary{
			CourierCode:   strings.ToLower(strings.TrimSpace(response.Data.Summary.CourierCode)),
			CourierName:   strings.TrimSpace(response.Data.Summary.CourierName),
			WaybillNumber: strings.TrimSpace(response.Data.Summary.WaybillNumber),
			ServiceCode:   strings.TrimSpace(response.Data.Summary.ServiceCode),
			WaybillDate:   strings.TrimSpace(response.Data.Summary.WaybillDate),
			ShipperName:   strings.TrimSpace(response.Data.Summary.ShipperName),
			ReceiverName:  strings.TrimSpace(response.Data.Summary.ReceiverName),
			Origin:        strings.TrimSpace(response.Data.Summary.Origin),
			Destination:   strings.TrimSpace(response.Data.Summary.Destination),
			Status:        strings.TrimSpace(response.Data.Summary.Status),
		},
		Details: WaybillDetails{
			WaybillNumber:    strings.TrimSpace(response.Data.Details.WaybillNumber),
			WaybillDate:      strings.TrimSpace(response.Data.Details.WaybillDate),
			WaybillTime:      strings.TrimSpace(response.Data.Details.WaybillTime),
			Weight:           strings.TrimSpace(string(response.Data.Details.Weight)),
			Origin:           strings.TrimSpace(response.Data.Details.Origin),
			Destination:      strings.TrimSpace(response.Data.Details.Destination),
			ShipperName:      strings.TrimSpace(response.Data.Details.ShipperName),
			ShipperAddress1:  strings.TrimSpace(response.Data.Details.ShipperAddress1),
			ShipperAddress2:  strings.TrimSpace(response.Data.Details.ShipperAddress2),
			ShipperAddress3:  strings.TrimSpace(response.Data.Details.ShipperAddress3),
			ShipperCity:      strings.TrimSpace(response.Data.Details.ShipperCity),
			ReceiverName:     strings.TrimSpace(response.Data.Details.ReceiverName),
			ReceiverAddress1: strings.TrimSpace(response.Data.Details.ReceiverAddress1),
			ReceiverAddress2: strings.TrimSpace(response.Data.Details.ReceiverAddress2),
			ReceiverAddress3: strings.TrimSpace(response.Data.Details.ReceiverAddress3),
			ReceiverCity:     strings.TrimSpace(response.Data.Details.ReceiverCity),
		},
		Delivery: WaybillDelivery{
			Status:      strings.TrimSpace(response.Data.DeliveryStatus.Status),
			PODReceiver: strings.TrimSpace(response.Data.DeliveryStatus.PODReceiver),
			PODDate:     strings.TrimSpace(response.Data.DeliveryStatus.PODDate),
			PODTime:     strings.TrimSpace(response.Data.DeliveryStatus.PODTime),
		},
		Manifest: make([]WaybillManifest, 0, len(response.Data.Manifest)),
	}
	for _, item := range response.Data.Manifest {
		result.Manifest = append(result.Manifest, WaybillManifest{
			Code:        strings.TrimSpace(item.Code),
			Description: strings.TrimSpace(item.Description),
			Date:        strings.TrimSpace(item.Date),
			Time:        strings.TrimSpace(item.Time),
			City:        strings.TrimSpace(item.City),
		})
	}
	return result, nil
}

func (c *Client) doTrackingJSON(request *http.Request, target any) error {
	response, err := c.httpClient.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return fmt.Errorf("%w: request timeout", tracking.ErrProviderTimeout)
		}
		return fmt.Errorf("%w: request failed", tracking.ErrProviderUnavailable)
	}
	defer response.Body.Close()

	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var providerError struct {
			Meta responseMeta `json:"meta"`
		}
		_ = decoder.Decode(&providerError)
		var mapped error
		switch response.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			mapped = tracking.ErrProviderUnauthorized
		case http.StatusTooManyRequests:
			// RajaOngkir also uses HTTP 429 for temporary throttling. Daily
			// quota is enforced separately by the local provider ledger, so a
			// 429 must stay retryable instead of being delayed until tomorrow.
			mapped = tracking.ErrProviderRateLimited
		case http.StatusBadRequest, http.StatusNotFound, http.StatusUnprocessableEntity:
			message := strings.ToLower(providerError.Meta.Message)
			if strings.Contains(message, "phone") ||
				strings.Contains(message, "last_phone_number") {
				mapped = tracking.ErrPhoneSuffixRequired
			} else {
				mapped = tracking.ErrWaybillNotFound
			}
		default:
			mapped = tracking.ErrProviderUnavailable
		}
		return &providerHTTPError{status: response.StatusCode, err: mapped}
	}
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: decode response", tracking.ErrProviderUnavailable)
	}
	return nil
}

func providerStatusCode(err error) int {
	var providerError *providerHTTPError
	if errors.As(err, &providerError) {
		return providerError.status
	}
	return 0
}

func (c *Client) newRequest(
	ctx context.Context,
	method string,
	path string,
	body io.Reader,
) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, method, c.resolve(path).String(), body)
	if err != nil {
		return nil, fmt.Errorf("create RajaOngkir request: %w", err)
	}
	c.setHeaders(request)
	return request, nil
}

func (c *Client) setHeaders(request *http.Request) {
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "emisell-api-kurir/0.5")
	request.Header.Set("key", c.apiKey)
}

func (c *Client) resolve(path string) *url.URL {
	return c.baseURL.ResolveReference(&url.URL{Path: path})
}

func (c *Client) doJSON(request *http.Request, target any) error {
	response, err := c.httpClient.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return fmt.Errorf("%w: %v", rates.ErrProviderUnavailable, err)
		}
		return fmt.Errorf("%w: request failed: %v", rates.ErrProviderUnavailable, err)
	}
	defer response.Body.Close()

	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var providerError struct {
			Meta responseMeta `json:"meta"`
		}
		_ = decoder.Decode(&providerError)
		message := strings.TrimSpace(providerError.Meta.Message)
		if message == "" {
			message = http.StatusText(response.StatusCode)
		}
		switch response.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return fmt.Errorf("%w: %s", rates.ErrProviderUnauthorized, message)
		case http.StatusTooManyRequests:
			return fmt.Errorf("%w: %s", rates.ErrProviderRateLimited, message)
		case http.StatusBadRequest, http.StatusNotFound, http.StatusUnprocessableEntity:
			return fmt.Errorf("%w: %s", rates.ErrRateNotAvailable, message)
		default:
			return fmt.Errorf("%w: HTTP %d: %s", rates.ErrProviderUnavailable, response.StatusCode, message)
		}
	}
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: decode response: %v", rates.ErrProviderUnavailable, err)
	}
	return nil
}

func parseETD(value string) (*int, *int) {
	matches := etdNumberPattern.FindAllString(value, 2)
	if len(matches) == 0 {
		return nil, nil
	}
	minDays, err := strconv.Atoi(matches[0])
	if err != nil {
		return nil, nil
	}
	maxDays := minDays
	if len(matches) > 1 {
		if parsed, err := strconv.Atoi(matches[1]); err == nil {
			maxDays = parsed
		}
	}
	if maxDays < minDays {
		minDays, maxDays = maxDays, minDays
	}
	return &minDays, &maxDays
}
