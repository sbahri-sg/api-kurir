package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/emisell/api-kurir/internal/enginegrant"
	"github.com/emisell/api-kurir/internal/tenancy"
	"github.com/labstack/echo/v5"
)

type appCredentialConnector func(context.Context, enginegrant.ProviderBinding, string) error

// WithAppCredentialConnection exposes only a service-authenticated reference
// binding operation, when the external provider gate is explicitly enabled.
func WithAppCredentialConnection(gate *enginegrant.ProviderGate, writer enginegrant.CredentialBindingWriter) ServerOption {
	return func(c *serverConfig) {
		if gate != nil && writer != nil {
			c.appCredentialConnector = func(ctx context.Context, b enginegrant.ProviderBinding, credential string) error {
				return gate.ConnectCredential(ctx, b, credential, writer)
			}
		}
	}
}

func appCredentialConnectionHandler(connect appCredentialConnector) echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		merchant := tenancy.TenantID(ctx)
		if merchant == "" || c.Request().Header.Get("Origin") != "" || c.Request().Header.Get("Cookie") != "" {
			return writeError(c, 403, "FORBIDDEN", "Server-to-server merchant context required.", nil)
		}
		var body struct {
			AppID          string `json:"app_id"`
			InstallationID string `json:"installation_id"`
			ProviderCode   string `json:"provider_code"`
			CredentialID   string `json:"credential_id"`
		}
		d := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, 4096))
		d.DisallowUnknownFields()
		if d.Decode(&body) != nil || d.Decode(&struct{}{}) != io.EOF || body.AppID == "" || body.InstallationID == "" || body.ProviderCode == "" || body.CredentialID == "" {
			return writeError(c, 400, "INVALID_REQUEST", "Installation and credential references required.", nil)
		}
		if connect == nil {
			return writeError(c, 503, "UNAVAILABLE", "Connection unavailable.", nil)
		}
		err := connect(ctx, enginegrant.ProviderBinding{MerchantID: merchant, ProviderCode: body.ProviderCode, AppID: body.AppID, InstallationID: body.InstallationID}, body.CredentialID)
		if errors.Is(err, enginegrant.ErrDenied) || errors.Is(err, enginegrant.ErrBindingNotFound) {
			return writeError(c, 403, "APP_GRANT_REQUIRED", "Installation or credential is not authorized.", nil)
		}
		if err != nil {
			return writeError(c, 503, "CONNECTION_UNAVAILABLE", "Connection could not be verified.", nil)
		}
		return c.JSON(200, map[string]string{"status": "connected"})
	}
}
