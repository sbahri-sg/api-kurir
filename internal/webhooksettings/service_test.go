package webhooksettings

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/providercredentials"
)

type repositoryStub struct {
	stored StoredSettings
	exists bool
}

func (r *repositoryStub) Get(context.Context) (StoredSettings, error) {
	if !r.exists {
		return StoredSettings{}, ErrNotConfigured
	}
	return r.stored, nil
}

func (r *repositoryStub) UpsertConfig(
	_ context.Context,
	callbackURL string,
	enabled bool,
	actor string,
) error {
	r.exists = true
	r.stored.Configured = true
	r.stored.CallbackURL = callbackURL
	r.stored.Enabled = enabled
	r.stored.Source = "database"
	r.stored.UpdatedBy = actor
	now := time.Now().UTC()
	r.stored.UpdatedAt = &now
	return nil
}

func (r *repositoryStub) RotateSecret(_ context.Context, input SecretInput) error {
	r.exists = true
	r.stored.Configured = true
	r.stored.SecretConfigured = true
	r.stored.SecretCiphertext = append([]byte(nil), input.Ciphertext...)
	r.stored.SecretFingerprint = append([]byte(nil), input.Fingerprint...)
	r.stored.SecretHint = maskedSecretHint(input.Prefix, input.LastFour)
	r.stored.Source = "database"
	return nil
}

func (r *repositoryStub) RecordTest(
	_ context.Context,
	testedAt time.Time,
	success bool,
	httpStatus int,
	message string,
) error {
	r.stored.LastTestAt = &testedAt
	r.stored.LastTestSuccess = &success
	if httpStatus > 0 {
		r.stored.LastTestHTTPStatus = &httpStatus
	}
	r.stored.LastTestError = message
	return nil
}

func testCipher(t *testing.T) *providercredentials.Cipher {
	t.Helper()
	key := make([]byte, 32)
	for index := range key {
		key[index] = byte(index + 1)
	}
	cipher, err := providercredentials.NewCipher(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	return cipher
}

func TestGenerateSecretIsWriteOnlyAndImmediatelyResolvable(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository, testCipher(t), "development", Fallback{})

	generated, err := service.GenerateSecret(context.Background(), "emisell")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(generated.Secret, "whsec_") || len(generated.Secret) < 32 {
		t.Fatalf("unexpected generated secret format: %q", generated.Secret)
	}
	if !generated.Settings.SecretConfigured || generated.Settings.SecretHint == generated.Secret {
		t.Fatalf("secret must be masked after storage: %#v", generated.Settings)
	}
	if generated.Settings.Enabled {
		t.Fatal("secret rotation must disable delivery until receiver is updated")
	}
	if _, err := service.Update(
		context.Background(),
		"http://127.0.0.1:9999/webhook",
		true,
		"emisell",
	); err != nil {
		t.Fatal(err)
	}
	endpoint, secret, enabled, err := service.ResolveWebhookDestination(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !enabled || endpoint != "http://127.0.0.1:9999/webhook" || string(secret) != generated.Secret {
		t.Fatalf("unexpected resolved destination: %q %t", endpoint, enabled)
	}
}

func TestEnableRequiresGeneratedSecret(t *testing.T) {
	service := NewService(&repositoryStub{}, testCipher(t), "development", Fallback{})
	_, err := service.Update(
		context.Background(),
		"http://127.0.0.1:9999/webhook",
		true,
		"emisell",
	)
	if err != ErrSecretNotConfigured {
		t.Fatalf("expected secret requirement, got %v", err)
	}
}

func TestWebhookTestUsesHMACAndContainsNoShipmentData(t *testing.T) {
	var received bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["type"] != "tracking.test" || strings.Contains(string(body), "waybill") {
			t.Fatalf("unexpected test payload: %s", body)
		}
		timestamp := request.Header.Get("X-Emisell-Webhook-Timestamp")
		secret := []byte("whsec_test-test-test-test-test-test")
		mac := hmac.New(sha256.New, secret)
		_, _ = mac.Write([]byte(timestamp + "."))
		_, _ = mac.Write(body)
		want := "v1=" + hex.EncodeToString(mac.Sum(nil))
		if request.Header.Get("X-Emisell-Webhook-Signature") != want {
			t.Fatal("invalid test webhook signature")
		}
		received = true
		writer.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	repository := &repositoryStub{}
	service := NewService(repository, testCipher(t), "development", Fallback{
		Enabled: true, CallbackURL: server.URL, Secret: "whsec_test-test-test-test-test-test",
	})
	result, err := service.Test(context.Background(), "emisell")
	if err != nil {
		t.Fatal(err)
	}
	if !received || !result.Success || result.HTTPStatus != http.StatusAccepted {
		t.Fatalf("unexpected test result: %#v", result)
	}
}

func TestProductionRejectsHTTPAndPrivateTargets(t *testing.T) {
	service := NewService(&repositoryStub{}, testCipher(t), "production", Fallback{})
	for _, target := range []string{
		"http://api.emisell.test/webhook",
		"https://127.0.0.1/webhook",
		"https://localhost/webhook",
	} {
		if _, err := service.Update(context.Background(), target, false, "emisell"); err != ErrInvalidURL {
			t.Fatalf("target %q should be rejected, got %v", target, err)
		}
	}
}
