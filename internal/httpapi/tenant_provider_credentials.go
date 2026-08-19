package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/emisell/api-kurir/internal/providercredentials"
	"github.com/emisell/api-kurir/internal/tenancy"
	"github.com/labstack/echo/v5"
)

func tenantProviderCredentialListHandler(
	service *providercredentials.Service,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, _ := tenancy.FromContext(c.Request().Context())
		result, err := service.ListForTenant(c.Request().Context(), identity.TenantID)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, adminResponse(c, result))
	}
}

func tenantProviderCredentialCreateHandler(
	service *providercredentials.Service,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, _ := tenancy.FromContext(c.Request().Context())
		var request createProviderCredentialRequest
		if err := decodeAdminJSON(c, &request); err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		result, err := service.AddForTenant(
			c.Request().Context(),
			identity.TenantID,
			request.ProviderCode,
			request.APIKey,
			request.DailyLimit,
			"tenant:"+identity.TenantID,
			requestID(c),
		)
		if response := writeTenantProviderCredentialError(c, err); response != nil {
			return response
		}
		return c.JSON(http.StatusCreated, adminResponse(c, result))
	}
}

func tenantProviderCredentialDisableHandler(
	service *providercredentials.Service,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, _ := tenancy.FromContext(c.Request().Context())
		id := strings.TrimSpace(c.Param("id"))
		if !validUUID.MatchString(id) {
			return writeError(
				c,
				http.StatusBadRequest,
				"INVALID_REQUEST",
				"ID credential provider tidak valid.",
				nil,
			)
		}
		err := service.DisableForTenant(
			c.Request().Context(),
			identity.TenantID,
			id,
			"tenant:"+identity.TenantID,
			requestID(c),
		)
		if errors.Is(err, providercredentials.ErrNotFound) {
			return writeError(
				c,
				http.StatusNotFound,
				"PROVIDER_CREDENTIAL_NOT_FOUND",
				"Credential tidak ditemukan, bukan milik merchant, atau sudah dinonaktifkan.",
				nil,
			)
		}
		if err != nil {
			return err
		}
		return c.NoContent(http.StatusNoContent)
	}
}

func writeTenantProviderCredentialError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, providercredentials.ErrUnsupportedProvider):
		return writeError(c, http.StatusBadRequest, "UNSUPPORTED_PROVIDER", "Provider belum didukung.", nil)
	case errors.Is(err, providercredentials.ErrInvalidSecret):
		return writeError(
			c,
			http.StatusUnprocessableEntity,
			"INVALID_PROVIDER_KEY",
			"API key ditolak oleh provider atau formatnya tidak valid.",
			nil,
		)
	case errors.Is(err, providercredentials.ErrInvalidDailyLimit):
		return writeError(
			c,
			http.StatusBadRequest,
			"INVALID_DAILY_LIMIT",
			"daily_limit harus antara 1 dan 100.000.000 hit.",
			nil,
		)
	case errors.Is(err, providercredentials.ErrDuplicate):
		return writeError(
			c,
			http.StatusConflict,
			"PROVIDER_KEY_EXISTS",
			"API key provider ini sudah pernah disimpan.",
			nil,
		)
	case err != nil:
		return err
	default:
		return nil
	}
}
