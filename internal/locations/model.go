package locations

import "strings"

type Location struct {
	PublicID    string   `json:"id"`
	Province    string   `json:"province,omitempty"`
	City        string   `json:"city,omitempty"`
	District    string   `json:"district,omitempty"`
	Subdistrict string   `json:"subdistrict,omitempty"`
	PostalCode  string   `json:"postal_code,omitempty"`
	PostalCodes []string `json:"postal_codes,omitempty"`
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
