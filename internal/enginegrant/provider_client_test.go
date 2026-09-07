package enginegrant

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type snapshotWriterFunc func(context.Context, ProviderSnapshot) error

func (f snapshotWriterFunc) ApplySnapshot(c context.Context, v ProviderSnapshot) error {
	return f(c, v)
}
func TestProviderProtocolAndSync(t *testing.T) {
	b := ProviderBinding{MerchantID: "m", ProviderCode: "rajaongkir", AppID: "raja-app", InstallationID: "ins", Active: true}
	v := ProviderSnapshot{MerchantID: b.MerchantID, ProviderCode: b.ProviderCode, AppID: b.AppID, InstallationID: b.InstallationID, Revision: 2, Active: true, Scopes: []string{"shipping.read"}}
	calls := 0
	key := strings.Repeat("a", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/internal/provider-grants/v1/check" || r.Header.Get("Authorization") != "Bearer "+key {
			t.Error("wrong contract")
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["providerCode"] != "rajaongkir" || body["installationId"] != "ins" {
			t.Error("wrong identity")
		}
		json.NewEncoder(w).Encode(v)
	}))
	defer server.Close()
	c, err := NewProviderClient(server.URL, key, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Verify(context.Background(), b, "rates.read"); err != nil {
		t.Fatal(err)
	}
	if err = c.Verify(context.Background(), b, "settings.write"); !errors.Is(err, ErrDenied) {
		t.Fatal("scope bypass")
	}
	v.Active = false
	v.Revoked = true
	v.Revision = 3
	v.Scopes = nil
	synced := false
	if err = c.Sync(context.Background(), b, snapshotWriterFunc(func(_ context.Context, got ProviderSnapshot) error {
		synced = got.Revoked && got.Revision == 3
		return nil
	})); err != nil || !synced {
		t.Fatal("revocation sync", err)
	}
	if err = c.Verify(context.Background(), b, "rates.read"); !errors.Is(err, ErrDenied) {
		t.Fatal("cached grant")
	}
	v.MerchantID = "other"
	if err = c.Sync(context.Background(), b, snapshotWriterFunc(func(context.Context, ProviderSnapshot) error { t.Fatal("identity bypass"); return nil })); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	if calls != 5 {
		t.Fatal("unexpected caching")
	}
	if _, err = NewProviderClient(server.URL, key, false); err == nil {
		t.Fatal("insecure endpoint allowed")
	}
}
