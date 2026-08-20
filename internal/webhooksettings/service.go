package webhooksettings

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/emisell/api-kurir/internal/providercredentials"
)

const webhookAPIversion = "2026-08-20"

type Service struct {
	repository Repository
	cipher     *providercredentials.Cipher
	appEnv     string
	fallback   Fallback
	client     *http.Client
	now        func() time.Time
}

func NewService(
	repository Repository,
	cipher *providercredentials.Cipher,
	appEnv string,
	fallback Fallback,
) *Service {
	return &Service{
		repository: repository,
		cipher:     cipher,
		appEnv:     strings.ToLower(strings.TrimSpace(appEnv)),
		fallback: Fallback{
			Enabled:     fallback.Enabled,
			CallbackURL: strings.TrimSpace(fallback.CallbackURL),
			Secret:      strings.TrimSpace(fallback.Secret),
		},
		client: &http.Client{Timeout: 5 * time.Second},
		now:    time.Now,
	}
}

func (s *Service) Get(ctx context.Context) (Settings, error) {
	stored, err := s.repository.Get(ctx)
	if err == nil {
		return stored.Settings, nil
	}
	if err != ErrNotConfigured {
		return Settings{}, err
	}
	configured := s.fallback.CallbackURL != "" || s.fallback.Secret != ""
	return Settings{
		Configured:       configured,
		CallbackURL:      s.fallback.CallbackURL,
		Enabled:          s.fallback.Enabled,
		SecretConfigured: s.fallback.Secret != "",
		SecretHint:       fallbackSecretHint(s.fallback.Secret),
		Source:           "environment",
	}, nil
}

func (s *Service) Update(
	ctx context.Context,
	callbackURL string,
	enabled bool,
	actor string,
) (Settings, error) {
	callbackURL = strings.TrimSpace(callbackURL)
	actor = normalizedActor(actor)
	if callbackURL != "" {
		if err := validateCallbackURL(callbackURL, s.appEnv); err != nil {
			return Settings{}, err
		}
	}
	if enabled && callbackURL == "" {
		return Settings{}, ErrInvalidURL
	}
	if err := s.materializeFallback(ctx, actor); err != nil {
		return Settings{}, err
	}
	if enabled {
		stored, err := s.repository.Get(ctx)
		if err != nil {
			return Settings{}, err
		}
		if len(stored.SecretCiphertext) == 0 {
			return Settings{}, ErrSecretNotConfigured
		}
	}
	if err := s.repository.UpsertConfig(ctx, callbackURL, enabled, actor); err != nil {
		return Settings{}, err
	}
	return s.Get(ctx)
}

func (s *Service) GenerateSecret(
	ctx context.Context,
	actor string,
) (GeneratedSecret, error) {
	actor = normalizedActor(actor)
	if err := s.materializeFallback(ctx, actor); err != nil {
		return GeneratedSecret{}, err
	}
	stored, err := s.repository.Get(ctx)
	if err != nil {
		return GeneratedSecret{}, err
	}
	// Rotation is fail-closed. Pending events stay in the outbox until Emisell
	// has received the new one-time secret and the operator re-enables delivery.
	if err := s.repository.UpsertConfig(ctx, stored.CallbackURL, false, actor); err != nil {
		return GeneratedSecret{}, err
	}
	randomValue := make([]byte, 32)
	if _, err := rand.Read(randomValue); err != nil {
		return GeneratedSecret{}, fmt.Errorf("generate tracking webhook secret: %w", err)
	}
	secret := "whsec_" + base64.RawURLEncoding.EncodeToString(randomValue)
	if err := s.persistSecret(ctx, secret, actor); err != nil {
		return GeneratedSecret{}, err
	}
	settings, err := s.Get(ctx)
	if err != nil {
		return GeneratedSecret{}, err
	}
	return GeneratedSecret{Settings: settings, Secret: secret}, nil
}

func (s *Service) ResolveWebhookDestination(
	ctx context.Context,
) (endpoint string, secret []byte, enabled bool, err error) {
	stored, err := s.repository.Get(ctx)
	if err == ErrNotConfigured {
		if !s.fallback.Enabled {
			return "", nil, false, nil
		}
		if s.fallback.CallbackURL == "" || s.fallback.Secret == "" {
			return "", nil, false, ErrNotConfigured
		}
		return s.fallback.CallbackURL, []byte(s.fallback.Secret), true, nil
	}
	if err != nil {
		return "", nil, false, err
	}
	if !stored.Enabled {
		return "", nil, false, nil
	}
	plaintext, err := s.decrypt(stored)
	if err != nil {
		return "", nil, false, err
	}
	return stored.CallbackURL, plaintext, true, nil
}

func (s *Service) Test(
	ctx context.Context,
	actor string,
) (TestResult, error) {
	actor = normalizedActor(actor)
	if err := s.materializeFallback(ctx, actor); err != nil {
		return TestResult{}, err
	}
	stored, err := s.repository.Get(ctx)
	if err != nil {
		return TestResult{}, err
	}
	if stored.CallbackURL == "" {
		return TestResult{}, ErrInvalidURL
	}
	if err := validateCallbackURL(stored.CallbackURL, s.appEnv); err != nil {
		return TestResult{}, err
	}
	secret, err := s.decrypt(stored)
	if err != nil {
		return TestResult{}, err
	}
	defer clearBytes(secret)

	eventID, err := randomEventID()
	if err != nil {
		return TestResult{}, err
	}
	testedAt := s.now().UTC()
	payload := map[string]any{
		"id":          eventID,
		"type":        "tracking.test",
		"api_version": webhookAPIversion,
		"occurred_at": testedAt.Format(time.RFC3339Nano),
		"data": map[string]any{
			"source":  "api-kurir-dashboard",
			"message": "Webhook test berhasil diterima.",
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return TestResult{}, fmt.Errorf("encode tracking webhook test: %w", err)
	}
	timestamp := strconv.FormatInt(testedAt.Unix(), 10)
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		stored.CallbackURL,
		bytes.NewReader(body),
	)
	if err != nil {
		return TestResult{}, fmt.Errorf("create tracking webhook test request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Emisell-Event-ID", eventID)
	request.Header.Set("X-Emisell-Event-Type", "tracking.test")
	request.Header.Set("X-Emisell-Webhook-Timestamp", timestamp)
	request.Header.Set(
		"X-Emisell-Webhook-Signature",
		webhookSignature(secret, timestamp, body),
	)

	response, requestErr := s.client.Do(request)
	if requestErr != nil {
		message := "Penerima webhook tidak dapat dihubungi."
		_ = s.repository.RecordTest(ctx, testedAt, false, 0, message)
		return TestResult{
			Success: false, EventID: eventID, TestedAt: testedAt, Message: message,
		}, nil
	}
	defer response.Body.Close()
	_, _ = io.CopyN(io.Discard, response.Body, 4096)
	success := response.StatusCode >= 200 && response.StatusCode < 300
	message := "Webhook test diterima Emisell."
	if !success {
		message = fmt.Sprintf("Penerima webhook merespons HTTP %d.", response.StatusCode)
	}
	recordMessage := ""
	if !success {
		recordMessage = message
	}
	if err := s.repository.RecordTest(
		ctx,
		testedAt,
		success,
		response.StatusCode,
		recordMessage,
	); err != nil {
		return TestResult{}, err
	}
	return TestResult{
		Success:    success,
		HTTPStatus: response.StatusCode,
		EventID:    eventID,
		TestedAt:   testedAt,
		Message:    message,
	}, nil
}

func (s *Service) materializeFallback(ctx context.Context, actor string) error {
	_, err := s.repository.Get(ctx)
	if err == nil {
		return nil
	}
	if err != ErrNotConfigured {
		return err
	}
	if err := s.repository.UpsertConfig(
		ctx,
		s.fallback.CallbackURL,
		false,
		actor,
	); err != nil {
		return err
	}
	if s.fallback.Secret != "" {
		if err := s.persistSecret(ctx, s.fallback.Secret, actor); err != nil {
			return err
		}
	}
	if s.fallback.Enabled && s.fallback.CallbackURL != "" && s.fallback.Secret != "" {
		return s.repository.UpsertConfig(ctx, s.fallback.CallbackURL, true, actor)
	}
	return nil
}

func (s *Service) persistSecret(ctx context.Context, secret, actor string) error {
	fingerprint := sha256.Sum256([]byte(secret))
	fingerprintHex := hex.EncodeToString(fingerprint[:])
	ciphertext, err := s.cipher.Encrypt(
		[]byte(secret),
		[]byte("tracking-webhook:"+fingerprintHex),
	)
	if err != nil {
		return err
	}
	prefixLength := 8
	if len(secret)-4 < prefixLength {
		prefixLength = len(secret) - 4
	}
	if prefixLength < 1 {
		return ErrSecretNotConfigured
	}
	return s.repository.RotateSecret(ctx, SecretInput{
		Ciphertext:  ciphertext,
		Fingerprint: fingerprint[:],
		Prefix:      secret[:prefixLength],
		LastFour:    secret[len(secret)-4:],
		Actor:       normalizedActor(actor),
	})
}

func (s *Service) decrypt(stored StoredSettings) ([]byte, error) {
	if len(stored.SecretCiphertext) == 0 || len(stored.SecretFingerprint) == 0 {
		return nil, ErrSecretNotConfigured
	}
	fingerprintHex := hex.EncodeToString(stored.SecretFingerprint)
	return s.cipher.Decrypt(
		stored.SecretCiphertext,
		[]byte("tracking-webhook:"+fingerprintHex),
	)
}

func validateCallbackURL(value, appEnv string) error {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return ErrInvalidURL
	}
	if parsed.Scheme != "https" && !(appEnv != "production" && parsed.Scheme == "http") {
		return ErrInvalidURL
	}
	if appEnv == "production" {
		host := strings.ToLower(parsed.Hostname())
		if host == "localhost" || strings.HasSuffix(host, ".localhost") {
			return ErrInvalidURL
		}
		if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() || ip.IsUnspecified()) {
			return ErrInvalidURL
		}
	}
	return nil
}

func webhookSignature(secret []byte, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

func randomEventID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate tracking webhook test ID: %w", err)
	}
	return "evt_test_" + hex.EncodeToString(value), nil
}

func normalizedActor(actor string) string {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return "system"
	}
	return actor
}

func fallbackSecretHint(secret string) string {
	if secret == "" {
		return ""
	}
	return "Tersimpan di environment"
}

func clearBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
