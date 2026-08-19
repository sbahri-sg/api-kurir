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
	"economy",
	"regular",
	"next_day",
	"express",
	"same_day",
	"instant",
	"cargo",
	"international",
	"special",
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
}

type CatalogCourier struct {
	Code                      string           `json:"code"`
	Name                      string           `json:"name"`
	ProviderCode              string           `json:"provider_code"`
	SupportsDomesticCost      bool             `json:"supports_domestic_cost"`
	SupportsInternationalCost bool             `json:"supports_international_cost"`
	SupportsTracking          bool             `json:"supports_tracking"`
	SelectionState            string           `json:"selection_state"`
	SelectedServiceCount      int              `json:"selected_service_count"`
	TotalServiceCount         int              `json:"total_service_count"`
	Services                  []CatalogService `json:"services"`
}

type Catalog struct {
	Preference Preference       `json:"preference"`
	Groups     []GroupOption    `json:"groups"`
	Couriers   []CatalogCourier `json:"couriers"`
}

func catalogCourier(source couriers.Courier) CatalogCourier {
	return CatalogCourier{
		Code:                      source.Code,
		Name:                      source.Name,
		ProviderCode:              source.ProviderCode,
		SupportsDomesticCost:      source.SupportsDomesticCost,
		SupportsInternationalCost: source.SupportsInternationalCost,
		SupportsTracking:          source.SupportsTracking,
		Services:                  make([]CatalogService, 0, len(source.Services)),
	}
}
