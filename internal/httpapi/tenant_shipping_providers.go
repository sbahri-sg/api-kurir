package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/emisell/api-kurir/internal/merchantproviders"
	"github.com/emisell/api-kurir/internal/tenancy"
	"github.com/labstack/echo/v5"
)

type changeTenantShippingProviderRequest struct {
	ExpectedVersion *int64 `json:"expected_version"`
}

func tenantShippingProviderCatalogHandler(
	service *merchantproviders.Service,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, _ := tenancy.FromContext(c.Request().Context())
		result, err := service.Catalog(c.Request().Context(), identity.TenantID)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, adminResponse(c, result))
	}
}

func tenantShippingProviderActivateHandler(
	service *merchantproviders.Service,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, _ := tenancy.FromContext(c.Request().Context())
		var request changeTenantShippingProviderRequest
		if err := decodeOptionalJSON(c, &request); err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		result, err := service.Activate(
			c.Request().Context(),
			identity.TenantID,
			strings.TrimSpace(c.Param("provider_code")),
			merchantproviders.ChangeInput{
				ExpectedVersion: request.ExpectedVersion,
				UpdatedBy:       "tenant:" + identity.TenantID,
			},
		)
		if response := writeTenantShippingProviderError(c, err); response != nil {
			return response
		}
		return c.JSON(http.StatusOK, adminResponse(c, result))
	}
}

func tenantShippingProviderDeactivateHandler(
	service *merchantproviders.Service,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, _ := tenancy.FromContext(c.Request().Context())
		var request changeTenantShippingProviderRequest
		if err := decodeOptionalJSON(c, &request); err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		result, err := service.Deactivate(
			c.Request().Context(),
			identity.TenantID,
			strings.TrimSpace(c.Param("provider_code")),
			merchantproviders.ChangeInput{
				ExpectedVersion: request.ExpectedVersion,
				UpdatedBy:       "tenant:" + identity.TenantID,
			},
		)
		if response := writeTenantShippingProviderError(c, err); response != nil {
			return response
		}
		return c.JSON(http.StatusOK, adminResponse(c, result))
	}
}

func writeTenantShippingProviderError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, merchantproviders.ErrInvalidProvider),
		errors.Is(err, merchantproviders.ErrInvalidCredential):
		return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
	case errors.Is(err, merchantproviders.ErrProviderNotFound):
		return writeError(c, http.StatusNotFound, "SHIPPING_PROVIDER_NOT_FOUND", "Provider pengiriman tidak ditemukan.", nil)
	case errors.Is(err, merchantproviders.ErrProviderUnavailable):
		return writeError(c, http.StatusConflict, "SHIPPING_PROVIDER_UNAVAILABLE", "Provider pengiriman belum tersedia untuk diaktifkan.", nil)
	case errors.Is(err, merchantproviders.ErrCredentialRequired):
		return writeError(c, http.StatusUnprocessableEntity, "PROVIDER_CREDENTIAL_REQUIRED", "Credential provider wajib dipilih.", nil)
	case errors.Is(err, merchantproviders.ErrCredentialUnavailable):
		return writeError(c, http.StatusUnprocessableEntity, "PROVIDER_CREDENTIAL_UNAVAILABLE", "Credential tidak valid, tidak aktif, atau bukan milik merchant.", nil)
	case errors.Is(err, merchantproviders.ErrVersionConflict):
		return writeError(c, http.StatusConflict, "SHIPPING_PROVIDER_VERSION_CONFLICT", "Pilihan provider telah berubah. Muat ulang lalu coba kembali.", nil)
	case err != nil:
		return err
	default:
		return nil
	}
}

func decodeOptionalJSON(c *echo.Context, target any) error {
	if c.Request().Body == nil || c.Request().ContentLength == 0 {
		return nil
	}
	return decodeAdminJSON(c, target)
}
