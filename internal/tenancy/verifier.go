package tenancy

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalidToken = errors.New("tenant context token is invalid")
	validSubject    = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
)

type Verifier struct {
	publicKey ed25519.PublicKey
	issuer    string
	audience  string
	maxTTL    time.Duration
	now       func() time.Time
}

type tokenHeader struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
}

type tokenClaims struct {
	Issuer        string          `json:"iss"`
	Audience      json.RawMessage `json:"aud"`
	Subject       string          `json:"sub"`
	IntegrationID string          `json:"integration_id"`
	DomainID      string          `json:"domain_id"`
	Scope         json.RawMessage `json:"scope"`
	IssuedAt      int64           `json:"iat"`
	ExpiresAt     int64           `json:"exp"`
	TokenID       string          `json:"jti"`
}

func NewVerifier(
	encodedPublicKey, issuer, audience string,
	maxTTL time.Duration,
) (*Verifier, error) {
	encodedPublicKey = strings.TrimSpace(encodedPublicKey)
	if encodedPublicKey == "" {
		return nil, nil
	}
	decoded, err := decodeBase64(encodedPublicKey)
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return nil, errors.New(
			"TENANT_CONTEXT_PUBLIC_KEY must be base64 Ed25519 public key (32 bytes)",
		)
	}
	issuer = strings.TrimSpace(issuer)
	audience = strings.TrimSpace(audience)
	if issuer == "" || audience == "" {
		return nil, errors.New("tenant context issuer and audience are required")
	}
	if maxTTL <= 0 || maxTTL > 15*time.Minute {
		return nil, errors.New("tenant context max TTL must be between 1ns and 15m")
	}
	return &Verifier{
		publicKey: ed25519.PublicKey(decoded),
		issuer:    issuer,
		audience:  audience,
		maxTTL:    maxTTL,
		now:       time.Now,
	}, nil
}

func (v *Verifier) Verify(token string) (Identity, error) {
	if v == nil {
		return Identity{}, ErrInvalidToken
	}
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 {
		return Identity{}, ErrInvalidToken
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Identity{}, ErrInvalidToken
	}
	var header tokenHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil ||
		header.Algorithm != "EdDSA" ||
		(header.Type != "" && !strings.EqualFold(header.Type, "JWT")) {
		return Identity{}, ErrInvalidToken
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !ed25519.Verify(
		v.publicKey,
		[]byte(parts[0]+"."+parts[1]),
		signature,
	) {
		return Identity{}, ErrInvalidToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Identity{}, ErrInvalidToken
	}
	var claims tokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Identity{}, ErrInvalidToken
	}
	if claims.Issuer != v.issuer || !audienceContains(claims.Audience, v.audience) {
		return Identity{}, ErrInvalidToken
	}
	claims.Subject = strings.TrimSpace(claims.Subject)
	if !validSubject.MatchString(claims.Subject) || claims.IssuedAt <= 0 || claims.ExpiresAt <= 0 {
		return Identity{}, ErrInvalidToken
	}
	now := v.now().UTC()
	issuedAt := time.Unix(claims.IssuedAt, 0)
	expiresAt := time.Unix(claims.ExpiresAt, 0)
	const clockSkew = 30 * time.Second
	if issuedAt.After(now.Add(clockSkew)) || !expiresAt.After(now.Add(-clockSkew)) {
		return Identity{}, ErrInvalidToken
	}
	if !expiresAt.After(issuedAt) || expiresAt.Sub(issuedAt) > v.maxTTL {
		return Identity{}, ErrInvalidToken
	}
	scopes, err := parseScopes(claims.Scope)
	if err != nil {
		return Identity{}, ErrInvalidToken
	}
	if !validOptionalIdentifier(claims.IntegrationID) || !validOptionalIdentifier(claims.DomainID) {
		return Identity{}, ErrInvalidToken
	}
	return Identity{
		TenantID:      claims.Subject,
		IntegrationID: strings.TrimSpace(claims.IntegrationID),
		DomainID:      strings.TrimSpace(claims.DomainID),
		Scopes:        scopes,
		TokenID:       strings.TrimSpace(claims.TokenID),
	}, nil
}

func audienceContains(raw json.RawMessage, expected string) bool {
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return single == expected
	}
	var multiple []string
	if json.Unmarshal(raw, &multiple) != nil {
		return false
	}
	for _, audience := range multiple {
		if audience == expected {
			return true
		}
	}
	return false
}

func parseScopes(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return []string{}, nil
	}
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return normalizeScopes(strings.Fields(single)), nil
	}
	var multiple []string
	if err := json.Unmarshal(raw, &multiple); err != nil {
		return nil, fmt.Errorf("parse scopes: %w", err)
	}
	return normalizeScopes(multiple), nil
}

func normalizeScopes(input []string) []string {
	seen := make(map[string]struct{}, len(input))
	result := make([]string, 0, len(input))
	for _, scope := range input {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, exists := seen[scope]; exists {
			continue
		}
		seen[scope] = struct{}{}
		result = append(result, scope)
	}
	return result
}

func validOptionalIdentifier(value string) bool {
	value = strings.TrimSpace(value)
	return value == "" || validSubject.MatchString(value)
}

func decodeBase64(value string) ([]byte, error) {
	for _, encoding := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.RawURLEncoding,
	} {
		decoded, err := encoding.DecodeString(value)
		if err == nil {
			return decoded, nil
		}
	}
	return nil, errors.New("invalid base64")
}
