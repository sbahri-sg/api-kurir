package biteship

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCredentialValidatorUsesCourierCatalogWithoutTrackingCall(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/couriers" {
			t.Fatalf("validator must not call paid tracking endpoint: %s", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"success":true,"couriers":[{"courier_code":"sicepat"}]}`))
	}))
	defer server.Close()

	err := NewCredentialValidator(server.URL, time.Second).Validate(
		context.Background(), "biteship", "biteship_test.valid",
	)
	if err != nil {
		t.Fatal(err)
	}
}
