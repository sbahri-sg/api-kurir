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
	ErrInvalidTenant          = errors.New("tenant ID is invalid")
	ErrInvalidPreference      = errors.New("shipping service preference is invalid")
	ErrUnknownService         = errors.New("shipping service is not in the active catalog")
	ErrSelectionLimitExceeded = errors.New("shipping service selection limit exceeded")
)

const (
	defaultMaxSelectedCouriers = 5
	defaultMaxSelectedServices = 20
	absoluteMaxCustomServices  = 200
)

type Service struct {
	repository  Repository
	couriers    couriers.Repository
	maxCouriers int
	maxServices int
}

type Option func(*Service)

func WithSelectionLimits(maxCouriers, maxServices int) Option {
	return func(service *Service) {
		if maxCouriers > 0 {
			service.maxCouriers = maxCouriers
		}
		if maxServices > 0 && maxServices <= absoluteMaxCustomServices {
			service.maxServices = maxServices
		}
	}
}

func NewService(
	repository Repository,
	courierRepository couriers.Repository,
	options ...Option,
) *Service {
	service := &Service{
		repository:  repository,
		couriers:    courierRepository,
		maxCouriers: defaultMaxSelectedCouriers,
		maxServices: defaultMaxSelectedServices,
	}
	for _, option := range options {
		option(service)
	}
	return service
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
	preference = filterPreference(preference, courierCatalog)
	result := Catalog{
		Preference: preference,
		Groups:     groupOptions(),
		Couriers:   make([]CatalogCourier, 0, len(courierCatalog)),
	}
	selectedCouriers := 0
	selectedServices := 0
	availableCouriers := 0
	availableServices := 0
	for _, courier := range courierCatalog {
		item := catalogCourier(courier)
		for _, service := range courier.Services {
			if !supportedGroup(service.Group) {
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
				selectedServices++
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
		if item.TotalServiceCount == 0 {
			continue
		}
		availableServices += item.TotalServiceCount
		availableCouriers++
		switch {
		case item.SelectedServiceCount == 0:
			item.SelectionState = "none"
		case item.SelectedServiceCount == item.TotalServiceCount:
			item.SelectionState = "all"
		default:
			item.SelectionState = "partial"
		}
		if item.SelectedServiceCount > 0 {
			selectedCouriers++
		}
		result.Couriers = append(result.Couriers, item)
	}
	result.Limits = SelectionLimits{
		Enforced: true,
		Couriers: limitUsage(s.maxCouriers, selectedCouriers, availableCouriers),
		Services: limitUsage(s.maxServices, selectedServices, availableServices),
	}
	applySelectability(result.Couriers, result.Limits)
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

// Filter implements rates.ResultPolicy. A tenant must explicitly save custom
// services before any rate option is allowed at checkout.
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
		return []rates.Result{}, nil
	}
	filtered := make([]rates.Result, 0, len(results))
	for _, result := range results {
		if !supportedGroup(result.Card.ServiceGroup) {
			continue
		}
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
	if input.Mode == "" {
		input.Mode = ModeCustom
	}
	if input.Mode != ModeCustom {
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
	if len(groups) != 0 || len(selections) > absoluteMaxCustomServices {
		return UpdateInput{}, ErrInvalidPreference
	}

	if len(selections) > 0 {
		catalog, err := s.couriers.List(ctx)
		if err != nil {
			return UpdateInput{}, err
		}
		available := make(map[string]struct{})
		for _, courier := range catalog {
			for _, service := range courier.Services {
				if !supportedGroup(service.Group) {
					continue
				}
				available[selectionKey(courier.Code, service.Code)] = struct{}{}
			}
		}
		selectedCouriers := make(map[string]struct{}, len(selections))
		for _, selection := range selections {
			if _, exists := available[selectionKey(
				selection.CourierCode,
				selection.ServiceCode,
			)]; !exists {
				return UpdateInput{}, ErrUnknownService
			}
			selectedCouriers[selection.CourierCode] = struct{}{}
		}
		if len(selectedCouriers) > s.maxCouriers || len(selections) > s.maxServices {
			return UpdateInput{}, &SelectionLimitError{
				MaxCouriers:       s.maxCouriers,
				MaxServices:       s.maxServices,
				RequestedCouriers: len(selectedCouriers),
				RequestedServices: len(selections),
			}
		}
	}

	input.EnabledGroups = groups
	input.Services = selections
	return input, nil
}

func limitUsage(maximum, selected, available int) LimitUsage {
	remaining := maximum - selected
	if remaining < 0 {
		remaining = 0
	}
	return LimitUsage{
		Maximum:   maximum,
		Selected:  selected,
		Remaining: remaining,
		Available: available,
	}
}

func applySelectability(catalog []CatalogCourier, limits SelectionLimits) {
	for courierIndex := range catalog {
		courier := &catalog[courierIndex]
		courierSelected := courier.SelectedServiceCount > 0
		courier.Selectable = courier.TotalServiceCount > 0 && (!limits.Enforced ||
			courierSelected ||
			(limits.Couriers.Remaining > 0 && limits.Services.Remaining > 0))
		for serviceIndex := range courier.Services {
			service := &courier.Services[serviceIndex]
			service.Selectable = !limits.Enforced ||
				service.Selected ||
				(limits.Services.Remaining > 0 &&
					(courierSelected || limits.Couriers.Remaining > 0))
		}
	}
}

func preferenceAllows(
	preference Preference,
	courierCode string,
	serviceCode string,
	serviceGroup string,
) bool {
	if !preference.Configured {
		return false
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

func supportedGroup(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, group := range SupportedGroups {
		if value == group {
			return true
		}
	}
	return false
}

func filterPreference(preference Preference, catalog []couriers.Courier) Preference {
	allowedSelections := make(map[string]struct{})
	for _, courier := range catalog {
		for _, service := range courier.Services {
			if supportedGroup(service.Group) {
				allowedSelections[selectionKey(courier.Code, service.Code)] = struct{}{}
			}
		}
	}
	services := make([]Selection, 0, len(preference.Services))
	for _, selection := range preference.Services {
		if _, allowed := allowedSelections[selectionKey(
			selection.CourierCode,
			selection.ServiceCode,
		)]; allowed {
			services = append(services, selection)
		}
	}
	groups := make([]string, 0, len(preference.EnabledGroups))
	for _, group := range preference.EnabledGroups {
		if supportedGroup(group) {
			groups = append(groups, group)
		}
	}
	preference.Services = services
	preference.EnabledGroups = groups
	return preference
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
		"economy":  "Economy",
		"regular":  "Regular",
		"next_day": "Next Day",
		"cargo":    "Cargo",
	}
	result := make([]GroupOption, 0, len(SupportedGroups))
	for _, group := range SupportedGroups {
		result = append(result, GroupOption{Code: group, Name: names[group]})
	}
	return result
}
