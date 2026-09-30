package hosted

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/emisell/api-kurir/internal/providercredentials"
)

const maxResponseBytes = 4 << 20

type Client struct {
	baseURL *url.URL
	client  *http.Client
}

type HTTPError struct {
	Status int
	Code   string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("hosted connector returned HTTP %d (%s)", e.Status, e.Code)
}

func NewClient(baseURL string, timeout time.Duration) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("hosted connector base URL is invalid")
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return &Client{baseURL: parsed, client: &http.Client{Timeout: timeout}}, nil
}

func (c *Client) Do(
	ctx context.Context,
	method, path, authHeader, credential string,
	payload any,
	target any,
) error {
	endpoint, err := c.baseURL.Parse(strings.TrimLeft(path, "/"))
	if err != nil {
		return err
	}
	var body io.Reader
	if payload != nil {
		encoded, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return marshalErr
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set(
		"X-Emisell-Execution-Mode",
		providercredentials.ExecutionEnvironment(ctx),
	)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if authHeader != "" && credential != "" {
		request.Header.Set(authHeader, credential)
	}
	response, err := c.client.Do(request)
	if err != nil {
		var netErr net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
			return context.DeadlineExceeded
		}
		return err
	}
	defer response.Body.Close()
	payloadBytes, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return err
	}
	if len(payloadBytes) > maxResponseBytes {
		return errors.New("hosted connector response is too large")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var failure struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(payloadBytes, &failure)
		return &HTTPError{Status: response.StatusCode, Code: failure.Error.Code}
	}
	if target == nil || len(bytes.TrimSpace(payloadBytes)) == 0 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(payloadBytes))
	decoder.UseNumber()
	return decoder.Decode(target)
}
