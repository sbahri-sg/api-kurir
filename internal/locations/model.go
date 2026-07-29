package locations

import "strings"

type Location struct {
	PublicID        string   `json:"id"`
	CompatibilityID int64    `json:"compatibility_id,omitempty"`
	ParentPublicID  string   `json:"parent_id,omitempty"`
	Level           string   `json:"level,omitempty"`
	ProvinceID      string   `json:"province_id,omitempty"`
	CityID          string   `json:"city_id,omitempty"`
	DistrictID      string   `json:"district_id,omitempty"`
	SubdistrictID   string   `json:"subdistrict_id,omitempty"`
	Province        string   `json:"province,omitempty"`
	City            string   `json:"city,omitempty"`
	District        string   `json:"district,omitempty"`
	Subdistrict     string   `json:"subdistrict,omitempty"`
	PostalCode      string   `json:"postal_code,omitempty"`
	PostalCodes     []string `json:"postal_codes,omitempty"`
}

type HierarchyLocation struct {
	PublicID        string `json:"id"`
	CompatibilityID int64  `json:"compatibility_id,omitempty"`
	Name            string `json:"name"`
	PostalCode      string `json:"zip_code,omitempty"`
}

func (l Location) Label() string {
	parts := make([]string, 0, 5)
	for _, value := range []string{
		l.Subdistrict,
		l.District,
		l.City,
		l.Province,
		l.PostalCode,
	} {
		if strings.TrimSpace(value) != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, ", ")
}
