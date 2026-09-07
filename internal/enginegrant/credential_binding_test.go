package enginegrant

import (
	"context"
	"errors"
	"testing"
)

type bindingWriterFunc func(context.Context, ProviderBinding, string) error

func (f bindingWriterFunc) BindCredential(c context.Context, b ProviderBinding, s string) error {
	return f(c, b, s)
}

func TestConnectCredentialChecks(t *testing.T) {
	b := ProviderBinding{MerchantID: "m", ProviderCode: "rajaongkir", AppID: "app", InstallationID: "ins", Active: true}
	var grantErr, credentialErr error
	writes := 0
	g, err := NewProviderGate(map[string]ProviderPolicy{"rajaongkir": {RequiresCredential: true, MerchantIDs: []string{"m"}}},
		bindingFunc(func(context.Context, string, string) (ProviderBinding, error) { return b, nil }),
		grantFunc(func(_ context.Context, _ ProviderBinding, op string) error {
			if op != "settings.write" {
				t.Fatal(op)
			}
			return grantErr
		}),
		credentialFunc(func(context.Context, string, string, string) error { return credentialErr }))
	if err != nil {
		t.Fatal(err)
	}
	w := bindingWriterFunc(func(_ context.Context, got ProviderBinding, c string) error {
		writes++
		if got != b || c != "cred" {
			t.Fatal("wrong reference")
		}
		return nil
	})
	if err := g.ConnectCredential(context.Background(), b, "cred", w); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*ProviderBinding){func(v *ProviderBinding) { v.MerchantID = "other" }, func(v *ProviderBinding) { v.AppID = "other" }, func(v *ProviderBinding) { v.InstallationID = "other" }, func(v *ProviderBinding) { v.ProviderCode = "emisell" }} {
		v := b
		mutate(&v)
		if err := g.ConnectCredential(context.Background(), v, "cred", w); err == nil {
			t.Fatal("identity allowed")
		}
	}
	grantErr = ErrDenied
	if err := g.ConnectCredential(context.Background(), b, "cred", w); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	grantErr = nil
	credentialErr = ErrDenied
	if err := g.ConnectCredential(context.Background(), b, "cred", w); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	if writes != 1 {
		t.Fatal("unauthorized write")
	}
}
