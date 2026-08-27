package httpapi

import (
	"net/http"

	"github.com/emisell/api-kurir/internal/providercredentials"
	"github.com/labstack/echo/v5"
)

const providerExecutionModeHeader = "X-Emisell-Execution-Mode"

func providerExecutionEnvironmentMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			environment, valid := providercredentials.NormalizeEnvironmentStrict(
				c.Request().Header.Get(providerExecutionModeHeader),
			)
			if !valid {
				return writeError(
					c,
					http.StatusBadRequest,
					"INVALID_EXECUTION_MODE",
					"X-Emisell-Execution-Mode hanya menerima live atau sandbox.",
					nil,
				)
			}
			request := c.Request().WithContext(
				providercredentials.WithExecutionEnvironment(c.Request().Context(), environment),
			)
			c.SetRequest(request)
			c.Response().Header().Set(providerExecutionModeHeader, environment)
			return next(c)
		}
	}
}
