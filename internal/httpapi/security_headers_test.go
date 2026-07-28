package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestSecurityHeadersMiddleware(t *testing.T) {
	t.Parallel()

	e := echo.New()
	e.Use(securityHeadersMiddleware("production"))
	e.GET("/health", func(c *echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)

	for header, expected := range map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"Referrer-Policy":           "no-referrer",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
	} {
		if actual := response.Header().Get(header); actual != expected {
			t.Fatalf("%s: got %q want %q", header, actual, expected)
		}
	}
}
