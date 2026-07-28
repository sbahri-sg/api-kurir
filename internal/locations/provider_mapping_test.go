package locations

import "testing"

func TestNormalizePostalCode(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"40132":   "40132",
		" 53131 ": "53131",
		"0":       "",
		"00000":   "",
		"1234":    "",
		"12A45":   "",
	}
	for input, expected := range tests {
		if actual := normalizePostalCode(input); actual != expected {
			t.Errorf(
				"normalizePostalCode(%q): got %q, want %q",
				input,
				actual,
				expected,
			)
		}
	}
}

func TestCanonicalPublicIDDoesNotDependOnProvider(t *testing.T) {
	t.Parallel()

	first := ImportedProviderLocation{
		ProviderLocationID: "123",
		Province:           "Jawa Barat",
		City:               "Kota Bandung",
		District:           "Coblong",
		Subdistrict:        "Dago",
	}
	second := first
	second.ProviderLocationID = "provider-B-999"

	if canonicalPublicID(first) != canonicalPublicID(second) {
		t.Fatal("canonical public ID must not depend on provider location ID")
	}
}
