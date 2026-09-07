package enginegrant

import (
	"context"
	"errors"
	"testing"
)

type selectorFunc func(context.Context, string, string) (string, error)

func (f selectorFunc) ActiveCredentialID(c context.Context, m, p string) (string, error) {
	return f(c, m, p)
}

func TestRuntimeDisabled(t *testing.T) {
	g, err := LoadRuntime("", nil, nil)
	if err != nil || g != nil {
		t.Fatal("default changed")
	}
	if _, err := LoadRuntime("relative", nil, nil); err == nil {
		t.Fatal("relative config accepted")
	}
}

func TestCredentialSelection(t *testing.T) {
	for _, tc := range []struct {
		selected        string
		sourceErr, want error
	}{
		{"credential", nil, nil}, {"other", nil, ErrDenied}, {"", nil, ErrDenied}, {"", errors.New("offline"), ErrUnavailable},
	} {
		v := SelectedCredentialVerifier{selectorFunc(func(_ context.Context, m, p string) (string, error) {
			if m != "merchant" || p != "rajaongkir" {
				t.Fatal("identity changed")
			}
			return tc.selected, tc.sourceErr
		})}
		if err := v.VerifyCredential(context.Background(), "merchant", "rajaongkir", "credential"); !errors.Is(err, tc.want) {
			t.Fatal(err, tc.want)
		}
	}
}
