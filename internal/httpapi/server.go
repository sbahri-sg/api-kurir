package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/emisell/api-kurir/internal/admin"
	"github.com/emisell/api-kurir/internal/apikeys"
	"github.com/emisell/api-kurir/internal/couriers"
	"github.com/emisell/api-kurir/internal/locations"
	"github.com/emisell/api-kurir/internal/providercredentials"
	"github.com/emisell/api-kurir/internal/rates"
	"github.com/emisell/api-kurir/internal/tracking"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

type Server struct {
	Echo *echo.Echo
}

func New(
	pool *pgxpool.Pool,
	rateService *rates.Service,
	locationRepository locations.Repository,
	courierRepository couriers.Repository,
	adminRepository admin.Repository,
	trackingService *tracking.Service,
	customerAPIKeyService *apikeys.Service,
	providerCredentialService *providercredentials.Service,
	apiKeys []string,
	adminAPIKeys []string,
	logger *slog.Logger,
	appEnv string,
) *Server {
	e := echo.New()
	e.Use(middleware.Recover())
	e.Use(securityHeadersMiddleware(appEnv))
	e.Use(requestIDMiddleware())
	e.Use(requestLogMiddleware(logger))
	if appEnv != "production" {
		e.Use(developmentCORSMiddleware())
	}
	e.HTTPErrorHandler = func(c *echo.Context, err error) {
		if response, _ := echo.UnwrapResponse(c.Response()); response != nil && response.Committed {
			return
		}

		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		message := "Terjadi kesalahan internal."
		var statusError interface{ StatusCode() int }
		if errors.As(err, &statusError) {
			status = statusError.StatusCode()
			code = "HTTP_ERROR"
			message = http.StatusText(status)
			switch status {
			case http.StatusNotFound:
				code = "NOT_FOUND"
				message = "Endpoint tidak ditemukan."
			case http.StatusMethodNotAllowed:
				code = "METHOD_NOT_ALLOWED"
				message = "Metode HTTP tidak didukung."
			}
		}

		if status >= http.StatusInternalServerError {
			logger.Error("unhandled HTTP error", "error", err, "request_id", requestID(c))
		}
		_ = writeError(c, status, code, message, nil)
	}

	e.GET("/health/live", func(c *echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	e.GET("/health/ready", readinessHandler(pool))

	v1 := e.Group("/v1")
	v1.Use(customerAPIKeyMiddleware(apiKeys, customerAPIKeyService))
	v1.GET("/destination/domestic-destination", locationSearchHandler(locationRepository))
	v1.GET("/couriers", courierListHandler(courierRepository))
	v1.POST("/calculate/domestic-cost", calculateRateHandler(rateService))
	v1.POST("/track/waybill", trackingHandler(trackingService))

	adminGroup := e.Group("/v1/admin")
	adminGroup.Use(apiKeyMiddleware(adminAPIKeys))
	adminGroup.GET("/overview", adminOverviewHandler(adminRepository))
	adminGroup.GET("/catalog", adminCatalogHandler(adminRepository))
	adminGroup.GET("/locations", locationSearchHandler(locationRepository))
	adminGroup.GET("/couriers", courierListHandler(courierRepository))
	adminGroup.POST("/calculate/domestic-cost", calculateRateHandler(rateService))
	adminGroup.POST("/track/waybill", trackingHandler(trackingService))
	adminGroup.GET("/rate-snapshots", adminRateSnapshotListHandler(adminRepository))
	adminGroup.GET("/location-mappings", adminLocationMappingListHandler(adminRepository))
	adminGroup.GET("/provider-quotas", adminProviderQuotaListHandler(adminRepository))
	adminGroup.GET("/api-keys", adminAPIKeyListHandler(customerAPIKeyService))
	adminGroup.POST("/api-keys", adminAPIKeyCreateHandler(customerAPIKeyService))
	adminGroup.POST("/api-keys/:id/revoke", adminAPIKeyRevokeHandler(customerAPIKeyService))
	adminGroup.GET(
		"/provider-credentials",
		adminProviderCredentialListHandler(providerCredentialService),
	)
	adminGroup.POST(
		"/provider-credentials",
		adminProviderCredentialCreateHandler(providerCredentialService),
	)
	adminGroup.POST(
		"/provider-credentials/:id/disable",
		adminProviderCredentialDisableHandler(providerCredentialService),
	)

	return &Server{Echo: e}
}

func securityHeadersMiddleware(appEnv string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			headers := c.Response().Header()
			headers.Set("X-Content-Type-Options", "nosniff")
			headers.Set("X-Frame-Options", "DENY")
			headers.Set("Referrer-Policy", "no-referrer")
			headers.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			headers.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
			if appEnv == "production" {
				headers.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			return next(c)
		}
	}
}

func requestIDMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			id := strings.TrimSpace(c.Request().Header.Get("X-Request-Id"))
			if !validRequestID.MatchString(id) {
				id = newRequestID()
			}
			c.Set("request_id", id)
			c.Response().Header().Set("X-Request-Id", id)
			return next(c)
		}
	}
}

func requestLogMiddleware(logger *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			startedAt := time.Now()
			err := next(c)
			logger.Info(
				"http request",
				"request_id", requestID(c),
				"method", c.Request().Method,
				"path", c.Request().URL.Path,
				"duration_ms", time.Since(startedAt).Milliseconds(),
			)
			return err
		}
	}
}

func apiKeyMiddleware(keys []string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			token := bearerToken(c.Request().Header.Get("Authorization"))
			if token == "" || !matchesAnyKey(token, keys) {
				return writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "API key tidak valid.", nil)
			}
			return next(c)
		}
	}
}

type customerKeyAuthenticator interface {
	Authenticate(ctx context.Context, token string) (bool, error)
}

func customerAPIKeyMiddleware(
	staticKeys []string,
	authenticator customerKeyAuthenticator,
) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			token := bearerToken(c.Request().Header.Get("Authorization"))
			if token == "" {
				return writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "API key tidak valid.", nil)
			}
			if matchesAnyKey(token, staticKeys) {
				return next(c)
			}
			if authenticator == nil {
				return writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "API key tidak valid.", nil)
			}
			valid, err := authenticator.Authenticate(c.Request().Context(), token)
			if err != nil {
				return err
			}
			if !valid {
				return writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "API key tidak valid.", nil)
			}
			return next(c)
		}
	}
}

func bearerToken(authorization string) string {
	parts := strings.Fields(authorization)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func developmentCORSMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			origin := c.Request().Header.Get("Origin")
			if origin == "http://localhost:5173" || origin == "http://127.0.0.1:5173" {
				headers := c.Response().Header()
				headers.Set("Access-Control-Allow-Origin", origin)
				headers.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-Id, X-Admin-Actor")
				headers.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				headers.Set("Vary", "Origin")
			}
			if c.Request().Method == http.MethodOptions {
				return c.NoContent(http.StatusNoContent)
			}
			return next(c)
		}
	}
}

func readinessHandler(pool *pgxpool.Pool) echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx, cancel := timeBoundContext(c.Request().Context(), time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
	}
}

func matchesAnyKey(token string, keys []string) bool {
	for _, key := range keys {
		if len(token) == len(key) && subtle.ConstantTimeCompare([]byte(token), []byte(key)) == 1 {
			return true
		}
	}
	return false
}

func newRequestID() string {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return "req_fallback"
	}
	return "req_" + hex.EncodeToString(value)
}

func requestID(c *echo.Context) string {
	value, _ := c.Get("request_id").(string)
	return value
}

func writeError(
	c *echo.Context,
	status int,
	code string,
	message string,
	details map[string]any,
) error {
	return c.JSON(status, map[string]any{
		"error": map[string]any{
			"code":       code,
			"message":    message,
			"request_id": requestID(c),
			"details":    details,
		},
	})
}
