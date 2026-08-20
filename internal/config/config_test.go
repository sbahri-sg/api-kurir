package config

import (
	"reflect"
	"testing"
)

func TestOverlappingValuesNormalizesAndDeduplicates(t *testing.T) {
	got := overlappingValues(
		[]string{"jne", "JNT", " tiki "},
		[]string{"sicepat", "JNE", "jne", "jnt"},
	)
	want := []string{"jne", "jnt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("overlap = %v, want %v", got, want)
	}
}
