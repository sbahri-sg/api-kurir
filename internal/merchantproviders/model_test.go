package merchantproviders

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProviderJSONIncludesPresentationMetadata(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(Provider{
		Code:        "emisell",
		Name:        "Emisell Kurir",
		Logo:        "https://api-kurir.emisell.com/provider-logos/emisell.svg",
		Description: "Layanan pengiriman bawaan Emisell.",
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(payload)
	for _, field := range []string{`"logo":`, `"description":`} {
		if !strings.Contains(encoded, field) {
			t.Fatalf("provider response does not contain %s: %s", field, encoded)
		}
	}
}
