package httpapi

import (
	"net/http"
	"time"

	"github.com/emisell/api-kurir/internal/couriers"
	"github.com/labstack/echo/v5"
)

func courierListHandler(repository couriers.Repository) echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx, cancel := timeBoundContext(c.Request().Context(), 500*time.Millisecond)
		defer cancel()
		result, err := repository.List(ctx)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, map[string]any{"data": result})
	}
}
