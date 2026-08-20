package providercredentials

import (
	"context"
	"errors"
	"testing"
)

type registryValidatorStub struct {
	calls int
}

func (v *registryValidatorStub) Validate(context.Context, string, string) error {
	v.calls++
	return nil
}

func TestValidatorRegistryRoutesByProviderCode(t *testing.T) {
	t.Parallel()
	raja := &registryValidatorStub{}
	biteship := &registryValidatorStub{}
	registry := NewValidatorRegistry(map[string]Validator{
		"rajaongkir": raja,
		"biteship":   biteship,
	})
	if err := registry.Validate(context.Background(), "biteship", "token"); err != nil {
		t.Fatal(err)
	}
	if raja.calls != 0 || biteship.calls != 1 {
		t.Fatalf("unexpected calls: raja=%d biteship=%d", raja.calls, biteship.calls)
	}
	if err := registry.Validate(context.Background(), "unknown", "token"); !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("expected unsupported provider, got %v", err)
	}
}
