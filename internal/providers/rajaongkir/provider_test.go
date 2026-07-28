package rajaongkir

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/locations"
	"github.com/emisell/api-kurir/internal/rates"
)

type mappingResolverStub struct{}

func (mappingResolverStub) ResolveProviderLocation(
	_ context.Context,
	locationPublicID string,
	_ string,
) (string, error) {
	if locationPublicID == "loc_origin" {
		return "100", nil
	}
	return "200", nil
}

func (mappingResolverStub) FindByPublicID(
	context.Context,
	string,
) (locations.Location, error) {
	return locations.Location{}, nil
}

func (mappingResolverStub) SaveProviderMapping(
	context.Context,
	string,
	string,
	string,
	string,
	string,
) error {
	return nil
}

type providerQuotaStub struct{}

func (providerQuotaStub) ConsumeProviderHit(
	context.Context,
	string,
	string,
	int64,
) error {
	return nil
}

func TestProviderFiltersUnrequestedCourier(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Timeout: time.Second,
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{
				"meta":{"message":"success","code":200,"status":"success"},
				"data":[
					{"name":"JNE","code":"jne","service":"REG","cost":10000},
					{"name":"TIKI","code":"tiki","service":"REG","cost":9000}
				]
			}`), nil
		}),
	}
	client, err := NewClient("https://provider.test/", "test-secret", httpClient)
	if err != nil {
		t.Fatal(err)
	}
	provider := NewProvider(
		client,
		mappingResolverStub{},
		providerQuotaStub{},
		"test",
		100,
		time.Hour,
	)

	quotes, err := provider.Quote(context.Background(), rates.Request{
		Origin:            "loc_origin",
		Destination:       "loc_destination",
		ActualWeightGrams: 1000,
		Couriers:          []string{"jne"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(quotes) != 1 || quotes[0].CourierCode != "jne" {
		t.Fatalf("unexpected filtered quotes: %#v", quotes)
	}
}

func TestSelectExactDestinationNormalizesAdministrativePrefixes(t *testing.T) {
	t.Parallel()

	selected, err := selectExactDestination(
		locations.Location{
			Province:    "Aceh",
			City:        "Kabupaten Aceh Selatan",
			District:    "Bakongan",
			Subdistrict: "Keude Bakongan",
			PostalCode:  "23773",
		},
		[]Destination{
			{
				ID:              "destination-1",
				Label:           "Keude Bakongan, Bakongan, Aceh Selatan",
				ProvinceName:    "ACEH",
				CityName:        "Aceh Selatan",
				DistrictName:    "Bakongan",
				SubdistrictName: "Keude Bakongan",
				ZipCode:         "23773",
			},
			{
				ID:              "wrong-postal",
				ProvinceName:    "Aceh",
				CityName:        "Aceh Selatan",
				DistrictName:    "Bakongan",
				SubdistrictName: "Keude Bakongan",
				ZipCode:         "23774",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != "destination-1" {
		t.Fatalf("unexpected destination: %#v", selected)
	}
}

func TestSelectExactDestinationNormalizesJakartaProvinceAlias(t *testing.T) {
	t.Parallel()

	selected, err := selectExactDestination(
		locations.Location{
			Province:    "Daerah Khusus Ibukota Jakarta",
			City:        "Kota Administrasi Jakarta Timur",
			District:    "Cakung",
			Subdistrict: "Cakung Timur",
			PostalCode:  "13910",
		},
		[]Destination{{
			ID:              "destination-cakung-timur",
			Label:           "Cakung Timur, Cakung, Jakarta Timur",
			ProvinceName:    "DKI Jakarta",
			CityName:        "Jakarta Timur",
			DistrictName:    "Kecamatan Cakung",
			SubdistrictName: "Kelurahan Cakung Timur",
			ZipCode:         "13910",
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != "destination-cakung-timur" {
		t.Fatalf("unexpected destination: %#v", selected)
	}
}
