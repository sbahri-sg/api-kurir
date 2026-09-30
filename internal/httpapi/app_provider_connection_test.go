package httpapi

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/emisell/api-kurir/internal/enginegrant"
	"github.com/labstack/echo/v5"
)

func TestAppCredentialConnectionBoundary(t *testing.T) {
	valid := `{"app_id":"app","installation_id":"ins","provider_code":"rajaongkir","credential_id":"cred"}`
	for _, tc := range []struct {
		name, key, merchant, origin, body string
		result                            error
		want                              int
	}{
		{"valid", "service", "merchant_123", "", valid, nil, 200},
		{"no authentication", "", "merchant_123", "", valid, nil, 401},
		{"no merchant", "service", "", "", valid, nil, 400},
		{"browser", "service", "merchant_123", "https://seller.example", valid, nil, 403},
		{"spoof body", "service", "merchant_123", "", strings.TrimSuffix(valid, "}") + `,"merchant_id":"other"}`, nil, 400},
		{"revoked", "service", "merchant_123", "", valid, enginegrant.ErrDenied, 403},
		{"offline", "service", "merchant_123", "", valid, errors.New("private repository error"), 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			calls := 0
			g := e.Group("/api/v1/app-integrations", serviceAPIKeyMiddleware([]string{"service"}, nil), merchantContextMiddleware(true))
			g.POST("/credential-binding", appCredentialConnectionHandler(func(_ context.Context, b enginegrant.ProviderBinding, c string) error {
				calls++
				if b.MerchantID != "merchant_123" || b.AppID != "app" || c != "cred" {
					t.Fatal("wrong identity")
				}
				return tc.result
			}))
			r := httptest.NewRequest("POST", "/api/v1/app-integrations/credential-binding", strings.NewReader(tc.body))
			r.Header.Set("key", tc.key)
			r.Header.Set(merchantIDHeader, tc.merchant)
			r.Header.Set("Origin", tc.origin)
			w := httptest.NewRecorder()
			e.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatal(w.Code, tc.want, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "private repository error") || strings.Contains(w.Body.String(), `"credential_id"`) {
				t.Fatal("internal details leaked")
			}
			if (tc.want == 400 || tc.name == "browser" || tc.want == 401) && calls != 0 {
				t.Fatal("unauthorized call")
			}
		})
	}
}
