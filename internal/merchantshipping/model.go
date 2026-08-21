package merchantshipping

import (
	"time"

	"github.com/emisell/api-kurir/internal/couriers"
)

const (
	ModeAll    = "all"
	ModeGroups = "groups"
	ModeCustom = "custom"
)

var SupportedGroups = []string{
	"regular",
	"next_day",
	"economy",
	"cargo",
}

type Selection struct {
	CourierCode string `json:"courier_code"`
	ServiceCode string `json:"service_code"`
}

type Preference struct {
	Configured    bool        `json:"configured"`
	Mode          string      `json:"mode"`
	EnabledGroups []string    `json:"enabled_groups"`
	Services      []Selection `json:"services"`
	Version       int64       `json:"version"`
	UpdatedAt     *time.Time  `json:"updated_at"`
}

type UpdateInput struct {
	Mode          string
	EnabledGroups []string
	Services      []Selection
	UpdatedBy     string
}

type GroupOption struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type CatalogService struct {
	Code            string `json:"code"`
	Name            string `json:"name"`
	Group           string `json:"group"`
	ServiceType     string `json:"service_type"`
	CalculationMode string `json:"calculation_mode"`
	Selected        bool   `json:"selected"`
	Selectable      bool   `json:"selectable"`
}

type CatalogCourier struct {
	Code                      string           `json:"code"`
	Name                      string           `json:"name"`
	ProviderCode              string           `json:"provider_code"`
	RateProviderCode          string           `json:"rate_provider_code"`
	TrackingProviderCode      string           `json:"tracking_provider_code"`
	SupportsDomesticCost      bool             `json:"supports_domestic_cost"`
	SupportsInternationalCost bool             `json:"supports_international_cost"`
	SupportsTracking          bool             `json:"supports_tracking"`
	SelectionState            string           `json:"selection_state"`
	Selectable                bool             `json:"selectable"`
	SelectedServiceCount      int              `json:"selected_service_count"`
	TotalServiceCount         int              `json:"total_service_count"`
	Services                  []CatalogService `json:"services"`
}

type LimitUsage struct {
	Maximum   int `json:"maximum"`
	Selected  int `json:"selected"`
	Remaining int `json:"remaining"`
	Available int `json:"available"`
}

type SelectionLimits struct {
	Enforced bool       `json:"enforced"`
	Couriers LimitUsage `json:"couriers"`
	Services LimitUsage `json:"services"`
}

type Catalog struct {
	Preference Preference       `json:"preference"`
	Limits     SelectionLimits  `json:"limits"`
	Groups     []GroupOption    `json:"groups"`
	Couriers   []CatalogCourier `json:"couriers"`
}

type SelectionLimitError struct {
	MaxCouriers       int `json:"max_couriers"`
	MaxServices       int `json:"max_services"`
	RequestedCouriers int `json:"requested_couriers"`
	RequestedServices int `json:"requested_services"`
}

func (e *SelectionLimitError) Error() string {
	return "shipping service selection limit exceeded"
}

func (e *SelectionLimitError) Unwrap() error {
	return ErrSelectionLimitExceeded
}

func catalogCourier(source couriers.Courier) CatalogCourier {
	return CatalogCourier{
		Code:                      source.Code,
		Name:                      source.Name,
		ProviderCode:              source.ProviderCode,
		RateProviderCode:          source.RateProviderCode,
		TrackingProviderCode:      source.TrackingProviderCode,
		SupportsDomesticCost:      source.SupportsDomesticCost,
		SupportsInternationalCost: source.SupportsInternationalCost,
		SupportsTracking:          source.SupportsTracking,
		Services:                  make([]CatalogService, 0, len(source.Services)),
	}
}
