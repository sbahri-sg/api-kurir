package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

type stubCustomerKeyAuthenticator struct {
	valid       bool
	scopedValid bool
	err         error
	calls       int
	scopeCalls  int
}

func (s *stubCustomerKeyAuthenticator) Authenticate(context.Context, string) (bool, error) {
	s.calls++
	return s.valid, s.err
}

func (s *stubCustomerKeyAuthenticator) AuthenticateScope(
	context.Context,
	string,
	string,
) (bool, error) {
	s.scopeCalls++
	return s.scopedValid, s.err
}

func TestCustomerAPIKeyMiddlewareAcceptsStaticAndGeneratedKeys(t *testing.T) {
	authenticator := &stubCustomerKeyAuthenticator{valid: true}
	e := echo.New()
	e.Use(customerAPIKeyMiddleware([]string{"static-key"}, authenticator))
	e.GET("/protected", func(c *echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer static-key")
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("static key status: got %d want %d", response.Code, http.StatusNoContent)
	}
	if authenticator.calls != 0 {
		t.Fatalf("static key should bypass database authentication, got %d calls", authenticator.calls)
	}

	request = httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer ek_live_generated")
	response = httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("generated key status: got %d want %d", response.Code, http.StatusNoContent)
	}
	if authenticator.calls != 1 {
		t.Fatalf("generated key should call authenticator once, got %d", authenticator.calls)
	}
}

func TestCustomerAPIKeyMiddlewareRejectsInvalidKey(t *testing.T) {
	authenticator := &stubCustomerKeyAuthenticator{}
	e := echo.New()
	e.Use(customerAPIKeyMiddleware(nil, authenticator))
	e.GET("/protected", func(c *echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer invalid")
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestCustomerAPIKeyMiddlewareAcceptsKeyHeaderWithoutChangingContract(t *testing.T) {
	t.Parallel()

	e := echo.New()
	e.Use(customerAPIKeyMiddleware([]string{"static-key"}, nil))
	e.GET("/protected", func(c *echo.Context) error {
		if rajaOngkirV2Compatibility(c) {
			t.Fatal("authentication header must not change the API contract")
		}
		return c.NoContent(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("key", "static-key")
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf(
			"status: got %d want %d, body=%s",
			response.Code,
			http.StatusNoContent,
			response.Body.String(),
		)
	}
}

func TestRajaOngkirCompatibilityMiddlewareSelectsContractByRoute(t *testing.T) {
	t.Parallel()

	e := echo.New()
	group := e.Group("/api/v1")
	group.Use(rajaOngkirV2CompatibilityMiddleware())
	group.Use(customerAPIKeyMiddleware([]string{"static-key"}, nil))
	group.GET("/protected", func(c *echo.Context) error {
		if !rajaOngkirV2Compatibility(c) {
			t.Fatal("RajaOngkir compatibility mode was not enabled")
		}
		return c.NoContent(http.StatusNoContent)
	})

	for _, headers := range []map[string]string{
		{"key": "static-key"},
		{"Authorization": "Bearer static-key"},
	} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/protected", nil)
		for name, value := range headers {
			request.Header.Set(name, value)
		}
		response := httptest.NewRecorder()
		e.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf(
				"headers=%v status: got %d want %d, body=%s",
				headers,
				response.Code,
				http.StatusNoContent,
				response.Body.String(),
			)
		}
	}
}

func TestCustomerAPIKeyMiddlewarePropagatesRepositoryFailure(t *testing.T) {
	authenticator := &stubCustomerKeyAuthenticator{err: errors.New("database unavailable")}
	e := echo.New()
	e.Use(customerAPIKeyMiddleware(nil, authenticator))
	e.GET("/protected", func(c *echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer ek_live_generated")
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d want %d", response.Code, http.StatusInternalServerError)
	}
}
