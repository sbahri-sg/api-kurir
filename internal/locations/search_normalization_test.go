package locations

import (
	"reflect"
	"testing"
)

func TestNormalizeLocationSearchTerms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "raw address",
			input: "Jl. Merdeka No. 12, Kel. Gambir, Kec. Gambir, Kota Jakarta Pusat",
			want:  []string{"merdeka", "gambir", "jakarta", "pusat"},
		},
		{
			name:  "postal code and hierarchy",
			input: "Desa Sasa, Kecamatan Ternate Selatan, Maluku Utara 65432",
			want:  []string{"sasa", "ternate", "selatan", "maluku", "utara", "65432"},
		},
		{
			name:  "deduplicated punctuation",
			input: "Gambir, gambir... DKI Jakarta",
			want:  []string{"gambir", "dki", "jakarta"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeLocationSearchTerms(test.input); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("terms: got %#v want %#v", got, test.want)
			}
		})
	}
}
