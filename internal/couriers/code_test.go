package couriers

import "testing"

func TestNormalizeCode(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"jnt":                      "jnt",
		"J&T":                      "jnt",
		"J&T Express":              "jnt",
		"Si Cepat Express":         "sicepat",
		"ID Express":               "ide",
		"POS Indonesia":            "pos",
		"Ninja Xpress":             "ninja",
		"Wahana Prestasi Logistik": "wahana",
		"partner-new":              "partner-new",
	}
	for input, expected := range tests {
		if actual := NormalizeCode(input); actual != expected {
			t.Errorf("NormalizeCode(%q)=%q want %q", input, actual, expected)
		}
	}
}
