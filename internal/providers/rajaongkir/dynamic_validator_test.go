package rajaongkir

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/providercredentials"
)

func TestCredentialValidatorValidatesDeliveryKeyAgainstSelectedEnvironment(t *testing.T) {
	t.Parallel()
	liveCalls := 0
	live := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		liveCalls++
		http.Error(response, "wrong environment", http.StatusUnauthorized)
	}))
	defer live.Close()
	sandboxCalls := 0
	sandbox := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		sandboxCalls++
		if request.URL.Path != "/tariff/api/v1/destination/search" ||
			request.URL.Query().Get("keyword") != "53131" ||
			request.Header.Get("x-api-key") != "valid-delivery-key" {
			http.Error(response, "unexpected validation request", http.StatusBadRequest)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"data":[]}`))
	}))
	defer sandbox.Close()

	validator := NewCredentialValidator(
		"https://shipping.invalid", time.Second, 0, live.URL, sandbox.URL,
	)
	err := validator.ValidateCredentials(
		providercredentials.WithExecutionEnvironment(context.Background(), providercredentials.EnvironmentSandbox),
		"rajaongkir",
		providercredentials.CredentialTypeProviderDeclared,
		map[string]string{"delivery_api_key": "valid-delivery-key"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if liveCalls != 0 || sandboxCalls != 1 {
		t.Fatalf("live calls=%d sandbox calls=%d", liveCalls, sandboxCalls)
	}
}

func TestCredentialValidatorRejectsInvalidDeliveryKey(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		http.Error(response, `{"message":"Invalid API Key"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	validator := NewCredentialValidator(
		"https://shipping.invalid", time.Second, 0, server.URL, server.URL,
	)
	err := validator.ValidateCredentials(
		context.Background(),
		"rajaongkir",
		providercredentials.CredentialTypeProviderDeclared,
		map[string]string{"delivery_api_key": "invalid-delivery-key"},
	)
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("expected invalid delivery key rejection, got %v", err)
	}
}
