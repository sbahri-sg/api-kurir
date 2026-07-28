package main

import "testing"

func TestClassifyRegionCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code       string
		level      string
		parentCode string
	}{
		{code: "11", level: "province"},
		{code: "11.01", level: "city", parentCode: "11"},
		{code: "11.01.02", level: "district", parentCode: "11.01"},
		{
			code:       "11.01.02.2001",
			level:      "subdistrict",
			parentCode: "11.01.02",
		},
	}
	for _, test := range tests {
		level, parentCode, err := classifyRegionCode(test.code)
		if err != nil {
			t.Fatal(err)
		}
		if level != test.level || parentCode != test.parentCode {
			t.Fatalf(
				"%s: got level=%s parent=%s",
				test.code,
				level,
				parentCode,
			)
		}
	}
}

func TestValidPostalCode(t *testing.T) {
	t.Parallel()

	if !validPostalCode("23773") {
		t.Fatal("expected valid postal code")
	}
	for _, invalid := range []string{"", "00000", "1234", "12A45"} {
		if validPostalCode(invalid) {
			t.Fatalf("expected invalid postal code: %q", invalid)
		}
	}
}
