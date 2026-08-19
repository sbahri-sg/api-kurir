package tenancy

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestVerifierAcceptsSignedMerchantContext(t *testing.T) {
	t.Parallel()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewVerifier(
		base64.StdEncoding.EncodeToString(publicKey),
		"emisell-api",
		"api-kurir",
		5*time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	verifier.now = func() time.Time { return now }
	token := signTestToken(t, privateKey, map[string]any{
		"iss": "emisell-api", "aud": "api-kurir", "sub": "merchant_123",
		"integration_id": "integration_456", "domain_id": "domain_789",
		"scope": []string{"shipping:read", "provider-credentials:write"},
		"iat":   now.Unix(), "exp": now.Add(time.Minute).Unix(), "jti": "req_1",
	})
	identity, err := verifier.Verify(token)
	if err != nil {
		t.Fatal(err)
	}
	if identity.TenantID != "merchant_123" ||
		identity.IntegrationID != "integration_456" ||
		identity.DomainID != "domain_789" ||
		!HasScope(identity, "shipping:read") {
		t.Fatalf("unexpected identity: %#v", identity)
	}
}

func TestVerifierRejectsExpiredWrongAudienceAndTamperedTokens(t *testing.T) {
	t.Parallel()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewVerifier(
		base64.StdEncoding.EncodeToString(publicKey),
		"emisell-api",
		"api-kurir",
		5*time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	verifier.now = func() time.Time { return now }

	tests := []string{
		signTestToken(t, privateKey, map[string]any{
			"iss": "emisell-api", "aud": "api-kurir", "sub": "merchant_123",
			"iat": now.Add(-2 * time.Minute).Unix(), "exp": now.Add(-time.Minute).Unix(),
		}),
		signTestToken(t, privateKey, map[string]any{
			"iss": "emisell-api", "aud": "other-service", "sub": "merchant_123",
			"iat": now.Unix(), "exp": now.Add(time.Minute).Unix(),
		}),
	}
	valid := signTestToken(t, privateKey, map[string]any{
		"iss": "emisell-api", "aud": "api-kurir", "sub": "merchant_123",
		"iat": now.Unix(), "exp": now.Add(time.Minute).Unix(),
	})
	replacement := "A"
	if valid[len(valid)-1:] == replacement {
		replacement = "B"
	}
	tests = append(tests, valid[:len(valid)-1]+replacement)
	for _, token := range tests {
		if _, err := verifier.Verify(token); err == nil {
			t.Fatalf("expected token rejection: %s", token)
		}
	}
}

func signTestToken(t *testing.T, privateKey ed25519.PrivateKey, claims map[string]any) string {
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
