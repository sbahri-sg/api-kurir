package locations

import "testing"

func TestNormalizeRegionDisplayName(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		" JAWA   TIMUR ":                 "Jawa Timur",
		"DKI JAKARTA":                    "DKI Jakarta",
		"DI YOGYAKARTA":                  "DI Yogyakarta",
		"NUSA TENGGARA BARAT (NTB)":      "Nusa Tenggara Barat (NTB)",
		"NANGGROE ACEH DARUSSALAM (NAD)": "Nanggroe Aceh Darussalam (NAD)",
		"BAU-BAU":                        "Bau-Bau",
		"GANTORANG/GANTARANG":            "Gantorang/Gantarang",
		"ILIR TIMUR II":                  "Ilir Timur II",
		"KEP. BALA BALAKANG":             "Kep. Bala Balakang",
		"KI'E":                           "Ki'e",
		"":                               "",
	}
	for input, expected := range tests {
		if actual := normalizeRegionDisplayName(input); actual != expected {
			t.Errorf(
				"normalizeRegionDisplayName(%q): got %q, want %q",
				input,
				actual,
				expected,
			)
		}
	}
}
