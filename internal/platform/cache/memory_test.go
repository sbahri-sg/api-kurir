package cache

import (
	"context"
	"testing"
	"time"
)

func TestMemoryCacheCopiesValuesAndExpires(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 28, 0, 0, 0, 0, time.UTC)
	cache := NewMemory(2)
	cache.now = func() time.Time { return now }

	input := []byte("rate")
	if err := cache.Set(context.Background(), "key", input, time.Minute); err != nil {
		t.Fatal(err)
	}
	input[0] = 'X'

	value, found, err := cache.Get(context.Background(), "key")
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(value) != "rate" {
		t.Fatalf("unexpected cached value: found=%v value=%q", found, value)
	}

	value[0] = 'Y'
	valueAgain, found, err := cache.Get(context.Background(), "key")
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(valueAgain) != "rate" {
		t.Fatalf("cache leaked mutable bytes: found=%v value=%q", found, valueAgain)
	}

	now = now.Add(time.Minute)
	_, found, err = cache.Get(context.Background(), "key")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected expired value to be removed")
	}
}
