package tenancy

import "context"

type contextKey struct{}

// Identity is the trusted merchant context carried from Emisell to API Kurir.
// TenantID is the stable Emisell merchant ID. IntegrationID optionally pins a
// request to one provider credential; DomainID is routing metadata only.
type Identity struct {
	TenantID      string
	IntegrationID string
	DomainID      string
	Scopes        []string
	TokenID       string
}

func WithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, contextKey{}, identity)
}

func FromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(contextKey{}).(Identity)
	return identity, ok && identity.TenantID != ""
}

func TenantID(ctx context.Context) string {
	identity, _ := FromContext(ctx)
	return identity.TenantID
}

func IntegrationID(ctx context.Context) string {
	identity, _ := FromContext(ctx)
	return identity.IntegrationID
}

func HasScope(identity Identity, required string) bool {
	for _, scope := range identity.Scopes {
		if scope == "*" || scope == required {
			return true
		}
	}
	return false
}
