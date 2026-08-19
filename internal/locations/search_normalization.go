package locations

import (
	"strings"
	"unicode"
)

const maxLocationSearchTerms = 16

var locationSearchStopWords = map[string]struct{}{
	"alamat":    {},
	"blok":      {},
	"desa":      {},
	"dsn":       {},
	"dusun":     {},
	"gang":      {},
	"gedung":    {},
	"gg":        {},
	"indonesia": {},
	"jalan":     {},
	"jl":        {},
	"jln":       {},
	"kab":       {},
	"kabupaten": {},
	"kec":       {},
	"kecamatan": {},
	"kel":       {},
	"kelurahan": {},
	"komplek":   {},
	"kota":      {},
	"kp":        {},
	"lantai":    {},
	"no":        {},
	"nomor":     {},
	"perumahan": {},
	"prov":      {},
	"provinsi":  {},
	"rt":        {},
	"rumah":     {},
	"ruko":      {},
	"rw":        {},
	"unit":      {},
}

// normalizeLocationSearchTerms turns a raw Indonesian address into bounded,
// unique lookup terms. Street labels and administrative prefixes are removed,
// while unknown words are retained as low-value terms so callers do not need
// to parse an address before using the location API.
func normalizeLocationSearchTerms(value string) []string {
	var normalized strings.Builder
	normalized.Grow(len(value))
	for _, character := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			normalized.WriteRune(character)
			continue
		}
		normalized.WriteByte(' ')
	}

	terms := make([]string, 0, maxLocationSearchTerms)
	seen := make(map[string]struct{}, maxLocationSearchTerms)
	for _, term := range strings.Fields(normalized.String()) {
		if _, ignored := locationSearchStopWords[term]; ignored {
			continue
		}
		if isDigits(term) && len(term) != 5 {
			continue
		}
		if len([]rune(term)) < 2 || len(term) > 64 {
			continue
		}
		if _, duplicate := seen[term]; duplicate {
			continue
		}
		seen[term] = struct{}{}
		terms = append(terms, term)
		if len(terms) == maxLocationSearchTerms {
			break
		}
	}
	return terms
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if !unicode.IsDigit(character) {
			return false
		}
	}
	return true
}
