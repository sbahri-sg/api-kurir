package enginegrant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReconcileDiscoversAndRefreshesRevocation(t *testing.T) {
	revoked := false
	targets := 0
	writes := []ProviderSnapshot{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("a", 64) {
			t.Error("missing engine authentication")
		}
		if strings.HasSuffix(r.URL.Path, "/targets") {
			targets++
			id, next := "ins1", "ins1"
			if r.URL.Query().Get("after") == "ins1" {
				id, next = "ins2", ""
			}
			json.NewEncoder(w).Encode(map[string]any{"targets": []providerTarget{{MerchantID: "m", ProviderCode: "rajaongkir", AppID: "app", InstallationID: id}}, "next": next})
			return
		}
		var b map[string]string
		json.NewDecoder(r.Body).Decode(&b)
		if b["operation"] != "binding.read" {
			t.Error("not a fresh binding read")
		}
		revision := int64(1)
		if revoked {
			revision = 2
		}
		json.NewEncoder(w).Encode(ProviderSnapshot{MerchantID: "m", ProviderCode: "rajaongkir", AppID: "app", InstallationID: b["installationId"], Revision: revision, Active: !revoked, Revoked: revoked})
	}))
	defer srv.Close()
	c, err := NewProviderClient(srv.URL, strings.Repeat("a", 64), true)
	if err != nil {
		t.Fatal(err)
	}
	writer := snapshotWriterFunc(func(_ context.Context, v ProviderSnapshot) error { writes = append(writes, v); return nil })
	n, err := c.Reconcile(context.Background(), writer)
	if err != nil || n != 2 {
		t.Fatal(n, err)
	}
	revoked = true
	n, err = c.Reconcile(context.Background(), writer)
	if err != nil || n != 2 {
		t.Fatal(n, err)
	}
	if targets != 4 || len(writes) != 4 || !writes[2].Revoked || writes[2].Revision != 2 {
		t.Fatal("incremental scan lost revoked binding")
	}
}
