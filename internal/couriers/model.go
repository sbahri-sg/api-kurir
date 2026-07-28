package couriers

type Service struct {
	Code            string `json:"code"`
	Name            string `json:"name"`
	ServiceType     string `json:"service_type"`
	CalculationMode string `json:"calculation_mode"`
}

type Courier struct {
	Code                      string    `json:"code"`
	Name                      string    `json:"name"`
	ProviderCode              string    `json:"provider_code"`
	SupportsDomesticCost      bool      `json:"supports_domestic_cost"`
	SupportsInternationalCost bool      `json:"supports_international_cost"`
	SupportsTracking          bool      `json:"supports_tracking"`
	CatalogSource             string    `json:"catalog_source"`
	CatalogVerifiedAt         string    `json:"catalog_verified_at"`
	Services                  []Service `json:"services"`
}
