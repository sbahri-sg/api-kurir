package enginegrant

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"time"
)

type ProviderSnapshot struct {
	MerchantID     string   `json:"merchantId"`
	ProviderCode   string   `json:"providerCode"`
	AppID          string   `json:"appId"`
	InstallationID string   `json:"installationId"`
	Revision       int64    `json:"revision,string"`
	Active         bool     `json:"active"`
	Revoked        bool     `json:"revoked"`
	Scopes         []string `json:"scopes"`
}
type ProviderClient struct {
	endpoint, key string
	http          *http.Client
}

func NewProviderClient(base, key string, allowLoopback bool) (*ProviderClient, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.Path != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		!(u.Scheme == "https" || (allowLoopback && u.Scheme == "http" && u.Hostname() == "127.0.0.1" && u.Port() != "")) || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(key) {
		return nil, ErrUnavailable
	}
	return &ProviderClient{base + "/internal/provider-grants/v1/check", key, &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil, MaxConnsPerHost: 8}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *ProviderClient) snapshot(ctx context.Context, b ProviderBinding, op string) (ProviderSnapshot, error) {
	var v ProviderSnapshot
	if !identifier.MatchString(b.MerchantID) || !identifier.MatchString(b.AppID) || !identifier.MatchString(b.InstallationID) || !providerCode.MatchString(b.ProviderCode) {
		return v, ErrDenied
	}
	raw, _ := json.Marshal(map[string]string{"merchantId": b.MerchantID, "providerCode": b.ProviderCode, "appId": b.AppID, "installationId": b.InstallationID, "operation": op})
	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewReader(raw))
	if err != nil {
		return v, ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return v, ErrUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode == 401 || res.StatusCode == 403 {
		return v, ErrDenied
	}
	if res.StatusCode != 200 {
		return v, ErrUnavailable
	}
	raw, err = io.ReadAll(io.LimitReader(res.Body, 8193))
	if err != nil || len(raw) > 8192 {
		return v, ErrUnavailable
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&v) != nil || d.Decode(&struct{}{}) != io.EOF || v.MerchantID != b.MerchantID || v.ProviderCode != b.ProviderCode || v.AppID != b.AppID || v.InstallationID != b.InstallationID || v.Revision < 1 || (v.Revoked && v.Active) {
		return v, ErrDenied
	}
	return v, nil
}
func (c *ProviderClient) Verify(ctx context.Context, b ProviderBinding, op string) error {
	scope := ""
	switch op {
	case "rates.read", "settings.read", "tracking.read":
		scope = "shipping.read"
	case "settings.write", "shipments.create":
		scope = "shipping.write"
	default:
		return ErrDenied
	}
	v, err := c.snapshot(ctx, b, op)
	if err != nil {
		return err
	}
	if !v.Active || v.Revoked || !slices.Contains(v.Scopes, scope) {
		return ErrDenied
	}
	return nil
}

type SnapshotWriter interface {
	ApplySnapshot(context.Context, ProviderSnapshot) error
}

// Sync pulls current authority; it never trusts a browser/event payload as state.
// An out-of-order notification triggers a fresh read, not a stale state write.
func (c *ProviderClient) Sync(ctx context.Context, b ProviderBinding, w SnapshotWriter) error {
	if w == nil {
		return ErrUnavailable
	}
	v, err := c.snapshot(ctx, b, "binding.read")
	if err != nil {
		return err
	}
	return w.ApplySnapshot(ctx, v)
}
