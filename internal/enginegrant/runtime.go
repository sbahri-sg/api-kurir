package enginegrant

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ActiveCredentialSelector interface {
	ActiveCredentialID(context.Context, string, string) (string, error)
}

// SelectedCredentialVerifier uses existing ownership/validation/selection rules.
// It never decrypts a secret or falls back to the built-in credential pool.
type SelectedCredentialVerifier struct{ Selector ActiveCredentialSelector }

func (v SelectedCredentialVerifier) VerifyCredential(ctx context.Context, merchant, provider, credential string) error {
	if v.Selector == nil {
		return ErrUnavailable
	}
	selected, err := v.Selector.ActiveCredentialID(ctx, merchant, provider)
	if err != nil {
		return ErrUnavailable
	}
	if selected == "" || selected != credential {
		return ErrDenied
	}
	return nil
}

// LoadRuntime is disabled when path is empty. Enabling requires a private,
// server-owned file and explicit merchant enrollment. No DB writes occur here.
func LoadRuntime(path string, pool *pgxpool.Pool, selector ActiveCredentialSelector) (*ProviderGate, error) {
	if path == "" {
		return nil, nil
	}
	if !filepath.IsAbs(path) {
		return nil, ErrUnavailable
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, ErrUnavailable
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Size() > 16384 {
		return nil, ErrUnavailable
	}
	var cfg struct {
		PlatformURL, EngineKey string
		Providers              map[string]ProviderPolicy
	}
	d := json.NewDecoder(io.LimitReader(f, 16385))
	d.DisallowUnknownFields()
	if d.Decode(&cfg) != nil || d.Decode(&struct{}{}) != io.EOF || pool == nil || selector == nil {
		return nil, ErrUnavailable
	}
	// Initial rate integration is BYOK only; built-in never enters this path.
	for _, policy := range cfg.Providers {
		if !policy.RequiresCredential {
			return nil, errors.New("external provider requires credential verification")
		}
	}
	client, err := NewProviderClient(cfg.PlatformURL, cfg.EngineKey, false)
	if err != nil {
		return nil, err
	}
	return NewProviderGate(cfg.Providers, PostgresBindings{Pool: pool}, client, SelectedCredentialVerifier{selector})
}
