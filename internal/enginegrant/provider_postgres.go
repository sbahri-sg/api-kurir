package enginegrant

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresBindings struct{ Pool *pgxpool.Pool }

// Credentials are intentionally not written by synchronization.
func (p PostgresBindings) ApplySnapshot(ctx context.Context, v ProviderSnapshot) error {
	if !identifier.MatchString(v.MerchantID) || !identifier.MatchString(v.AppID) || !identifier.MatchString(v.InstallationID) || !providerCode.MatchString(v.ProviderCode) || v.Revision < 1 || (v.Active && v.Revoked) {
		return ErrDenied
	}
	tag, err := p.Pool.Exec(ctx, `INSERT INTO app_platform_provider_bindings(merchant_id,provider_code,installation_id,app_id,revision,active,revoked)
 VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(merchant_id,provider_code,installation_id) DO UPDATE SET
 revision=EXCLUDED.revision, active=EXCLUDED.active, revoked=EXCLUDED.revoked
 WHERE app_platform_provider_bindings.app_id=EXCLUDED.app_id
 AND app_platform_provider_bindings.revision<EXCLUDED.revision
 AND (NOT app_platform_provider_bindings.revoked OR EXCLUDED.revoked)`, v.MerchantID, v.ProviderCode, v.InstallationID, v.AppID, v.Revision, v.Active, v.Revoked)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	// Equal revision retries must be identical; stale events and identity changes fail closed.
	var same bool
	err = p.Pool.QueryRow(ctx, `SELECT app_id=$4 AND revision=$5 AND active=$6 AND revoked=$7 FROM app_platform_provider_bindings WHERE merchant_id=$1 AND provider_code=$2 AND installation_id=$3`, v.MerchantID, v.ProviderCode, v.InstallationID, v.AppID, v.Revision, v.Active, v.Revoked).Scan(&same)
	if err != nil {
		return err
	}
	if !same {
		return ErrDenied
	}
	return nil
}
func (p PostgresBindings) Resolve(ctx context.Context, merchant, provider string) (ProviderBinding, error) {
	rows, err := p.Pool.Query(ctx, `SELECT merchant_id,provider_code,app_id,installation_id,credential_id,active FROM app_platform_provider_bindings WHERE merchant_id=$1 AND provider_code=$2 AND active AND NOT revoked LIMIT 2`, merchant, provider)
	if err != nil {
		return ProviderBinding{}, err
	}
	defer rows.Close()
	var b ProviderBinding
	n := 0
	for rows.Next() {
		n++
		if err = rows.Scan(&b.MerchantID, &b.ProviderCode, &b.AppID, &b.InstallationID, &b.CredentialID, &b.Active); err != nil {
			return b, err
		}
	}
	if err = rows.Err(); err != nil {
		return b, err
	}
	if n != 1 {
		return ProviderBinding{}, ErrBindingNotFound
	}
	return b, nil
}
