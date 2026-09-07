package enginegrant

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type providerTarget struct {
	MerchantID     string `json:"merchantId"`
	ProviderCode   string `json:"providerCode"`
	AppID          string `json:"appId"`
	InstallationID string `json:"installationId"`
}

// Reconcile scans all enrolled installations, including revoked ones. Every run
// starts over, so late commits with earlier IDs cannot be skipped forever.
func (c *ProviderClient) Reconcile(ctx context.Context, w SnapshotWriter) (int, error) {
	if w == nil {
		return 0, ErrUnavailable
	}
	after := ""
	count := 0
	for {
		if err := ctx.Err(); err != nil {
			return count, err
		}
		endpoint := strings.TrimSuffix(c.endpoint, "/check") + "/targets?after=" + url.QueryEscape(after)
		req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
		if err != nil {
			return count, ErrUnavailable
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		res, err := c.http.Do(req)
		if err != nil {
			return count, ErrUnavailable
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 65537))
		res.Body.Close()
		if readErr != nil || len(raw) > 65536 || res.StatusCode != 200 {
			return count, ErrUnavailable
		}
		var page struct {
			Targets []providerTarget `json:"targets"`
			Next    string           `json:"next"`
		}
		d := json.NewDecoder(bytes.NewReader(raw))
		d.DisallowUnknownFields()
		if d.Decode(&page) != nil || d.Decode(&struct{}{}) != io.EOF || len(page.Targets) > 100 {
			return count, ErrUnavailable
		}
		last := after
		for _, target := range page.Targets {
			if target.InstallationID <= last {
				return count, ErrUnavailable
			}
			last = target.InstallationID
			b := ProviderBinding{MerchantID: target.MerchantID, ProviderCode: target.ProviderCode, AppID: target.AppID, InstallationID: target.InstallationID}
			if err = c.Sync(ctx, b, w); err != nil {
				return count, err
			}
			count++
		}
		if page.Next == "" {
			return count, nil
		}
		if len(page.Targets) == 0 || page.Next != last || page.Next <= after {
			return count, ErrUnavailable
		}
		after = page.Next
	}
}
