package httpapi

import (
	"errors"
	"net/http"

	"github.com/emisell/api-kurir/internal/webhooksettings"
	"github.com/labstack/echo/v5"
)

type updateTrackingWebhookRequest struct {
	CallbackURL string `json:"callback_url"`
	Enabled     bool   `json:"enabled"`
}

func adminTrackingWebhookGetHandler(
	service *webhooksettings.Service,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		result, err := service.Get(c.Request().Context())
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, adminResponse(c, result))
	}
}

func adminTrackingWebhookUpdateHandler(
	service *webhooksettings.Service,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		var request updateTrackingWebhookRequest
		if err := decodeAdminJSON(c, &request); err != nil {
			return writeError(
				c,
				http.StatusBadRequest,
				"INVALID_REQUEST",
				err.Error(),
				nil,
			)
		}
		result, err := service.Update(
			c.Request().Context(),
			request.CallbackURL,
			request.Enabled,
			adminActor(c),
		)
		if response := trackingWebhookSettingsError(c, err); response != nil {
			return response
		}
		return c.JSON(http.StatusOK, adminResponse(c, result))
	}
}

func adminTrackingWebhookGenerateSecretHandler(
	service *webhooksettings.Service,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		result, err := service.GenerateSecret(
			c.Request().Context(),
			adminActor(c),
		)
		if response := trackingWebhookSettingsError(c, err); response != nil {
			return response
		}
		return c.JSON(http.StatusCreated, adminResponse(c, result))
	}
}

func adminTrackingWebhookTestHandler(
	service *webhooksettings.Service,
) echo.HandlerFunc {
	return func(c *echo.Context) error {
		result, err := service.Test(
			c.Request().Context(),
			adminActor(c),
		)
		if response := trackingWebhookSettingsError(c, err); response != nil {
			return response
		}
		return c.JSON(http.StatusOK, adminResponse(c, result))
	}
}

func trackingWebhookSettingsError(c *echo.Context, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, webhooksettings.ErrInvalidURL):
		return writeError(
			c,
			http.StatusUnprocessableEntity,
			"INVALID_WEBHOOK_URL",
			"URL webhook tidak valid. Production wajib menggunakan HTTPS publik.",
			nil,
		)
	case errors.Is(err, webhooksettings.ErrSecretNotConfigured):
		return writeError(
			c,
			http.StatusConflict,
			"WEBHOOK_SECRET_REQUIRED",
			"Generate secret webhook sebelum mengaktifkan pengiriman.",
			nil,
		)
	case errors.Is(err, webhooksettings.ErrNotConfigured):
		return writeError(
			c,
			http.StatusConflict,
			"WEBHOOK_NOT_CONFIGURED",
			"Lengkapi URL dan secret webhook terlebih dahulu.",
			nil,
		)
	default:
		return err
	}
}
