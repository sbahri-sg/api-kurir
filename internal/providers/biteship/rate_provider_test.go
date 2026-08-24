package biteship

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/locations"
	"github.com/emisell/api-kurir/internal/rates"
	"github.com/emisell/api-kurir/internal/tenancy"
)

type rateLocationStoreStub struct {
	mu        sync.Mutex
	mappings  map[string]string
	locations map[string]locations.Location
}

func (s *rateLocationStoreStub) ResolveProviderLocation(
	_ context.Context,
	publicID string,
	providerCode string,
) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if providerCode != "biteship" {
		return "", locations.ErrProviderMappingNotFound
	}
	value := s.mappings[publicID]
	if value == "" {
		return "", locations.ErrProviderMappingNotFound
	}
	return value, nil
}

func (s *rateLocationStoreStub) FindByPublicID(
	_ context.Context,
	publicID string,
) (locations.Location, error) {
	location, found := s.locations[publicID]
	if !found {
		return locations.Location{}, locations.ErrLocationNotFound
	}
	return location, nil
}

func (s *rateLocationStoreStub) SaveProviderMapping(
	_ context.Context,
	publicID, providerCode, providerLocationID, _, _ string,
) error {
	if providerCode != "biteship" {
		return errors.New("unexpected provider")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mappings[publicID] = providerLocationID
	return nil
}

func TestDynamicRateProviderMapsLocationsAndNormalizesSupportedServices(t *testing.T) {
	t.Parallel()

	var rateRequest CourierRateRequest
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "biteship_test.valid" {
			t.Fatal("missing Biteship authorization token")
		}
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/maps/areas":
			input := request.URL.Query().Get("input")
			if input == "" || request.URL.Query().Get("countries") != "ID" {
				t.Fatalf("unexpected map query: %s", request.URL.RawQuery)
			}
			if input == "13910 Cakung Jakarta Timur DKI Jakarta" {
				_, _ = writer.Write([]byte(`{"success":true,"areas":[{"id":"IDNP6IDNC148IDND1423","name":"Cakung, Jakarta Timur, DKI Jakarta 13910","country_code":"ID","administrative_division_level_1_name":"DKI Jakarta","administrative_division_level_2_name":"Kota Jakarta Timur","administrative_division_level_3_name":"Cakung","postal_code":13910}]}`))
				return
			}
			_, _ = writer.Write([]byte(`{"success":true,"areas":[{"id":"IDNP5IDNC181IDND1610","name":"Sleman, Sleman, DI Yogyakarta 55511","country_code":"ID","administrative_division_level_1_name":"DI Yogyakarta","administrative_division_level_2_name":"Kabupaten Sleman","administrative_division_level_3_name":"Sleman","postal_code":55511}]}`))
		case "/v1/rates/couriers":
			if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("unexpected rate request: %s %s", request.Method, request.Header.Get("Content-Type"))
			}
			if err := json.NewDecoder(request.Body).Decode(&rateRequest); err != nil {
				t.Fatal(err)
			}
			_, _ = writer.Write([]byte(`{
                  "success":true,
                  "pricing":[
                    {"courier_code":"jne","courier_name":"JNE","courier_service_code":"reg","courier_service_name":"JNE Regular","price":18000,"shipment_duration_range":"1 - 2","shipment_duration_unit":"days"},
                    {"courier_code":"jne","courier_name":"JNE","courier_service_code":"yes","courier_service_name":"JNE YES","price":30000,"shipment_duration_range":"1","shipment_duration_unit":"day"},
                    {"courier_code":"jne","courier_name":"JNE","courier_service_code":"oke","courier_service_name":"JNE OKE","price":15000,"shipment_duration_range":"2 - 3","shipment_duration_unit":"days"},
                    {"courier_code":"jne","courier_name":"JNE","courier_service_code":"jtr","courier_service_name":"JNE Trucking","price":45000,"shipment_duration_range":"3 - 5","shipment_duration_unit":"days"},
                    {"courier_code":"jne","courier_name":"JNE","courier_service_code":"ss","courier_service_name":"Super Speed","price":90000}
                  ]
                }`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	resolverTenant := "not-called"
	quotaTenant := "not-called"
	quota := &quotaStub{seenTenant: &quotaTenant}
	store := &rateLocationStoreStub{
		mappings: make(map[string]string),
		locations: map[string]locations.Location{
			"origin": {
				PublicID: "origin", Level: "district", Province: "DKI Jakarta",
				City: "Jakarta Timur", District: "Cakung", PostalCode: "13910",
			},
			"destination": {
				PublicID: "destination", Level: "district", Province: "Daerah Istimewa Yogyakarta",
				City: "Kabupaten Sleman", District: "Sleman", PostalCode: "55511",
			},
		},
	}
	provider := NewDynamicRateProvider(
		credentialResolverStub{
			secret: "biteship_test.valid", alias: "biteship-test", limit: 100,
			seenTenant: &resolverTenant,
		},
		server.URL,
		time.Second,
		store,
		quota,
		nil,
		14*24*time.Hour,
	)
	ctx := tenancy.WithIdentity(context.Background(), tenancy.Identity{TenantID: "merchant_123"})
	quotes, err := provider.Quote(ctx, rates.Request{
		TenantID: "merchant_123", Origin: "origin", Destination: "destination",
		ActualWeightGrams: 1_200, ItemValue: 100_000, Couriers: []string{"jne"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(quotes) != 4 {
		t.Fatalf("supported quotes=%d want=4: %#v", len(quotes), quotes)
	}
	groups := make([]string, 0, len(quotes))
	for _, quote := range quotes {
		groups = append(groups, quote.ServiceGroup)
		if quote.ProviderCode != "biteship" || quote.ClassificationSource != "official_public" {
			t.Fatalf("unexpected normalized quote: %#v", quote)
		}
	}
	sort.Strings(groups)
	wantGroups := []string{"cargo", "economy", "next_day", "regular"}
	for index := range wantGroups {
		if groups[index] != wantGroups[index] {
			t.Fatalf("groups=%v want=%v", groups, wantGroups)
		}
	}
	if rateRequest.OriginAreaID == "" || rateRequest.DestinationAreaID == "" ||
		rateRequest.Couriers != "jne" || len(rateRequest.Items) != 1 ||
		rateRequest.Items[0].Weight != 1_200 {
		t.Fatalf("unexpected Biteship rate payload: %#v", rateRequest)
	}
	if resolverTenant != "" || quotaTenant != "" {
		t.Fatalf("platform fallback leaked tenant context: resolver=%q quota=%q", resolverTenant, quotaTenant)
	}
	if quota.calls != 3 {
		t.Fatalf("quota calls=%d want=3 (two maps and one rate)", quota.calls)
	}
}

func TestSelectBiteshipAreaRequiresExactAdministrativeMatch(t *testing.T) {
	t.Parallel()
	location := locations.Location{
		Province: "Jawa Timur", City: "Kabupaten Malang", District: "Singosari",
		PostalCodes: []string{"65153"},
	}
	area, err := selectBiteshipArea(location, []Area{
		{ID: "wrong-city", CountryCode: "ID", ProvinceName: "Jawa Timur", CityName: "Kota Malang", DistrictName: "Singosari", PostalCode: 65153},
		{ID: "correct", CountryCode: "ID", ProvinceName: "Jawa Timur", CityName: "Kabupaten Malang", DistrictName: "Singosari", PostalCode: 65153},
	})
	if err != nil || area.ID != "correct" {
		t.Fatalf("area=%#v err=%v", area, err)
	}
}

func TestAdministrativeNameNormalizesIndonesianSpecialRegions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		local    string
		provider string
	}{
		{local: "Daerah Khusus Ibukota Jakarta", provider: "DKI Jakarta"},
		{local: "Daerah Istimewa Yogyakarta", provider: "DI Yogyakarta"},
		{local: "Kota Administrasi Jakarta Timur", provider: "Jakarta Timur"},
		{local: "Kabupaten Sleman", provider: "Sleman"},
	}
	for _, test := range tests {
		if !sameAdministrativeName(test.local, test.provider) {
			t.Fatalf("expected %q to match %q", test.local, test.provider)
		}
	}
}
