package enginegrant

import (
	"context"
	"slices"
)

// CredentialBindingWriter performs an atomic compare-and-set, preserving the
// installation identity and rejecting concurrent revocation/credential changes.
type CredentialBindingWriter interface {
	BindCredential(context.Context, ProviderBinding, string) error
}

// ConnectCredential is an internal backend operation, not a browser endpoint.
// Identity must come from an authenticated operator/backend, never an arbitrary
// merchant header. It links an existing selected credential; it creates no secret.
func (g *ProviderGate) ConnectCredential(ctx context.Context, expected ProviderBinding, credential string, writer CredentialBindingWriter) error {
	if g == nil || writer == nil || g.credentials == nil || !identifier.MatchString(credential) {
		return ErrDenied
	}
	policy, ok := g.policies[expected.ProviderCode]
	if !ok || !policy.RequiresCredential || !slices.Contains(policy.MerchantIDs, expected.MerchantID) {
		return ErrDenied
	}
	b, err := g.bindings.Resolve(ctx, expected.MerchantID, expected.ProviderCode)
	if err != nil {
		return err
	}
	if !b.Active || b.MerchantID != expected.MerchantID || b.ProviderCode != expected.ProviderCode || b.AppID != expected.AppID || b.InstallationID != expected.InstallationID {
		return ErrDenied
	}
	if err := g.grants.Verify(ctx, b, "settings.write"); err != nil {
		return err
	}
	if err := g.credentials.VerifyCredential(ctx, b.MerchantID, b.ProviderCode, credential); err != nil {
		return err
	}
	return writer.BindCredential(ctx, b, credential)
}

func (p PostgresBindings) BindCredential(ctx context.Context, b ProviderBinding, credential string) error {
	if b.ProviderCode == "emisell" || !b.Active || !identifier.MatchString(credential) {
		return ErrDenied
	}
	tag, err := p.Pool.Exec(ctx, `UPDATE app_platform_provider_bindings b SET credential_id=$5
 WHERE merchant_id=$1 AND provider_code=$2 AND app_id=$3 AND installation_id=$4
 AND active AND NOT revoked AND credential_id=$6
 AND EXISTS (SELECT 1 FROM tenant_active_shipping_providers s
 JOIN provider_credentials c ON c.id=s.credential_id
 WHERE s.tenant_id=b.merchant_id AND s.provider_code=b.provider_code
 AND c.id::text=$5 AND c.tenant_id=s.tenant_id AND c.provider_code=s.provider_code
 AND c.active AND c.validation_status='valid')`, b.MerchantID, b.ProviderCode, b.AppID, b.InstallationID, credential, b.CredentialID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrDenied
	}
	return nil
}
