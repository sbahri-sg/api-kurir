package partnerexplorer

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type ExecuteRequest struct {
	Method          string
	URL             string
	AuthHeader      string
	AuthValue       string
	RequestID       string
	Body            io.Reader
	TrustedInternal bool
}

type ExecuteResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	Truncated  bool
	Duration   time.Duration
}

type Executor interface {
	Do(ctx context.Context, request ExecuteRequest) (ExecuteResponse, error)
}

type HTTPExecutor struct {
	timeout  time.Duration
	resolver *net.Resolver
}

func NewHTTPExecutor(timeout time.Duration) *HTTPExecutor {
	return &HTTPExecutor{timeout: timeout, resolver: net.DefaultResolver}
}

func (e *HTTPExecutor) Do(ctx context.Context, input ExecuteRequest) (ExecuteResponse, error) {
	dialContext := e.safeDialContext
	if input.TrustedInternal {
		dialer := &net.Dialer{Timeout: 4 * time.Second, KeepAlive: -1}
		dialContext = dialer.DialContext
	}
	transport := &http.Transport{
		Proxy:             nil,
		DisableKeepAlives: true,
		TLSClientConfig:   &tls.Config{MinVersion: tls.VersionTLS12},
		DialContext:       dialContext,
	}
	client := &http.Client{
		Timeout:   e.timeout,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("redirect provider diblokir oleh Explorer")
		},
	}
	request, err := http.NewRequestWithContext(ctx, input.Method, input.URL, input.Body)
	if err != nil {
		return ExecuteResponse{}, err
	}
	request.Header.Set("Accept", "application/json")
	if input.Body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if input.AuthHeader != "" && input.AuthValue != "" {
		request.Header.Set(input.AuthHeader, input.AuthValue)
	}
	request.Header.Set("X-Request-Id", input.RequestID)
	request.Header.Set("User-Agent", "Emisell-Partner-Explorer/1.0")

	startedAt := time.Now()
	response, err := client.Do(request)
	duration := time.Since(startedAt)
	if err != nil {
		return ExecuteResponse{Duration: duration}, err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, MaxResponseBytes+1))
	if err != nil {
		return ExecuteResponse{Duration: duration}, err
	}
	truncated := len(payload) > MaxResponseBytes
	if truncated {
		payload = payload[:MaxResponseBytes]
	}
	return ExecuteResponse{
		StatusCode: response.StatusCode,
		Header:     response.Header.Clone(),
		Body:       payload,
		Truncated:  truncated,
		Duration:   duration,
	}, nil
}

func (e *HTTPExecutor) safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, ErrTargetBlocked
	}
	if port != "443" {
		return nil, fmt.Errorf("%w: port selain 443 tidak diizinkan", ErrTargetBlocked)
	}
	lookupContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	addresses, err := e.resolver.LookupIPAddr(lookupContext, host)
	if err != nil {
		return nil, fmt.Errorf("resolve provider host: %w", err)
	}
	if len(addresses) == 0 {
		return nil, errors.New("resolve provider host: tidak ada alamat publik")
	}
	for _, address := range addresses {
		if !safePublicIP(address.IP) {
			return nil, fmt.Errorf("%w: host mengarah ke jaringan non-publik", ErrTargetBlocked)
		}
	}
	dialer := &net.Dialer{Timeout: 4 * time.Second, KeepAlive: -1}
	return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].IP.String(), port))
}

func safePublicIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return false
	}
	_, carrierNAT, _ := net.ParseCIDR("100.64.0.0/10")
	return !carrierNAT.Contains(ip)
}

func upstreamErrorMessage(err error) string {
	if errors.Is(err, ErrTargetBlocked) {
		return "Target provider diblokir oleh kebijakan keamanan Explorer."
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "timeout"), strings.Contains(message, "deadline"):
		return "Provider tidak merespons sebelum batas waktu."
	case strings.Contains(message, "certificate"), strings.Contains(message, "tls"):
		return "Koneksi TLS provider tidak valid."
	default:
		return "Provider tidak dapat dihubungi dari test runner."
	}
}
