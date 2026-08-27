package rajaongkir

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/rates"
	"github.com/emisell/api-kurir/internal/tracking"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestCalculateDomestic(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Timeout: time.Second,
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.Path != "/api/v1/calculate/domestic-cost" {
				t.Fatalf("path: got %s", request.URL.Path)
			}
			if request.Header.Get("key") != "test-secret" {
				t.Fatal("API key header was not sent")
			}
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			form, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatal(err)
			}
			if form.Get("origin") != "100" ||
				form.Get("destination") != "200" ||
				form.Get("weight") != "10300" ||
				form.Get("courier") != "jne:tiki" ||
				form.Get("price") != "lowest" {
				t.Fatalf("unexpected form: %v", form)
			}
			return jsonResponse(http.StatusOK, `{
			"meta":{"message":"success","code":200,"status":"success"},
			"data":[{
				"name":"Jalur Nugraha Ekakurir (JNE)",
				"code":"jne",
				"service":"JTR",
				"description":"JNE Trucking",
				"cost":44000,
				"etd":"3-7 day"
			}]
			}`), nil
		}),
	}

	client, err := NewClient("https://provider.test/api/v1/", "test-secret", httpClient)
	if err != nil {
		t.Fatal(err)
	}
	quotes, err := client.CalculateDomestic(context.Background(), DomesticCostRequest{
		Origin:      "100",
		Destination: "200",
		WeightGrams: 10300,
		Couriers:    []string{"jne", "tiki"},
		PriceFilter: "lowest",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(quotes) != 1 || quotes[0].Cost != 44000 || quotes[0].ServiceCode != "JTR" {
		t.Fatalf("unexpected quote: %#v", quotes)
	}
	if quotes[0].ETDMinDays == nil || *quotes[0].ETDMinDays != 3 ||
		quotes[0].ETDMaxDays == nil || *quotes[0].ETDMaxDays != 7 {
		t.Fatalf("unexpected ETD: %#v", quotes[0])
	}
}

func TestCalculateDomesticNormalizesSubKilogramWeight(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Timeout: time.Second,
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			form, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatal(err)
			}
			if form.Get("weight") != "1000" {
				t.Fatalf("provider weight: got %s want 1000", form.Get("weight"))
			}
			return jsonResponse(http.StatusOK, `{
				"meta":{"message":"success","code":200,"status":"success"},
				"data":[]
			}`), nil
		}),
	}

	client, err := NewClient(
		"https://provider.test/api/v1/",
		"test-secret",
		httpClient,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CalculateDomestic(context.Background(), DomesticCostRequest{
		Origin:      "100",
		Destination: "200",
		WeightGrams: 500,
		Couriers:    []string{"jne"},
	})
	if !errors.Is(err, rates.ErrRateNotAvailable) {
		t.Fatalf("error: got %v want ErrRateNotAvailable", err)
	}
}

func TestCalculateDistrictDomesticUsesDistrictEndpoint(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Timeout: time.Second,
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.Path != "/api/v1/calculate/district/domestic-cost" {
				t.Fatalf("path: got %s", request.URL.Path)
			}
			return jsonResponse(http.StatusOK, `{
				"meta":{"message":"success","code":200,"status":"success"},
				"data":[{
					"name":"JNE",
					"code":"jne",
					"service":"REG",
					"description":"Layanan Reguler",
					"cost":15000,
					"etd":"1-2"
				}]
			}`), nil
		}),
	}
	client, err := NewClient(
		"https://provider.test/api/v1/",
		"test-secret",
		httpClient,
	)
	if err != nil {
		t.Fatal(err)
	}
	quotes, err := client.CalculateDistrictDomestic(
		context.Background(),
		DomesticCostRequest{
			Origin:      "1391",
			Destination: "1376",
			WeightGrams: 1000,
			Couriers:    []string{"jne"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(quotes) != 1 || quotes[0].Cost != 15000 {
		t.Fatalf("unexpected quote: %#v", quotes)
	}
}

func TestSearchDestinationsAcceptsNumericIDs(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Timeout: time.Second,
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.Query().Get("search") != "Bandung" {
				t.Fatalf("unexpected query: %s", request.URL.RawQuery)
			}
			return jsonResponse(http.StatusOK, `{
			"meta":{"message":"success","code":200,"status":"success"},
			"data":[{
				"id":123,
				"label":"Coblong, Bandung",
				"province_name":"Jawa Barat",
				"city_name":"Bandung",
				"district_name":"Coblong",
				"subdistrict_name":"",
				"zip_code":40132
			}]
			}`), nil
		}),
	}

	client, err := NewClient("https://provider.test/", "test-secret", httpClient)
	if err != nil {
		t.Fatal(err)
	}
	destinations, err := client.SearchDestinations(context.Background(), "Bandung", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(destinations) != 1 || destinations[0].ID != "123" || destinations[0].ZipCode != "40132" {
		t.Fatalf("unexpected destination: %#v", destinations)
	}
}

func TestListHierarchyLocations(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Timeout: time.Second,
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.Path != "/api/v1/destination/sub-district/5823" {
				t.Fatalf("path: got %s", request.URL.Path)
			}
			if request.Header.Get("key") != "test-secret" {
				t.Fatal("API key header was not sent")
			}
			return jsonResponse(http.StatusOK, `{
				"meta":{"message":"success","code":200,"status":"success"},
				"data":[
					{"id":68513,"name":"BALERAKSA","zip_code":"53355"},
					{"id":"68514","name":"KARANGSARI","zip_code":null}
				]
			}`), nil
		}),
	}

	client, err := NewClient(
		"https://provider.test/api/v1/",
		"test-secret",
		httpClient,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.ListSubdistricts(context.Background(), "5823")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("unexpected hierarchy result: %#v", result)
	}
	if result[0].ID != "68513" || result[0].ZipCode != "53355" {
		t.Fatalf("unexpected first hierarchy location: %#v", result[0])
	}
	if result[1].ZipCode != "" {
		t.Fatalf("expected null postal code to be empty: %#v", result[1])
	}
}

func TestUnauthorizedDoesNotExposeKey(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Timeout: time.Second,
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return jsonResponse(
				http.StatusUnauthorized,
				`{"meta":{"message":"invalid key","code":401}}`,
			), nil
		}),
	}

	const secret = "must-not-appear"
	client, err := NewClient("https://provider.test/", secret, httpClient)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CalculateDomestic(context.Background(), DomesticCostRequest{
		Origin: "1", Destination: "2", WeightGrams: 1000, Couriers: []string{"jne"},
	})
	if err == nil || !strings.Contains(err.Error(), rates.ErrProviderUnauthorized.Error()) {
		t.Fatalf("expected unauthorized error, got %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("error exposed the API key")
	}
}

func TestCalculateDomesticRejectsQuoteWithoutCourierCode(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Timeout: time.Second,
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{
				"meta":{"message":"success","code":200,"status":"success"},
				"data":[{"service":"REG","cost":10000}]
			}`), nil
		}),
	}

	client, err := NewClient("https://provider.test/", "test-secret", httpClient)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CalculateDomestic(context.Background(), DomesticCostRequest{
		Origin: "1", Destination: "2", WeightGrams: 1000, Couriers: []string{"jne"},
	})
	if !errors.Is(err, rates.ErrProviderUnavailable) {
		t.Fatalf("expected invalid provider response, got %v", err)
	}
}

func TestTrackWaybillDoesNotRequirePhoneContext(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Timeout: time.Second,
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.Path != "/api/v1/track/waybill" {
				t.Fatalf("path: got %s", request.URL.Path)
			}
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			form, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatal(err)
			}
			if form.Get("awb") != "TEST123456789" ||
				form.Get("courier") != "jne" ||
				form.Has("last_phone_number") {
				t.Fatalf("unexpected tracking form: %v", form)
			}
			return jsonResponse(http.StatusOK, `{
				"meta":{"message":"success","code":200,"status":"success"},
				"data":{
					"delivered":false,
					"summary":{
						"courier_code":"jne",
						"courier_name":"JNE",
						"waybill_number":"TEST123456789",
						"service_code":"REG",
						"waybill_date":"2026-07-27",
						"shipper_name":"TOKO EMISELL",
						"receiver_name":"BUDI",
						"origin":"Jakarta",
						"destination":"Bandung",
						"status":"IN TRANSIT"
					},
					"details":{
						"waybill_number":"TEST123456789",
						"waybill_date":"2026-07-27",
						"waybill_time":"10:00",
						"weight":1,
						"origin":"Jakarta",
						"destination":"Bandung",
						"shipper_name":"TOKO EMISELL",
						"shipper_address1":"Jalan Asal 1",
						"shipper_address2":"",
						"shipper_address3":"",
						"shipper_city":"Jakarta",
						"receiver_name":"BUDI",
						"receiver_address1":"Jalan Tujuan 1",
						"receiver_address2":"",
						"receiver_address3":"",
						"receiver_city":"Bandung"
					},
					"delivery_status":{
						"status":"",
						"pod_receiver":"BUDI",
						"pod_date":"",
						"pod_time":""
					},
					"manifest":[{
						"manifest_code":"TRANSIT",
						"manifest_description":"In transit at gateway",
						"manifest_date":"2026-07-28",
						"manifest_time":"08:15",
						"city_name":"Jakarta"
					}]
				}
			}`), nil
		}),
	}

	client, err := NewClient("https://provider.test/api/v1/", "test-secret", httpClient)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.TrackWaybill(context.Background(), WaybillRequest{
		AWB:     "TEST123456789",
		Courier: "jne",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.Status != "IN TRANSIT" ||
		result.Summary.WaybillNumber != "TEST123456789" ||
		result.Summary.ShipperName != "TOKO EMISELL" ||
		len(result.Manifest) != 1 ||
		result.Details.Weight != "1" ||
		result.Details.ReceiverAddress1 != "Jalan Tujuan 1" ||
		result.Delivery.PODReceiver != "BUDI" {
		t.Fatalf("unexpected tracking result: %#v", result)
	}
}

func TestTrackWaybillMapsHTTP429ToTransientRateLimit(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Timeout: time.Second,
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return jsonResponse(
				http.StatusTooManyRequests,
				`{"meta":{"message":"Too many requests","code":429},"data":null}`,
			), nil
		}),
	}
	client, err := NewClient("https://provider.test/", "test-secret", httpClient)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.TrackWaybill(context.Background(), WaybillRequest{
		AWB: "TEST123456789", Courier: "tiki",
	})
	if !errors.Is(err, tracking.ErrProviderRateLimited) {
		t.Fatalf("expected transient rate-limit error, got %v", err)
	}
	if errors.Is(err, tracking.ErrProviderQuota) {
		t.Fatalf("HTTP 429 must not be treated as daily quota exhaustion: %v", err)
	}
	if providerStatusCode(err) != http.StatusTooManyRequests {
		t.Fatalf("unexpected status: %d", providerStatusCode(err))
	}
}

func TestHierarchyMapsHTTP429ToTransientRateLimit(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Timeout: time.Second,
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return jsonResponse(
				http.StatusTooManyRequests,
				`{"meta":{"message":"Too many requests","code":429},"data":null}`,
			), nil
		}),
	}
	client, err := NewClient("https://provider.test/", "test-secret", httpClient)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.ListProvinces(context.Background())
	if !errors.Is(err, rates.ErrProviderRateLimited) {
		t.Fatalf("expected transient rate-limit error, got %v", err)
	}
	if errors.Is(err, rates.ErrProviderQuotaExhausted) {
		t.Fatalf("HTTP 429 must not mark the daily quota exhausted: %v", err)
	}
}
