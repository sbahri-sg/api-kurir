package merchantproviders

import (
	"context"
	"errors"
	"strings"

	"github.com/emisell/api-kurir/internal/providercredentials"
	"github.com/emisell/api-kurir/internal/tenancy"
)

var (
	ErrInvalidTenant         = errors.New("tenant ID is invalid")
	ErrInvalidProvider       = errors.New("provider code is invalid")
	ErrInvalidCredential     = errors.New("provider credential ID is invalid")
	ErrProviderNotFound      = errors.New("shipping provider is not found")
	ErrProviderUnavailable   = errors.New("shipping provider is unavailable")
	ErrReleaseUnavailable    = errors.New("shipping provider has no published release")
	ErrCredentialRequired    = errors.New("provider credential is required")
	ErrCredentialUnavailable = errors.New("provider credential is unavailable")
	ErrVersionConflict       = errors.New("shipping provider selection version conflict")
)

type Service struct {
	repository             Repository
	credentialAvailability CredentialAvailabilityResolver
}

type CredentialAvailabilityResolver interface {
	AvailableCredentialFieldsForTenant(
		ctx context.Context,
		tenantID, providerCode string,
	) (providercredentials.Availability, error)
}

func NewService(
	repository Repository,
	availability ...CredentialAvailabilityResolver,
) *Service {
	service := &Service{repository: repository}
	if len(availability) > 0 {
		service.credentialAvailability = availability[0]
	}
	return service
}

func (s *Service) Catalog(ctx context.Context, tenantID string) (Catalog, error) {
	tenantID = strings.TrimSpace(tenantID)
	if !validTenantID(tenantID) {
		return Catalog{}, ErrInvalidTenant
	}
	return s.repository.Catalog(ctx, tenantID)
}

func (s *Service) Provider(
	ctx context.Context,
	tenantID, providerCode string,
) (Detail, error) {
	tenantID = strings.TrimSpace(tenantID)
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	if !validTenantID(tenantID) {
		return Detail{}, ErrInvalidTenant
	}
	if !validProviderCode(providerCode) {
		return Detail{}, ErrInvalidProvider
	}
	catalog, err := s.repository.Catalog(ctx, tenantID)
	if err != nil {
		return Detail{}, err
	}
	var provider *Provider
	for index := range catalog.Providers {
		if catalog.Providers[index].Code == providerCode {
			provider = &catalog.Providers[index]
			break
		}
	}
	if provider == nil {
		return Detail{}, ErrProviderNotFound
	}
	detail := Detail{
		Provider:               *provider,
		AutoPickupStatus:       AutoPickupStatusUnsupported,
		AutoPickupEnvironments: make(map[string]AutoPickupEnvironmentStatus),
		AvailableCredentials:   make(map[string][]string),
	}
	for _, environment := range provider.Environments {
		detail.AvailableCredentials[environment.Code] = []string{}
		detail.AutoPickupEnvironments[environment.Code] = AutoPickupEnvironmentStatus{
			Status:             AutoPickupStatusUnsupported,
			MissingCredentials: []string{},
		}
	}
	availability := providercredentials.Availability{
		FieldsByEnvironment: map[string][]string{},
		StatusByEnvironment: map[string]string{},
	}
	if provider.RequiresCredential && s.credentialAvailability != nil {
		availability, err = s.credentialAvailability.AvailableCredentialFieldsForTenant(
			ctx, tenantID, providerCode,
		)
		if errors.Is(err, providercredentials.ErrNotFound) {
			err = nil
		}
		if err != nil {
			return Detail{}, err
		}
	}
	for environment, fields := range availability.FieldsByEnvironment {
		detail.AvailableCredentials[environment] = append([]string(nil), fields...)
	}
	pickupSupported := hasPickupCredentialField(provider.CredentialFields) ||
		contains(provider.RequiredScopes, "pickup:write")
	if !pickupSupported {
		return detail, nil
	}
	if !provider.RequiresCredential {
		detail.AutoPickup = true
		detail.AutoPickupStatus = AutoPickupStatusConfigured
		for _, environment := range provider.Environments {
			detail.AutoPickupEnvironments[environment.Code] = AutoPickupEnvironmentStatus{
				Enabled:            true,
				Status:             AutoPickupStatusConfigured,
				MissingCredentials: []string{},
			}
		}
		return detail, nil
	}
	detail.AutoPickupStatus = AutoPickupStatusCredentialMissing
	hasInvalidCredential := false
	for _, environment := range provider.Environments {
		pickupFields := pickupCredentialFieldsForEnvironment(
			provider.CredentialFields,
			environment.Code,
		)
		if len(pickupFields) == 0 {
			continue
		}
		missing := make([]string, 0, len(pickupFields))
		for _, code := range pickupFields {
			if !contains(detail.AvailableCredentials[environment.Code], code) {
				missing = append(missing, code)
			}
		}
		status := AutoPickupEnvironmentStatus{
			Status:             AutoPickupStatusCredentialMissing,
			MissingCredentials: missing,
		}
		if len(missing) == 0 && availability.StatusByEnvironment[environment.Code] == "invalid" {
			status.Status = AutoPickupStatusCredentialInvalidOrExpired
			hasInvalidCredential = true
		} else if len(missing) == 0 {
			status.Enabled = true
			status.Status = AutoPickupStatusConfigured
			detail.AutoPickup = true
		}
		detail.AutoPickupEnvironments[environment.Code] = status
	}
	if detail.AutoPickup {
		detail.AutoPickupStatus = AutoPickupStatusConfigured
	} else if hasInvalidCredential {
		detail.AutoPickupStatus = AutoPickupStatusCredentialInvalidOrExpired
	}
	return detail, nil
}

func hasPickupCredentialField(fields []providercredentials.FieldDefinition) bool {
	for _, field := range fields {
		if contains(field.Capabilities, "pickup:write") {
			return true
		}
	}
	return false
}

func pickupCredentialFieldsForEnvironment(
	fields []providercredentials.FieldDefinition,
	environment string,
) []string {
	result := make([]string, 0)
	for _, field := range providercredentials.FieldsForEnvironment(fields, environment) {
		if contains(field.Capabilities, "pickup:write") {
			result = append(result, field.Code)
		}
	}
	return result
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
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

// ActiveProviderCode returns the merchant's explicitly selected shipping
// integration. Fulfillment uses it to pin a booking to one provider before
// any external side effect is executed.
func (s *Service) ActiveProviderCode(ctx context.Context, tenantID string) (string, error) {
	tenantID = strings.TrimSpace(tenantID)
	if !validTenantID(tenantID) {
		return "", ErrInvalidTenant
	}
	catalog, err := s.repository.Catalog(ctx, tenantID)
	if err != nil {
		return "", err
	}
	if catalog.ActiveProviderCode == nil {
		return "", nil
	}
	return strings.ToLower(strings.TrimSpace(*catalog.ActiveProviderCode)), nil
}

// AllowsPlatformCredential reports whether a merchant explicitly selected the
// built-in Emisell integration. External/BYOK providers must always resolve a
// credential owned by the merchant and are never allowed to borrow this pool.
func (s *Service) AllowsPlatformCredential(
	ctx context.Context,
	tenantID string,
) (bool, error) {
	tenantID = strings.TrimSpace(tenantID)
	if !validTenantID(tenantID) {
		return false, ErrInvalidTenant
	}
	catalog, err := s.repository.Catalog(ctx, tenantID)
	if err != nil {
		return false, err
	}
	return catalog.ActiveProviderCode != nil &&
		*catalog.ActiveProviderCode == EmisellProviderCode, nil
}

// AllowsProviderFallback is shared by the rate and tracking routers. Requests
// without merchant scope are platform operations and may use the platform
// fallback. Merchant requests may do so only while Emisell Kurir is active;
// RajaOngkir BYOK never borrows the Biteship balance.
func (s *Service) AllowsProviderFallback(ctx context.Context) (bool, error) {
	identity, tenantScoped := tenancy.FromContext(ctx)
	if !tenantScoped {
		return true, nil
	}
	return s.AllowsPlatformCredential(ctx, identity.TenantID)
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
