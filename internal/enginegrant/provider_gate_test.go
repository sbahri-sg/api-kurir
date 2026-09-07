package enginegrant

import (
	"context"
	"errors"
	"testing"
)

type bindingFunc func(context.Context, string, string) (ProviderBinding, error)

func (f bindingFunc) Resolve(c context.Context, m, p string) (ProviderBinding, error) {
	return f(c, m, p)
}

type grantFunc func(context.Context, ProviderBinding, string) error

func (f grantFunc) Verify(c context.Context, b ProviderBinding, o string) error { return f(c, b, o) }

type credentialFunc func(context.Context, string, string, string) error

func (f credentialFunc) VerifyCredential(c context.Context, m, p, id string) error {
	return f(c, m, p, id)
}

func TestProviderBindingAndLiveGrant(t *testing.T) {
	b := ProviderBinding{MerchantID: "merchant-a", ProviderCode: "rajaongkir", AppID: "app-raja", InstallationID: "ins-a", CredentialID: "cred-a", Active: true}
	var resolveErr, grantErr, credentialErr error
	var operations []string
	credentials := 0
	g, err := NewProviderGate(map[string]ProviderPolicy{"rajaongkir": {RequiresCredential: true, MerchantIDs: []string{"merchant-a"}}},
		bindingFunc(func(_ context.Context, m, p string) (ProviderBinding, error) { return b, resolveErr }),
		grantFunc(func(_ context.Context, got ProviderBinding, op string) error {
			operations = append(operations, op)
			if got != b {
				t.Fatal("wrong binding")
			}
			return grantErr
		}),
		credentialFunc(func(_ context.Context, m, p, id string) error {
			credentials++
			if m != "merchant-a" || p != "rajaongkir" || id != "cred-a" {
				t.Fatal("wrong credential identity")
			}
			return credentialErr
		}))
	if err != nil {
		t.Fatal(err)
	}
	// Rollout must not migrate other existing merchants using the same provider.
	if err := g.Authorize(context.Background(), "legacy-merchant", "rajaongkir", "rates.read"); err != nil {
		t.Fatal("legacy merchant affected", err)
	}
	check := func(op string, want error) {
		t.Helper()
		if err := g.Authorize(context.Background(), "merchant-a", "rajaongkir", op); !errors.Is(err, want) {
			t.Fatalf("%s: %v want %v", op, err, want)
		}
	}
	for _, op := range []string{"rates.read", "settings.read", "settings.write", "shipments.create", "tracking.read"} {
		check(op, nil)
	}
	if len(operations) != 5 || operations[1] == operations[2] {
		t.Fatal("operation separation lost")
	}
	grantErr = ErrDenied
	check("rates.read", ErrDenied)
	if credentials != 5 {
		t.Fatal("revoked installation reached credentials")
	}
	grantErr = errors.New("offline")
	check("rates.read", ErrUnavailable)
	grantErr = nil
	credentialErr = ErrDenied
	check("rates.read", ErrDenied)
	credentialErr = nil
	for _, change := range []func(){func() { b.MerchantID = "merchant-b" }, func() { b.ProviderCode = "emisell" }, func() { b.Active = false }, func() { b.InstallationID = "" }, func() { b.AppID = "" }, func() { b.CredentialID = "" }} {
		original := b
		change()
		check("rates.read", ErrDenied)
		b = original
	}
	resolveErr = ErrBindingNotFound
	check("rates.read", ErrDenied)
	resolveErr = errors.New("database offline")
	check("rates.read", ErrUnavailable)
	// Existing built-in provider is not enrolled and must not use RajaOngkir's binding.
	if err := g.Authorize(context.Background(), "merchant-a", "emisell", "rates.read"); err != nil {
		t.Fatal(err)
	}
	check("unknown.operation", ErrDenied)
}

func TestProviderPoliciesAreGenericAndCopied(t *testing.T) {
	policies := map[string]ProviderPolicy{"another_provider": {MerchantIDs: []string{"m"}}}
	b := ProviderBinding{MerchantID: "m", ProviderCode: "another_provider", AppID: "app", InstallationID: "ins", Active: true}
	g, err := NewProviderGate(policies, bindingFunc(func(context.Context, string, string) (ProviderBinding, error) { return b, nil }), grantFunc(func(context.Context, ProviderBinding, string) error { return ErrDenied }), nil)
	if err != nil {
		t.Fatal(err)
	}
	delete(policies, "another_provider")
	if err := g.Authorize(context.Background(), "m", "another_provider", "rates.read"); !errors.Is(err, ErrDenied) {
		t.Fatal("caller mutated enrollment")
	}
	if _, err := NewProviderGate(map[string]ProviderPolicy{"rajaongkir": {RequiresCredential: true}}, g.bindings, g.grants, nil); err == nil {
		t.Fatal("missing credential verifier")
	}
}

func TestRejectBuiltInAndImplicitRollout(t *testing.T) {
	b := bindingFunc(func(context.Context, string, string) (ProviderBinding, error) {
		return ProviderBinding{}, ErrBindingNotFound
	})
	v := grantFunc(func(context.Context, ProviderBinding, string) error { return ErrDenied })
	for _, policies := range []map[string]ProviderPolicy{
		{"emisell": {MerchantIDs: []string{"m"}}},
		{"rajaongkir": {}},
		{"rajaongkir": {MerchantIDs: []string{"*"}}},
	} {
		if _, err := NewProviderGate(policies, b, v, nil); err == nil {
			t.Fatal("unsafe rollout accepted")
		}
	}
	policies := map[string]ProviderPolicy{"rajaongkir": {MerchantIDs: []string{"m"}}}
	g, err := NewProviderGate(policies, b, v, nil)
	if err != nil {
		t.Fatal(err)
	}
	policies["rajaongkir"].MerchantIDs[0] = "other"
	if !errors.Is(g.Authorize(context.Background(), "m", "rajaongkir", "rates.read"), ErrDenied) {
		t.Fatal("rollout list mutated")
	}
}
