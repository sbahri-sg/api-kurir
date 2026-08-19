package httpapi

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/tenancy"
	"github.com/labstack/echo/v5"
)

func TestTenantContextMiddlewareAuthenticatesMerchantAndScope(t *testing.T) {
	t.Parallel()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := tenancy.NewVerifier(
		base64.StdEncoding.EncodeToString(publicKey),
		"emisell-api",
		"api-kurir",
		5*time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	token := signTenantHTTPTestToken(t, privateKey, map[string]any{
		"iss": "emisell-api", "aud": "api-kurir", "sub": "merchant_123",
		"scope": []string{"provider-credentials:read"},
		"iat":   now.Unix(), "exp": now.Add(time.Minute).Unix(),
	})

	e := echo.New()
	e.Use(tenantContextMiddleware(verifier, true))
	e.Use(tenantScopeMiddleware("provider-credentials:read"))
	e.GET("/protected", func(c *echo.Context) error {
		identity, ok := tenancy.FromContext(c.Request().Context())
		if !ok {
			t.Fatal("tenant identity not propagated")
		}
		return c.JSON(http.StatusOK, map[string]string{"tenant_id": identity.TenantID})
	})
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set(tenantContextHeader, token)
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestTenantContextMiddlewareRejectsMissingAndInvalidContext(t *testing.T) {
	t.Parallel()
	e := echo.New()
	e.Use(tenantContextMiddleware(nil, true))
	e.GET("/protected", func(c *echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set(tenantContextHeader, "invalid")
	response = httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured verifier status=%d body=%s", response.Code, response.Body.String())
	}
}

func signTenantHTTPTestToken(
	t *testing.T,
	privateKey ed25519.PrivateKey,
	claims map[string]any,
) string {
	t.Helper()
	header, err := json.Marshal(map[string]string{"alg": "EdDSA", "typ": "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	encodedHeader := base64.RawURLEncoding.EncodeToString(header)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	message := encodedHeader + "." + encodedPayload
	signature := ed25519.Sign(privateKey, []byte(message))
	return message + "." + base64.RawURLEncoding.EncodeToString(signature)
}
