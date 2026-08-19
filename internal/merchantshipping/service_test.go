package merchantshipping

import (
	"context"
	"errors"
	"testing"

	"github.com/emisell/api-kurir/internal/couriers"
	"github.com/emisell/api-kurir/internal/rates"
)

type memoryRepository struct {
	preference Preference
	replaced   UpdateInput
}

func (repository *memoryRepository) Get(context.Context, string) (Preference, error) {
	return repository.preference, nil
}

func (repository *memoryRepository) Replace(
	_ context.Context,
	_ string,
	input UpdateInput,
) (Preference, error) {
	repository.replaced = input
	return Preference{
		Configured:    true,
		Mode:          input.Mode,
		EnabledGroups: input.EnabledGroups,
		Services:      input.Services,
		Version:       1,
	}, nil
}

type courierRepository struct {
	items []couriers.Courier
}

func (repository courierRepository) List(context.Context) ([]couriers.Courier, error) {
	return repository.items, nil
}

func TestUpdateCustomNormalizesAndValidatesCanonicalServices(t *testing.T) {
	t.Parallel()
	repository := &memoryRepository{}
	service := NewService(repository, courierRepository{items: testCouriers()})

	preference, err := service.Update(context.Background(), "merchant_123", UpdateInput{
		Mode: " CUSTOM ",
		Services: []Selection{
			{CourierCode: "JNE", ServiceCode: "reg"},
			{CourierCode: "jne", ServiceCode: "REG"},
			{CourierCode: "JNE", ServiceCode: "sps"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if preference.Mode != ModeCustom || len(preference.Services) != 2 {
		t.Fatalf("unexpected preference: %#v", preference)
	}
	if repository.replaced.Services[0] != (Selection{CourierCode: "jne", ServiceCode: "REG"}) ||
		repository.replaced.Services[1] != (Selection{CourierCode: "jne", ServiceCode: "SPS"}) {
		t.Fatalf("unexpected normalized services: %#v", repository.replaced.Services)
	}

	_, err = service.Update(context.Background(), "merchant_123", UpdateInput{
		Mode:     ModeCustom,
		Services: []Selection{{CourierCode: "jne", ServiceCode: "NOT_REAL"}},
	})
	if !errors.Is(err, ErrUnknownService) {
		t.Fatalf("error=%v want ErrUnknownService", err)
	}
}

func TestCatalogBuildsTriStateCourierSelection(t *testing.T) {
	t.Parallel()
	repository := &memoryRepository{preference: Preference{
		Configured: true,
		Mode:       ModeCustom,
		Services:   []Selection{{CourierCode: "jne", ServiceCode: "REG"}},
	}}
	service := NewService(repository, courierRepository{items: testCouriers()})

	catalog, err := service.Catalog(context.Background(), "merchant_123")
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Couriers) != 1 {
		t.Fatalf("unexpected courier count: %d", len(catalog.Couriers))
	}
	jne := catalog.Couriers[0]
	if jne.SelectionState != "partial" || jne.SelectedServiceCount != 1 ||
		jne.TotalServiceCount != 2 || !jne.Services[0].Selected || jne.Services[1].Selected {
		t.Fatalf("unexpected JNE catalog: %#v", jne)
	}
}

func TestFilterUsesCanonicalServiceAndGroup(t *testing.T) {
	t.Parallel()
	results := []rates.Result{
		{Card: rates.RateCard{
			CourierCode: "jne", ServiceCode: "REG23", CanonicalServiceCode: "REG", ServiceGroup: "regular",
		}},
		{Card: rates.RateCard{
			CourierCode: "jne", ServiceCode: "CTCSPS", CanonicalServiceCode: "SPS", ServiceGroup: "express",
		}},
	}

	customRepository := &memoryRepository{preference: Preference{
		Configured: true,
		Mode:       ModeCustom,
		Services:   []Selection{{CourierCode: "jne", ServiceCode: "REG"}},
	}}
	service := NewService(customRepository, courierRepository{})
	filtered, err := service.Filter(context.Background(), "merchant_123", results)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].Card.ServiceCode != "REG23" {
		t.Fatalf("unexpected custom filter result: %#v", filtered)
	}

	customRepository.preference = Preference{
		Configured:    true,
		Mode:          ModeGroups,
		EnabledGroups: []string{"express"},
	}
	filtered, err = service.Filter(context.Background(), "merchant_123", results)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].Card.ServiceCode != "CTCSPS" {
		t.Fatalf("unexpected group filter result: %#v", filtered)
	}
}

func TestUnconfiguredTenantKeepsBackwardCompatibleResults(t *testing.T) {
	t.Parallel()
	repository := &memoryRepository{preference: Preference{
		Configured: false,
		Mode:       ModeAll,
	}}
	service := NewService(repository, courierRepository{})
	results := []rates.Result{{Card: rates.RateCard{
		CourierCode: "new", ServiceCode: "RAW", ServiceGroup: "unknown",
	}}}
	filtered, err := service.Filter(context.Background(), "merchant_123", results)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 {
		t.Fatalf("unconfigured preference changed historical results: %#v", filtered)
	}
}

func testCouriers() []couriers.Courier {
	return []couriers.Courier{{
		Code: "jne",
		Name: "JNE",
		Services: []couriers.Service{
			{Code: "REG", Name: "JNE Regular", Group: "regular", ServiceType: "parcel"},
			{Code: "SPS", Name: "JNE Super Speed", Group: "express", ServiceType: "parcel"},
		},
	}}
}
