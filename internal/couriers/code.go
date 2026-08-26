package couriers

import (
	"strings"
	"unicode"
)

// NormalizeCode converts provider-specific courier aliases into API Kurir's
// stable courier codes. Unknown partner codes are preserved in lowercase so a
// new provider can be integrated without waiting for a central alias release.
func NormalizeCode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}

	compact := strings.Map(func(character rune) rune {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			return character
		}
		return -1
	}, value)

	if canonical, exists := courierCodeAliases[compact]; exists {
		return canonical
	}
	return value
}

var courierCodeAliases = map[string]string{
	"anteraja":               "anteraja",
	"idexpress":              "ide",
	"ide":                    "ide",
	"jandt":                  "jnt",
	"jandtexpress":           "jnt",
	"jt":                     "jnt",
	"jtexpress":              "jnt",
	"jnt":                    "jnt",
	"jntexpress":             "jnt",
	"lion":                   "lion",
	"lionparcel":             "lion",
	"ninja":                  "ninja",
	"ninjaexpress":           "ninja",
	"ninjaxpress":            "ninja",
	"pos":                    "pos",
	"posindonesia":           "pos",
	"sap":                    "sap",
	"sapexpress":             "sap",
	"sapx":                   "sap",
	"sapxexpress":            "sap",
	"sicepat":                "sicepat",
	"sicepatexpress":         "sicepat",
	"sentral":                "sentral",
	"sentralcargo":           "sentral",
	"star":                   "star",
	"starcargo":              "star",
	"wahana":                 "wahana",
	"wahanaexpress":          "wahana",
	"wahanaprestasi":         "wahana",
	"wahanaprestasilogistik": "wahana",
}
