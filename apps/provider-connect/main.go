// Operator-only connection tool; no browser route or secret transfer.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/emisell/api-kurir/internal/enginegrant"
	"github.com/emisell/api-kurir/internal/providercredentials"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(); err != nil {
		// Do not expose connection strings or repository errors on the terminal.
		fmt.Fprintln(os.Stderr, "Credential binding failed; check enrollment, grant and selected credential.")
		os.Exit(1)
	}
	fmt.Println("Credential reference connected; no provider secret was copied.")
}

func run() error {
	config := flag.String("config", "", "private runtime config file")
	m := flag.String("merchant", "", "authenticated operator target merchant")
	p := flag.String("provider", "", "external provider")
	a := flag.String("app", "", "approved app ID")
	i := flag.String("installation", "", "active installation ID")
	c := flag.String("credential", "", "existing selected credential ID (not API key)")
	flag.Parse()
	if *config == "" || os.Getenv("DATABASE_URL") == "" {
		return enginegrant.ErrDenied
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	// ActiveCredentialID uses only repository metadata; no cipher is needed.
	selector := providercredentials.NewService(providercredentials.NewPostgresRepository(pool), nil, nil)
	gate, err := enginegrant.LoadRuntime(*config, pool, selector)
	if err != nil {
		return err
	}
	return gate.ConnectCredential(ctx, enginegrant.ProviderBinding{MerchantID: *m, ProviderCode: *p, AppID: *a, InstallationID: *i}, *c, enginegrant.PostgresBindings{Pool: pool})
}
