package biteship

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxAPIResponseBytes = 8 * 1024 * 1024

type APIError struct {
	StatusCode int
	Code       int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Biteship API %d/%d: %s", e.StatusCode, e.Code, e.Message)
}

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClient(baseURL, token string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   strings.TrimSpace(token),
		http:    &http.Client{Timeout: timeout},
	}
}

func (c *Client) doJSON(ctx context.Context, method, path string, result any) error {
	endpoint, err := url.JoinPath(c.baseURL, path)
	if err != nil {
		return fmt.Errorf("build Biteship URL: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create Biteship request: %w", err)
	}
	request.Header.Set("Authorization", c.token)
	request.Header.Set("Accept", "application/json")

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("call Biteship API: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxAPIResponseBytes))
	if err != nil {
		return fmt.Errorf("read Biteship response: %w", err)
	}
	var envelope struct {
		Success bool   `json:"success"`
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(responseBody, &envelope)
	if response.StatusCode < http.StatusOK ||
		response.StatusCode >= http.StatusMultipleChoices ||
		!envelope.Success {
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = http.StatusText(response.StatusCode)
		}
		return &APIError{
			StatusCode: response.StatusCode,
			Code:       envelope.Code,
			Message:    message,
		}
	}
	if err := json.Unmarshal(responseBody, result); err != nil {
		return fmt.Errorf("decode Biteship response: %w", err)
	}
	return nil
}
