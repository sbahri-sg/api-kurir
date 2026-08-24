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

func (repository *memoryRepository) SelectedCourierCodes(
	context.Context,
	string,
) ([]string, error) {
	seen := make(map[string]struct{})
	codes := make([]string, 0)
	for _, selection := range repository.preference.Services {
		if _, exists := seen[selection.CourierCode]; exists {
			continue
		}
		seen[selection.CourierCode] = struct{}{}
		codes = append(codes, selection.CourierCode)
	}
	return codes, nil
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
			{CourierCode: "JNE", ServiceCode: "jtr"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if preference.Mode != ModeCustom || len(preference.Services) != 2 {
		t.Fatalf("unexpected preference: %#v", preference)
	}
	if repository.replaced.Services[0] != (Selection{CourierCode: "jne", ServiceCode: "JTR"}) ||
		repository.replaced.Services[1] != (Selection{CourierCode: "jne", ServiceCode: "REG"}) {
		t.Fatalf("unexpected normalized services: %#v", repository.replaced.Services)
	}

	_, err = service.Update(context.Background(), "merchant_123", UpdateInput{
		Mode:     ModeCustom,
		Services: []Selection{{CourierCode: "jne", ServiceCode: "NOT_REAL"}},
	})
	if !errors.Is(err, ErrUnknownService) {
		t.Fatalf("error=%v want ErrUnknownService", err)
	}

	_, err = service.Update(context.Background(), "merchant_123", UpdateInput{
		Mode:     ModeCustom,
		Services: []Selection{{CourierCode: "jne", ServiceCode: "SPS"}},
	})
	if !errors.Is(err, ErrUnknownService) {
		t.Fatalf("hidden group error=%v want ErrUnknownService", err)
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
		jne.TotalServiceCount != 2 || !jne.Selectable || !jne.Services[0].Selected ||
		jne.Services[1].Selected || !jne.Services[1].Selectable {
		t.Fatalf("unexpected JNE catalog: %#v", jne)
	}
	if !catalog.Limits.Enforced || catalog.Limits.Couriers.Maximum != 5 ||
		catalog.Limits.Couriers.Selected != 1 || catalog.Limits.Couriers.Remaining != 4 ||
		catalog.Limits.Services.Maximum != 20 || catalog.Limits.Services.Selected != 1 ||
		catalog.Limits.Services.Remaining != 19 {
		t.Fatalf("unexpected catalog limits: %#v", catalog.Limits)
	}
}

func TestSelectedCourierCodesUsesSavedServiceConfiguration(t *testing.T) {
	t.Parallel()
	repository := &memoryRepository{preference: Preference{
		Configured: true,
		Mode:       ModeCustom,
		Services: []Selection{
			{CourierCode: "jnt", ServiceCode: "EZ"},
			{CourierCode: "jne", ServiceCode: "REG"},
			{CourierCode: "jne", ServiceCode: "JTR"},
		},
	}}
	service := NewService(repository, courierRepository{items: append(
		testCouriers(),
		couriers.Courier{
			Code: "jnt",
			Name: "J&T",
			Services: []couriers.Service{{
				Code: "EZ", Name: "J&T EZ", Group: "regular", ServiceType: "parcel",
			}},
		},
	)})

	codes, err := service.SelectedCourierCodes(context.Background(), "merchant_123")
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 2 || codes[0] != "jne" || codes[1] != "jnt" {
		t.Fatalf("unexpected selected couriers: %#v", codes)
	}

	repository.preference = Preference{Configured: true, Mode: ModeCustom}
	_, err = service.SelectedCourierCodes(context.Background(), "merchant_123")
	if !errors.Is(err, ErrNoSelectedServices) {
		t.Fatalf("empty selection error=%v want ErrNoSelectedServices", err)
	}
}

func TestUpdateOnlyAcceptsCustomMode(t *testing.T) {
	t.Parallel()
	service := NewService(&memoryRepository{}, courierRepository{items: testCouriers()})
	for _, mode := range []string{ModeAll, ModeGroups} {
		_, err := service.Update(context.Background(), "merchant_123", UpdateInput{
			Mode:          mode,
			EnabledGroups: []string{"regular"},
		})
		if !errors.Is(err, ErrInvalidPreference) {
			t.Fatalf("mode=%s error=%v want ErrInvalidPreference", mode, err)
		}
	}

	preference, err := service.Update(context.Background(), "merchant_123", UpdateInput{
		Services: []Selection{{CourierCode: "jne", ServiceCode: "REG"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if preference.Mode != ModeCustom {
		t.Fatalf("mode=%s want custom", preference.Mode)
	}
}

func TestUpdateEnforcesCourierAndServiceLimits(t *testing.T) {
	t.Parallel()
	catalog := []couriers.Courier{
		{
			Code: "jne",
			Name: "JNE",
			Services: []couriers.Service{
				{Code: "REG", Name: "JNE Regular", Group: "regular"},
				{Code: "YES", Name: "JNE YES", Group: "next_day"},
				{Code: "JTR", Name: "JNE Trucking", Group: "cargo"},
			},
		},
		{
			Code:     "jnt",
			Name:     "J&T",
			Services: []couriers.Service{{Code: "EZ", Name: "J&T EZ", Group: "regular"}},
		},
	}
	service := NewService(
		&memoryRepository{},
		courierRepository{items: catalog},
		WithSelectionLimits(1, 2),
	)

	_, err := service.Update(context.Background(), "merchant_123", UpdateInput{
		Mode: ModeCustom,
		Services: []Selection{
			{CourierCode: "jne", ServiceCode: "REG"},
			{CourierCode: "jnt", ServiceCode: "EZ"},
		},
	})
	var limitError *SelectionLimitError
	if !errors.As(err, &limitError) || limitError.RequestedCouriers != 2 ||
		limitError.RequestedServices != 2 {
		t.Fatalf("courier limit error=%#v", err)
	}

	_, err = service.Update(context.Background(), "merchant_123", UpdateInput{
		Mode: ModeCustom,
		Services: []Selection{
			{CourierCode: "jne", ServiceCode: "REG"},
			{CourierCode: "jne", ServiceCode: "YES"},
			{CourierCode: "jne", ServiceCode: "JTR"},
		},
	})
	if !errors.As(err, &limitError) || limitError.RequestedServices != 3 {
		t.Fatalf("service limit error=%#v", err)
	}
}

func TestCatalogDisablesUnselectedOptionsAtLimit(t *testing.T) {
	t.Parallel()
	repository := &memoryRepository{preference: Preference{
		Configured: true,
		Mode:       ModeCustom,
		Services:   []Selection{{CourierCode: "jne", ServiceCode: "REG"}},
	}}
	service := NewService(
		repository,
		courierRepository{items: []couriers.Courier{
			{
				Code: "jne",
				Name: "JNE",
				Services: []couriers.Service{
					{Code: "REG", Name: "JNE Regular", Group: "regular"},
					{Code: "YES", Name: "JNE YES", Group: "next_day"},
				},
			},
			{
				Code:     "jnt",
				Name:     "J&T",
				Services: []couriers.Service{{Code: "EZ", Name: "J&T EZ", Group: "regular"}},
			},
		}},
		WithSelectionLimits(1, 2),
	)

	catalog, err := service.Catalog(context.Background(), "merchant_123")
	if err != nil {
		t.Fatal(err)
	}
	if !catalog.Couriers[0].Selectable || !catalog.Couriers[0].Services[1].Selectable {
		t.Fatalf("selected courier must still allow its remaining service: %#v", catalog.Couriers[0])
	}
	if catalog.Couriers[1].Selectable || catalog.Couriers[1].Services[0].Selectable {
		t.Fatalf("new courier must be disabled at courier limit: %#v", catalog.Couriers[1])
	}
}

func TestFilterUsesCanonicalServiceAndGroup(t *testing.T) {
	t.Parallel()
	results := []rates.Result{
		{Card: rates.RateCard{
			CourierCode: "jne", ServiceCode: "REG23", CanonicalServiceCode: "REG", ServiceGroup: "regular",
		}},
		{Card: rates.RateCard{
			CourierCode: "jne", ServiceCode: "CTCJTR", CanonicalServiceCode: "JTR", ServiceGroup: "cargo",
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
		EnabledGroups: []string{"cargo"},
	}
	filtered, err = service.Filter(context.Background(), "merchant_123", results)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].Card.ServiceCode != "CTCJTR" {
		t.Fatalf("unexpected group filter result: %#v", filtered)
	}
}

func TestCatalogOnlyExposesEmisellCheckoutGroups(t *testing.T) {
	t.Parallel()
	repository := &memoryRepository{preference: Preference{
		Configured: true,
		Mode:       ModeCustom,
		Services: []Selection{
			{CourierCode: "jne", ServiceCode: "REG"},
			{CourierCode: "jne", ServiceCode: "SPS"},
		},
	}}
	items := append(testCouriers(), couriers.Courier{
		Code: "instant-only",
		Name: "Instant Only",
		Services: []couriers.Service{
			{Code: "INSTANT", Name: "Instant", Group: "instant", ServiceType: "instant"},
		},
	})
	service := NewService(repository, courierRepository{items: items})
	catalog, err := service.Catalog(context.Background(), "merchant_123")
	if err != nil {
		t.Fatal(err)
	}
	wantGroups := []string{"regular", "next_day", "economy", "cargo"}
	if len(catalog.Groups) != len(wantGroups) {
		t.Fatalf("groups=%#v", catalog.Groups)
	}
	for index, group := range catalog.Groups {
		if group.Code != wantGroups[index] {
			t.Fatalf("group[%d]=%s want %s", index, group.Code, wantGroups[index])
		}
	}
	if len(catalog.Preference.Services) != 1 || catalog.Preference.Services[0].ServiceCode != "REG" {
		t.Fatalf("hidden preference leaked: %#v", catalog.Preference.Services)
	}
	if len(catalog.Couriers) != 1 || len(catalog.Couriers[0].Services) != 2 {
		t.Fatalf("unexpected filtered catalog: %#v", catalog.Couriers)
	}
	for _, item := range catalog.Couriers[0].Services {
		if !supportedGroup(item.Group) {
			t.Fatalf("unsupported group leaked: %#v", item)
		}
	}
}

func TestUnconfiguredTenantDoesNotExposeServices(t *testing.T) {
	t.Parallel()
	repository := &memoryRepository{preference: Preference{
		Configured: false,
		Mode:       ModeCustom,
	}}
	service := NewService(repository, courierRepository{})
	results := []rates.Result{{Card: rates.RateCard{
		CourierCode: "new", ServiceCode: "RAW", ServiceGroup: "unknown",
	}}}
	filtered, err := service.Filter(context.Background(), "merchant_123", results)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 0 {
		t.Fatalf("unconfigured preference exposed checkout results: %#v", filtered)
	}
}

func testCouriers() []couriers.Courier {
	return []couriers.Courier{{
		Code: "jne",
		Name: "JNE",
		Services: []couriers.Service{
			{Code: "REG", Name: "JNE Regular", Group: "regular", ServiceType: "parcel"},
			{Code: "JTR", Name: "JNE Trucking", Group: "cargo", ServiceType: "cargo"},
			{Code: "SPS", Name: "JNE Super Speed", Group: "express", ServiceType: "parcel"},
		},
	}}
}
