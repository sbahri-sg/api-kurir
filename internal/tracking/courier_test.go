package tracking

import "testing"

func TestNormalizeResultCourierUsesCanonicalProviderAlias(t *testing.T) {
	t.Parallel()

	result := normalizeResultCourier(Result{Summary: map[string]any{
		"courier_code": "J&T Express",
	}}, "jnt")
	if result.Summary["courier_code"] != "jnt" {
		t.Fatalf("unexpected courier summary: %#v", result.Summary)
	}
}

func TestNormalizeResultCourierFallsBackToRequestedCourier(t *testing.T) {
	t.Parallel()

	result := normalizeResultCourier(Result{}, "Si Cepat")
	if result.Summary["courier_code"] != "sicepat" {
		t.Fatalf("unexpected courier summary: %#v", result.Summary)
	}
}
