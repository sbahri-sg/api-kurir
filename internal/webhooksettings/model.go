package webhooksettings

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotConfigured       = errors.New("tracking webhook is not configured")
	ErrInvalidURL          = errors.New("tracking webhook URL is invalid")
	ErrSecretNotConfigured = errors.New("tracking webhook secret is not configured")
)

type Settings struct {
	Configured         bool       `json:"configured"`
	CallbackURL        string     `json:"callback_url"`
	Enabled            bool       `json:"enabled"`
	SecretConfigured   bool       `json:"secret_configured"`
	SecretHint         string     `json:"secret_hint"`
	Source             string     `json:"source"`
	LastTestAt         *time.Time `json:"last_test_at"`
	LastTestSuccess    *bool      `json:"last_test_success"`
	LastTestHTTPStatus *int       `json:"last_test_http_status"`
	LastTestError      string     `json:"last_test_error"`
	UpdatedBy          string     `json:"updated_by"`
	UpdatedAt          *time.Time `json:"updated_at"`
}

type StoredSettings struct {
	Settings
	SecretCiphertext  []byte
	SecretFingerprint []byte
}

type SecretInput struct {
	Ciphertext  []byte
	Fingerprint []byte
	Prefix      string
	LastFour    string
	Actor       string
}

type GeneratedSecret struct {
	Settings Settings `json:"settings"`
	Secret   string   `json:"secret"`
}

type TestResult struct {
	Success    bool      `json:"success"`
	HTTPStatus int       `json:"http_status"`
	EventID    string    `json:"event_id"`
	TestedAt   time.Time `json:"tested_at"`
	Message    string    `json:"message"`
}

type Repository interface {
	Get(ctx context.Context) (StoredSettings, error)
	UpsertConfig(ctx context.Context, callbackURL string, enabled bool, actor string) error
	RotateSecret(ctx context.Context, input SecretInput) error
	RecordTest(ctx context.Context, testedAt time.Time, success bool, httpStatus int, message string) error
}

type Fallback struct {
	Enabled     bool
	CallbackURL string
	Secret      string
}
