package biteship

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/emisell/api-kurir/internal/providercredentials"
)

const maxValidationResponseBytes = 2 * 1024 * 1024

type CredentialValidator struct {
	baseURL string
	client  *http.Client
}

func NewCredentialValidator(baseURL string, timeout time.Duration) *CredentialValidator {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &CredentialValidator{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		client:  &http.Client{Timeout: timeout},
	}
}

func (v *CredentialValidator) Validate(
	ctx context.Context,
	providerCode, secret string,
) error {
	if !strings.EqualFold(strings.TrimSpace(providerCode), "biteship") {
		return providercredentials.ErrUnsupportedProvider
	}
	secret = strings.TrimSpace(secret)
	if !strings.HasPrefix(secret, "biteship_live.") &&
		!strings.HasPrefix(secret, "biteship_test.") {
		return errors.New("Biteship token must use biteship_live or biteship_test prefix")
	}
	endpoint, err := url.JoinPath(v.baseURL, "v1", "couriers")
	if err != nil {
		return fmt.Errorf("build Biteship validation URL: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create Biteship validation request: %w", err)
	}
	request.Header.Set("Authorization", secret)
	request.Header.Set("Accept", "application/json")

	response, err := v.client.Do(request)
	if err != nil {
		return fmt.Errorf("validate Biteship credential: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxValidationResponseBytes))
	if err != nil {
		return fmt.Errorf("read Biteship validation response: %w", err)
	}
	var payload struct {
		Success  bool            `json:"success"`
		Message  string          `json:"message"`
		Couriers json.RawMessage `json:"couriers"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("decode Biteship validation response: %w", err)
	}
	if response.StatusCode < http.StatusOK ||
		response.StatusCode >= http.StatusMultipleChoices ||
		!payload.Success {
		message := strings.TrimSpace(payload.Message)
		if message == "" {
			message = http.StatusText(response.StatusCode)
		}
		return fmt.Errorf("Biteship rejected credential: %s", message)
	}
	if len(payload.Couriers) == 0 || string(payload.Couriers) == "null" {
		return errors.New("Biteship credential returned no courier catalog")
	}
	return nil
}
