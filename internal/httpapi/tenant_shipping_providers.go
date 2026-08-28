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

type tenantShippingProviderSummary struct {
	Code               string `json:"code"`
	Name               string `json:"name"`
	Logo               string `json:"logo"`
	Description        string `json:"description"`
	BuiltIn            bool   `json:"built_in"`
	IntegrationType    string `json:"integration_type"`
	DistributionType   string `json:"distribution_type"`
	RequiresCredential bool   `json:"requires_credential"`
	Available          bool   `json:"available"`
	Installed          bool   `json:"installed"`
	Active             bool   `json:"active"`
}

type tenantShippingProviderCatalogResponse struct {
	ActiveProviderCode *string                         `json:"active_provider_code"`
	Version            int64                           `json:"version"`
	Providers          []tenantShippingProviderSummary `json:"providers"`
}

func tenantShippingProviderCatalog(
	catalog merchantproviders.Catalog,
) tenantShippingProviderCatalogResponse {
	result := tenantShippingProviderCatalogResponse{
		ActiveProviderCode: catalog.ActiveProviderCode,
		Version:            catalog.Version,
		Providers:          make([]tenantShippingProviderSummary, 0, len(catalog.Providers)),
	}
	for _, provider := range catalog.Providers {
		result.Providers = append(result.Providers, tenantShippingProviderSummary{
			Code:               provider.Code,
			Name:               provider.Name,
			Logo:               provider.Logo,
			Description:        provider.Description,
			BuiltIn:            provider.BuiltIn,
			IntegrationType:    provider.IntegrationType,
			DistributionType:   provider.DistributionType,
			RequiresCredential: provider.RequiresCredential,
			Available:          provider.Available,
			Installed:          provider.Installed,
			Active:             provider.Active,
		})
	}
	return result
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
		c.Response().Header().Set("Cache-Control", "no-store")
		return c.JSON(http.StatusOK, adminResponse(c, tenantShippingProviderCatalog(result)))
	}
}

func tenantShippingProviderDetailHandler(
	service *merchantproviders.Service,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, _ := tenancy.FromContext(c.Request().Context())
		result, err := service.Provider(
			c.Request().Context(),
			identity.TenantID,
			strings.TrimSpace(c.Param("provider_code")),
		)
		if response := writeTenantShippingProviderError(c, err); response != nil {
			return response
		}
		c.Response().Header().Set("Cache-Control", "no-store")
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
		return c.JSON(http.StatusOK, adminResponse(c, tenantShippingProviderCatalog(result)))
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
		return c.JSON(http.StatusOK, adminResponse(c, tenantShippingProviderCatalog(result)))
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
	case errors.Is(err, merchantproviders.ErrReleaseUnavailable):
		return writeError(c, http.StatusConflict, "SHIPPING_PROVIDER_RELEASE_UNAVAILABLE", "Provider partner belum memiliki release yang dipublikasikan.", nil)
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
