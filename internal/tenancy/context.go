package tenancy

import "context"

type contextKey struct{}

// Identity is the trusted merchant context carried from Emisell to API Kurir.
// TenantID comes from the server-to-server X-Emisell-Merchant-ID header after
// the caller API key has been authenticated. ProviderCredentialID is internal
// only: workers use it to keep an existing shipment pinned to its credential.
type Identity struct {
	TenantID             string
	ProviderCredentialID string
}

func WithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, contextKey{}, identity)
}

// WithoutIdentity keeps the parent context lifecycle while removing the
// merchant scope. Credential resolvers use it only after the merchant's active
// integration has explicitly authorized use of the built-in platform pool.
func WithoutIdentity(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextKey{}, Identity{})
}

func FromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(contextKey{}).(Identity)
	return identity, ok && identity.TenantID != ""
}

func TenantID(ctx context.Context) string {
	identity, _ := FromContext(ctx)
	return identity.TenantID
}

func ProviderCredentialID(ctx context.Context) string {
	identity, _ := FromContext(ctx)
	return identity.ProviderCredentialID
}
