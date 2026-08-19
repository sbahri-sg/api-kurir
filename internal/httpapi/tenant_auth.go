package httpapi

import (
	"net/http"
	"strings"

	"github.com/emisell/api-kurir/internal/tenancy"
	"github.com/labstack/echo/v5"
)

const tenantContextHeader = "X-Emisell-Tenant-Token"

func tenantContextMiddleware(
	verifier *tenancy.Verifier,
	required bool,
) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			token := strings.TrimSpace(c.Request().Header.Get(tenantContextHeader))
			if token == "" {
				if required {
					return writeError(
						c,
						http.StatusUnauthorized,
						"TENANT_CONTEXT_REQUIRED",
						"Konteks merchant Emisell wajib dikirim.",
						nil,
					)
				}
				return next(c)
			}
			if verifier == nil {
				return writeError(
					c,
					http.StatusServiceUnavailable,
					"TENANT_AUTH_NOT_CONFIGURED",
					"Verifikasi konteks merchant belum dikonfigurasi.",
					nil,
				)
			}
			identity, err := verifier.Verify(token)
			if err != nil {
				return writeError(
					c,
					http.StatusUnauthorized,
					"INVALID_TENANT_CONTEXT",
					"Konteks merchant tidak valid atau sudah kedaluwarsa.",
					nil,
				)
			}
			request := c.Request().WithContext(
				tenancy.WithIdentity(c.Request().Context(), identity),
			)
			c.SetRequest(request)
			return next(c)
		}
	}
}

func tenantScopeMiddleware(required string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			identity, ok := tenancy.FromContext(c.Request().Context())
			if !ok {
				return writeError(
					c,
					http.StatusUnauthorized,
					"TENANT_CONTEXT_REQUIRED",
					"Konteks merchant Emisell wajib dikirim.",
					nil,
				)
			}
			if !tenancy.HasScope(identity, required) {
				return writeError(
					c,
					http.StatusForbidden,
					"INSUFFICIENT_TENANT_SCOPE",
					"Konteks merchant tidak memiliki scope yang diperlukan.",
					map[string]any{"required_scope": required},
				)
			}
			return next(c)
		}
	}
}

func requireTenantScopeIfPresent(c *echo.Context, required string) error {
	identity, ok := tenancy.FromContext(c.Request().Context())
	if !ok || tenancy.HasScope(identity, required) {
		return nil
	}
	return writeError(
		c,
		http.StatusForbidden,
		"INSUFFICIENT_TENANT_SCOPE",
		"Konteks merchant tidak memiliki scope yang diperlukan.",
		map[string]any{"required_scope": required},
	)
}
