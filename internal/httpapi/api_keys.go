package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/emisell/api-kurir/internal/apikeys"
	"github.com/labstack/echo/v5"
)

func adminAPIKeyListHandler(service *apikeys.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		limit, offset, err := adminPagination(c)
		if err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		result, err := service.List(c.Request().Context(), limit, offset)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, adminListResponse(c, result, limit, offset))
	}
}

func adminAPIKeyCreateHandler(service *apikeys.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		result, err := service.Generate(
			c.Request().Context(),
			adminActor(c),
			requestID(c),
		)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusCreated, adminResponse(c, result))
	}
}

func adminAPIKeyRevokeHandler(service *apikeys.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		id := strings.TrimSpace(c.Param("id"))
		if !validUUID.MatchString(id) {
			return writeError(
				c,
				http.StatusBadRequest,
				"INVALID_REQUEST",
				"ID API key tidak valid.",
				nil,
			)
		}
		err := service.Revoke(
			c.Request().Context(),
			id,
			adminActor(c),
			requestID(c),
		)
		if errors.Is(err, apikeys.ErrNotFound) {
			return writeError(
				c,
				http.StatusNotFound,
				"API_KEY_NOT_FOUND",
				"API key tidak ditemukan atau sudah dinonaktifkan.",
				nil,
			)
		}
		if err != nil {
			return err
		}
		return c.NoContent(http.StatusNoContent)
	}
}
