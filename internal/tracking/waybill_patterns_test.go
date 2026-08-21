package tracking

import "testing"

func TestRankedCourierCandidatesKeepsSellerChoiceThenDetectsJNT(t *testing.T) {
	t.Parallel()
	candidates, formatStatus := rankedCourierCandidates(
		"JY1224870535", "jne", []string{"jne", "jnt", "lion"},
	)
	if formatStatus != "matched" {
		// JNE has a deliberately broad legacy format, so provider verification
		// remains authoritative even though J&T is the stronger candidate.
		t.Fatalf("unexpected format status: %s", formatStatus)
	}
	if len(candidates) < 2 || candidates[0] != "jne" || candidates[1] != "jnt" {
		t.Fatalf("unexpected candidates: %#v", candidates)
	}
}

func TestRankedCourierCandidatesDoesNotRejectUnknownFormat(t *testing.T) {
	t.Parallel()
	candidates, formatStatus := rankedCourierCandidates(
		"NEWFORMAT-2026", "jne", []string{"jne", "jnt"},
	)
	if formatStatus != "unclassified" {
		t.Fatalf("unexpected format status: %s", formatStatus)
	}
	if len(candidates) != 1 || candidates[0] != "jne" {
		t.Fatalf("seller-selected provider must still be checked: %#v", candidates)
	}
}
