package merchantproviders

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrInvalidTenant         = errors.New("tenant ID is invalid")
	ErrInvalidProvider       = errors.New("provider code is invalid")
	ErrInvalidCredential     = errors.New("provider credential ID is invalid")
	ErrProviderNotFound      = errors.New("shipping provider is not found")
	ErrProviderUnavailable   = errors.New("shipping provider is unavailable")
	ErrCredentialRequired    = errors.New("provider credential is required")
	ErrCredentialUnavailable = errors.New("provider credential is unavailable")
	ErrVersionConflict       = errors.New("shipping provider selection version conflict")
)

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Catalog(ctx context.Context, tenantID string) (Catalog, error) {
	tenantID = strings.TrimSpace(tenantID)
	if !validTenantID(tenantID) {
		return Catalog{}, ErrInvalidTenant
	}
	return s.repository.Catalog(ctx, tenantID)
}

func (s *Service) HasActiveProvider(ctx context.Context, tenantID string) (bool, error) {
	tenantID = strings.TrimSpace(tenantID)
	if !validTenantID(tenantID) {
		return false, ErrInvalidTenant
	}
	if repository, ok := s.repository.(interface {
		HasActiveProvider(context.Context, string) (bool, error)
	}); ok {
		return repository.HasActiveProvider(ctx, tenantID)
	}
	catalog, err := s.repository.Catalog(ctx, tenantID)
	if err != nil {
		return false, err
	}
	return catalog.ActiveProviderCode != nil, nil
}

func (s *Service) Activate(
	ctx context.Context,
	tenantID string,
	providerCode string,
	input ChangeInput,
) (Catalog, error) {
	tenantID, providerCode, input, err := normalizeChange(tenantID, providerCode, input)
	if err != nil {
		return Catalog{}, err
	}
	return s.repository.Activate(ctx, tenantID, providerCode, input)
}

func (s *Service) Deactivate(
	ctx context.Context,
	tenantID string,
	providerCode string,
	input ChangeInput,
) (Catalog, error) {
	tenantID, providerCode, input, err := normalizeChange(tenantID, providerCode, input)
	if err != nil {
		return Catalog{}, err
	}
	return s.repository.Deactivate(ctx, tenantID, providerCode, input)
}

func normalizeChange(
	tenantID string,
	providerCode string,
	input ChangeInput,
) (string, string, ChangeInput, error) {
	tenantID = strings.TrimSpace(tenantID)
	if !validTenantID(tenantID) {
		return "", "", ChangeInput{}, ErrInvalidTenant
	}
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	if !validProviderCode(providerCode) {
		return "", "", ChangeInput{}, ErrInvalidProvider
	}
	input.UpdatedBy = strings.TrimSpace(input.UpdatedBy)
	if input.ExpectedVersion != nil && *input.ExpectedVersion < 0 {
		return "", "", ChangeInput{}, ErrVersionConflict
	}
	return tenantID, providerCode, input, nil
}

func validTenantID(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune("._:-", character) {
			continue
		}
		return false
	}
	return true
}

func validProviderCode(value string) bool {
	if len(value) < 2 || len(value) > 48 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') ||
			character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}
