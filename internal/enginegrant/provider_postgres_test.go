package enginegrant

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/url"
	"os"
	"testing"
)

func TestBindingPersistence(t *testing.T) {
	dsn := os.Getenv("PROVIDER_GRANT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable database not configured")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Hostname() != "127.0.0.1" || u.Path != "/provider_grant_test" {
		t.Fatal("disposable loopback DB required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	raw, err := os.ReadFile("../../migrations/sql/000077_app_platform_provider_bindings.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(raw)); err != nil {
		t.Fatal(err)
	}
	p := PostgresBindings{Pool: pool}
	v := ProviderSnapshot{MerchantID: "m", ProviderCode: "rajaongkir", AppID: "app", InstallationID: "ins", Revision: 1, Active: true}
	apply := func(want error) {
		t.Helper()
		if err := p.ApplySnapshot(ctx, v); !errors.Is(err, want) {
			t.Fatal(err, want)
		}
	}
	apply(nil)
	apply(nil)
	b, err := p.Resolve(ctx, "m", "rajaongkir")
	if err != nil || b.InstallationID != "ins" {
		t.Fatal(err)
	}
	v.Revision = 2
	v.Active = false
	v.Revoked = true
	apply(nil)
	v.Revision = 1
	v.Active = true
	v.Revoked = false
	apply(ErrDenied)
	v.Revision = 3
	apply(ErrDenied) // revoked installation is terminal even for a newer stale activation.
	if _, err = p.Resolve(ctx, "m", "rajaongkir"); !errors.Is(err, ErrBindingNotFound) {
		t.Fatal(err)
	}
	v.InstallationID = "new-ins"
	apply(nil)
	v.InstallationID = "ins"
	v.Revision = 4
	v.Active = false
	v.Revoked = true
	apply(nil)
	b, err = p.Resolve(ctx, "m", "rajaongkir")
	if err != nil || b.InstallationID != "new-ins" {
		t.Fatal("old uninstall changed new installation", err)
	}
	if _, err = p.Resolve(ctx, "other", "rajaongkir"); !errors.Is(err, ErrBindingNotFound) {
		t.Fatal("merchant leak")
	}
}
