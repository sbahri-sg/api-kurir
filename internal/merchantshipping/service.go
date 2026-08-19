package merchantshipping

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/emisell/api-kurir/internal/couriers"
	"github.com/emisell/api-kurir/internal/rates"
)

var (
	ErrInvalidTenant     = errors.New("tenant ID is invalid")
	ErrInvalidPreference = errors.New("shipping service preference is invalid")
	ErrUnknownService    = errors.New("shipping service is not in the active catalog")
)

const maxCustomServices = 200

type Service struct {
	repository Repository
	couriers   couriers.Repository
}

func NewService(repository Repository, courierRepository couriers.Repository) *Service {
	return &Service{repository: repository, couriers: courierRepository}
}

func (s *Service) Catalog(ctx context.Context, tenantID string) (Catalog, error) {
	tenantID = strings.TrimSpace(tenantID)
	if !validTenantID(tenantID) {
		return Catalog{}, ErrInvalidTenant
	}
	preference, err := s.repository.Get(ctx, tenantID)
	if err != nil {
		return Catalog{}, err
	}
	courierCatalog, err := s.couriers.List(ctx)
	if err != nil {
		return Catalog{}, err
	}
	result := Catalog{
		Preference: preference,
		Groups:     groupOptions(),
		Couriers:   make([]CatalogCourier, 0, len(courierCatalog)),
	}
	for _, courier := range courierCatalog {
		item := catalogCourier(courier)
		for _, service := range courier.Services {
			if strings.EqualFold(strings.TrimSpace(service.Group), "unknown") {
				continue
			}
			selected := preferenceAllows(
				preference,
				courier.Code,
				service.Code,
				service.Group,
			)
			if selected {
				item.SelectedServiceCount++
			}
			item.Services = append(item.Services, CatalogService{
				Code:            service.Code,
				Name:            service.Name,
				Group:           service.Group,
				ServiceType:     service.ServiceType,
				CalculationMode: service.CalculationMode,
				Selected:        selected,
			})
		}
		item.TotalServiceCount = len(item.Services)
		switch {
		case item.SelectedServiceCount == 0:
			item.SelectionState = "none"
		case item.SelectedServiceCount == item.TotalServiceCount:
			item.SelectionState = "all"
		default:
			item.SelectionState = "partial"
		}
		result.Couriers = append(result.Couriers, item)
	}
	return result, nil
}

func (s *Service) Update(
	ctx context.Context,
	tenantID string,
	input UpdateInput,
) (Preference, error) {
	tenantID = strings.TrimSpace(tenantID)
	if !validTenantID(tenantID) {
		return Preference{}, ErrInvalidTenant
	}
	normalized, err := s.normalizeUpdate(ctx, input)
	if err != nil {
		return Preference{}, err
	}
	return s.repository.Replace(ctx, tenantID, normalized)
}

// Filter implements rates.ResultPolicy. Tenants without a saved preference
// keep the historical allow-all behaviour for backwards compatibility.
func (s *Service) Filter(
	ctx context.Context,
	tenantID string,
	results []rates.Result,
) ([]rates.Result, error) {
	preference, err := s.repository.Get(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if !preference.Configured {
		return results, nil
	}
	filtered := make([]rates.Result, 0, len(results))
	for _, result := range results {
		canonicalCode := result.Card.CanonicalServiceCode
		if canonicalCode == "" {
			canonicalCode = result.Card.ServiceCode
		}
		if preferenceAllows(
			preference,
			result.Card.CourierCode,
			canonicalCode,
			result.Card.ServiceGroup,
		) {
			filtered = append(filtered, result)
		}
	}
	return filtered, nil
}

func (s *Service) normalizeUpdate(
	ctx context.Context,
	input UpdateInput,
) (UpdateInput, error) {
	input.Mode = strings.ToLower(strings.TrimSpace(input.Mode))
	input.UpdatedBy = strings.TrimSpace(input.UpdatedBy)
	if input.Mode != ModeAll && input.Mode != ModeGroups && input.Mode != ModeCustom {
		return UpdateInput{}, ErrInvalidPreference
	}

	groups, validGroups := normalizeGroups(input.EnabledGroups)
	if !validGroups {
		return UpdateInput{}, ErrInvalidPreference
	}
	selections, validSelections := normalizeSelections(input.Services)
	if !validSelections {
		return UpdateInput{}, ErrInvalidPreference
	}
	switch input.Mode {
	case ModeAll:
		if len(groups) != 0 || len(selections) != 0 {
			return UpdateInput{}, ErrInvalidPreference
		}
	case ModeGroups:
		if len(groups) == 0 || len(selections) != 0 {
			return UpdateInput{}, ErrInvalidPreference
		}
	case ModeCustom:
		if len(groups) != 0 || len(selections) > maxCustomServices {
			return UpdateInput{}, ErrInvalidPreference
		}
	}

	if input.Mode == ModeCustom && len(selections) > 0 {
		catalog, err := s.couriers.List(ctx)
		if err != nil {
			return UpdateInput{}, err
		}
		available := make(map[string]struct{})
		for _, courier := range catalog {
			for _, service := range courier.Services {
				if service.Group == "unknown" {
					continue
				}
				available[selectionKey(courier.Code, service.Code)] = struct{}{}
			}
		}
		for _, selection := range selections {
			if _, exists := available[selectionKey(
				selection.CourierCode,
				selection.ServiceCode,
			)]; !exists {
				return UpdateInput{}, ErrUnknownService
			}
		}
	}

	input.EnabledGroups = groups
	input.Services = selections
	return input, nil
}

func preferenceAllows(
	preference Preference,
	courierCode string,
	serviceCode string,
	serviceGroup string,
) bool {
	if !preference.Configured {
		return true
	}
	serviceGroup = strings.ToLower(strings.TrimSpace(serviceGroup))
	switch preference.Mode {
	case ModeAll:
		return serviceGroup != "" && serviceGroup != "unknown"
	case ModeGroups:
		for _, group := range preference.EnabledGroups {
			if group == serviceGroup {
				return true
			}
		}
		return false
	case ModeCustom:
		key := selectionKey(courierCode, serviceCode)
		for _, selection := range preference.Services {
			if selectionKey(selection.CourierCode, selection.ServiceCode) == key {
				return true
			}
		}
	}
	return false
}

func normalizeGroups(input []string) ([]string, bool) {
	valid := make(map[string]struct{}, len(SupportedGroups))
	for _, group := range SupportedGroups {
		valid[group] = struct{}{}
	}
	seen := make(map[string]struct{}, len(input))
	result := make([]string, 0, len(input))
	for _, group := range input {
		group = strings.ToLower(strings.TrimSpace(group))
		if _, exists := valid[group]; !exists {
			return nil, false
		}
		if _, duplicate := seen[group]; duplicate {
			continue
		}
		seen[group] = struct{}{}
		result = append(result, group)
	}
	sort.Strings(result)
	return result, true
}

func normalizeSelections(input []Selection) ([]Selection, bool) {
	seen := make(map[string]struct{}, len(input))
	result := make([]Selection, 0, len(input))
	for _, selection := range input {
		selection.CourierCode = strings.ToLower(strings.TrimSpace(selection.CourierCode))
		selection.ServiceCode = strings.ToUpper(strings.TrimSpace(selection.ServiceCode))
		if selection.CourierCode == "" || selection.ServiceCode == "" ||
			len(selection.CourierCode) > 32 || len(selection.ServiceCode) > 64 {
			return nil, false
		}
		key := selectionKey(selection.CourierCode, selection.ServiceCode)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, selection)
	}
	sort.Slice(result, func(left, right int) bool {
		return selectionKey(result[left].CourierCode, result[left].ServiceCode) <
			selectionKey(result[right].CourierCode, result[right].ServiceCode)
	})
	return result, true
}

func selectionKey(courierCode, serviceCode string) string {
	return strings.ToLower(strings.TrimSpace(courierCode)) + ":" +
		strings.ToUpper(strings.TrimSpace(serviceCode))
}

func validTenantID(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune("._:-", character) {
			continue
		}
		return false
	}
	return true
}

func groupOptions() []GroupOption {
	names := map[string]string{
		"economy":       "Economy",
		"regular":       "Regular",
		"next_day":      "Next Day",
		"express":       "Express",
		"same_day":      "Same Day",
		"instant":       "Instant",
		"cargo":         "Cargo",
		"international": "International",
		"special":       "Special",
	}
	result := make([]GroupOption, 0, len(SupportedGroups))
	for _, group := range SupportedGroups {
		result = append(result, GroupOption{Code: group, Name: names[group]})
	}
	return result
}
