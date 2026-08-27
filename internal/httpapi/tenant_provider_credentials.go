package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/emisell/api-kurir/internal/providercredentials"
	"github.com/emisell/api-kurir/internal/tenancy"
	"github.com/labstack/echo/v5"
)

type tenantProviderCredentialResponse struct {
	ProviderCode     string     `json:"provider_code"`
	Environment      string     `json:"environment"`
	CredentialAlias  string     `json:"credential_alias"`
	DisplayKey       string     `json:"display_key"`
	DailyLimit       int64      `json:"daily_limit"`
	Active           bool       `json:"active"`
	ValidationStatus string     `json:"validation_status"`
	LastValidatedAt  time.Time  `json:"last_validated_at"`
	LastSelectedAt   *time.Time `json:"last_selected_at"`
	CreatedAt        time.Time  `json:"created_at"`
	DisabledAt       *time.Time `json:"disabled_at"`
}

func tenantProviderCredential(item providercredentials.Credential) tenantProviderCredentialResponse {
	return tenantProviderCredentialResponse{
		ProviderCode:     item.ProviderCode,
		Environment:      item.Environment,
		CredentialAlias:  item.CredentialAlias,
		DisplayKey:       item.DisplayKey,
		DailyLimit:       item.DailyLimit,
		Active:           item.Active,
		ValidationStatus: item.ValidationStatus,
		LastValidatedAt:  item.LastValidatedAt,
		LastSelectedAt:   item.LastSelectedAt,
		CreatedAt:        item.CreatedAt,
		DisabledAt:       item.DisabledAt,
	}
}

func tenantProviderCredentialListHandler(
	service *providercredentials.Service,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, _ := tenancy.FromContext(c.Request().Context())
		result, err := service.ListForTenant(c.Request().Context(), identity.TenantID)
		if err != nil {
			return err
		}
		response := make([]tenantProviderCredentialResponse, 0, len(result))
		for _, item := range result {
			response = append(response, tenantProviderCredential(item))
		}
		return c.JSON(http.StatusOK, adminResponse(c, response))
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
		var result providercredentials.Credential
		var err error
		if len(request.Credentials) > 0 {
			if strings.TrimSpace(request.APIKey) != "" {
				return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "Gunakan credentials atau api_key, bukan keduanya.", nil)
			}
			result, err = service.AddForTenantEnvironmentCredentials(
				c.Request().Context(), identity.TenantID, request.ProviderCode,
				request.Environment, request.Credentials, request.DailyLimit,
				"tenant:"+identity.TenantID, requestID(c),
			)
		} else {
			result, err = service.AddForTenantEnvironmentCredentials(
				c.Request().Context(), identity.TenantID, request.ProviderCode,
				request.Environment, map[string]string{"api_key": request.APIKey},
				request.DailyLimit,
				"tenant:"+identity.TenantID, requestID(c),
			)
		}
		if response := writeTenantProviderCredentialError(c, err); response != nil {
			return response
		}
		return c.JSON(
			http.StatusCreated,
			adminResponse(c, tenantProviderCredential(result)),
		)
	}
}

func tenantProviderCredentialDisableHandler(
	service *providercredentials.Service,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, _ := tenancy.FromContext(c.Request().Context())
		providerCode := strings.ToLower(strings.TrimSpace(c.Param("provider_code")))
		if providerCode == "" {
			return writeError(
				c,
				http.StatusBadRequest,
				"INVALID_REQUEST",
				"Kode provider wajib diisi.",
				nil,
			)
		}
		err := service.DisableForTenantProvider(
			c.Request().Context(),
			identity.TenantID,
			providerCode,
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
		if response := writeTenantProviderCredentialError(c, err); response != nil {
			return response
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
	case errors.Is(err, providercredentials.ErrEnvironmentUnavailable):
		return writeError(
			c,
			http.StatusUnprocessableEntity,
			"PROVIDER_ENVIRONMENT_UNAVAILABLE",
			"Mode live/sandbox tidak tersedia untuk credential provider ini.",
			nil,
		)
	case errors.Is(err, providercredentials.ErrDuplicate):
		return writeError(
			c,
			http.StatusConflict,
			"PROVIDER_KEY_EXISTS",
			"API key sudah terikat pada credential lain dan tidak dapat digunakan untuk merchant ini.",
			nil,
		)
	case err != nil:
		return err
	default:
		return nil
	}
}
