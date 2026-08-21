package httpapi

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/emisell/api-kurir/internal/tenancy"
	"github.com/labstack/echo/v5"
)

const merchantIDHeader = "X-Emisell-Merchant-ID"

var validMerchantID = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

func merchantContextMiddleware(required bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			merchantID := strings.TrimSpace(c.Request().Header.Get(merchantIDHeader))
			if merchantID == "" {
				if required {
					return writeError(
						c,
						http.StatusBadRequest,
						"MERCHANT_ID_REQUIRED",
						"Header X-Emisell-Merchant-ID wajib dikirim.",
						nil,
					)
				}
				return next(c)
			}
			serviceCaller, _ := c.Get(serviceAPIKeyContextKey).(bool)
			if !serviceCaller {
				return writeError(
					c,
					http.StatusForbidden,
					"MERCHANT_CONTEXT_FORBIDDEN",
					"Header merchant hanya boleh digunakan oleh Main Service Emisell.",
					nil,
				)
			}
			if !validMerchantID.MatchString(merchantID) {
				return writeError(
					c,
					http.StatusBadRequest,
					"INVALID_MERCHANT_ID",
					"Format merchant ID tidak valid.",
					nil,
				)
			}
			request := c.Request().WithContext(
				tenancy.WithIdentity(c.Request().Context(), tenancy.Identity{
					TenantID: merchantID,
				}),
			)
			c.SetRequest(request)
			return next(c)
		}
	}
}
