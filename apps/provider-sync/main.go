// Explicit reconciliation tool. No auto-migration, provider activation or secret transfer.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/emisell/api-kurir/internal/enginegrant"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "provider binding synchronization failed:", err)
		os.Exit(1)
	}
}
func run() error {
	path := flag.String("config", "", "private configuration file")
	m := flag.String("merchant", "", "merchant ID")
	p := flag.String("provider", "", "provider code")
	a := flag.String("app", "", "app ID")
	i := flag.String("installation", "", "installation ID")
	watch := flag.Bool("watch", false, "periodically reconcile all enrolled installations")
	interval := flag.Duration("interval", 30*time.Second, "reconciliation interval, minimum 15s")
	flag.Parse()
	if *watch && (*m != "" || *p != "" || *a != "" || *i != "" || *interval < 15*time.Second) {
		return errors.New("watch requires no manual target and interval >=15s")
	}
	fd, err := syscall.Open(*path, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return errors.New("private config unavailable")
	}
	f := os.NewFile(uintptr(fd), *path)
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Size() > 8192 {
		return errors.New("invalid private config")
	}
	var cfg struct {
		DatabaseURL, PlatformURL, EngineKey string
		AllowLoopback                       bool
	}
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	if d.Decode(&cfg) != nil || d.Decode(&struct{}{}) != io.EOF {
		return errors.New("invalid config")
	}
	client, err := enginegrant.NewProviderClient(cfg.PlatformURL, cfg.EngineKey, cfg.AllowLoopback)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return errors.New("database configuration invalid")
	}
	defer pool.Close()
	if *watch {
		for {
			cycle, stop := context.WithTimeout(ctx, 5*time.Minute)
			n, err := client.Reconcile(cycle, enginegrant.PostgresBindings{Pool: pool})
			stop()
			if ctx.Err() != nil {
				return nil
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, "reconciliation incomplete; will retry (no authorization fallback)")
			} else {
				fmt.Printf("Reconciled %d provider bindings.\n", n)
			}
			timer := time.NewTimer(*interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
		}
	}
	ctx, stop := context.WithTimeout(ctx, 10*time.Second)
	defer stop()
	if err = client.Sync(ctx, enginegrant.ProviderBinding{MerchantID: *m, ProviderCode: *p, AppID: *a, InstallationID: *i}, enginegrant.PostgresBindings{Pool: pool}); err != nil {
		return errors.New("authority or binding storage rejected synchronization")
	}
	fmt.Println("Provider installation binding synchronized; credentials and provider activation unchanged.")
	return nil
}
