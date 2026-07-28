package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/emisell/api-kurir/internal/providercredentials"
	"github.com/labstack/echo/v5"
)

type createProviderCredentialRequest struct {
	ProviderCode string `json:"provider_code"`
	APIKey       string `json:"api_key"`
}

func adminProviderCredentialListHandler(
	service *providercredentials.Service,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		result, err := service.List(c.Request().Context())
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, adminResponse(c, result))
	}
}

func adminProviderCredentialCreateHandler(
	service *providercredentials.Service,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		var request createProviderCredentialRequest
		if err := decodeAdminJSON(c, &request); err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		result, err := service.Add(
			c.Request().Context(),
			request.ProviderCode,
			request.APIKey,
			adminActor(c),
			requestID(c),
		)
		switch {
		case errors.Is(err, providercredentials.ErrUnsupportedProvider):
			return writeError(
				c,
				http.StatusBadRequest,
				"UNSUPPORTED_PROVIDER",
				"Provider belum didukung untuk credential database.",
				nil,
			)
		case errors.Is(err, providercredentials.ErrInvalidSecret):
			return writeError(
				c,
				http.StatusUnprocessableEntity,
				"INVALID_PROVIDER_KEY",
				"API key ditolak oleh provider atau formatnya tidak valid.",
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
		}
		return c.JSON(http.StatusCreated, adminResponse(c, result))
	}
}

func adminProviderCredentialDisableHandler(
	service *providercredentials.Service,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
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
		err := service.Disable(
			c.Request().Context(),
			id,
			adminActor(c),
			requestID(c),
		)
		if errors.Is(err, providercredentials.ErrNotFound) {
			return writeError(
				c,
				http.StatusNotFound,
				"PROVIDER_CREDENTIAL_NOT_FOUND",
				"Credential tidak ditemukan atau sudah dinonaktifkan.",
				nil,
			)
		}
		if err != nil {
			return err
		}
		return c.NoContent(http.StatusNoContent)
	}
}
