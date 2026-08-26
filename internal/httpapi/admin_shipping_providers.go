package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/emisell/api-kurir/internal/admin"
	"github.com/emisell/api-kurir/internal/providercredentials"
	"github.com/labstack/echo/v5"
)

var validShippingProviderCode = regexp.MustCompile(`^[a-z0-9_-]{2,48}$`)

type createShippingProviderRequest struct {
	Code            string `json:"code"`
	Name            string `json:"name"`
	Logo            string `json:"logo"`
	Description     string `json:"description"`
	IntegrationType string `json:"integration_type"`
	CredentialType  string `json:"credential_type"`
	DisplayOrder    int    `json:"display_order"`
}

type updateShippingProviderRequest struct {
	Name            string `json:"name"`
	Logo            string `json:"logo"`
	Description     string `json:"description"`
	IntegrationType string `json:"integration_type"`
	CredentialType  string `json:"credential_type"`
	Available       bool   `json:"available"`
	DisplayOrder    int    `json:"display_order"`
}

func adminShippingProviderListHandler(repository admin.Repository) echo.HandlerFunc {
	return func(c *echo.Context) error {
		result, err := repository.ListShippingProviders(c.Request().Context())
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, adminResponse(c, result))
	}
}

func adminShippingProviderCreateHandler(repository admin.Repository) echo.HandlerFunc {
	return func(c *echo.Context) error {
		var request createShippingProviderRequest
		if err := decodeAdminJSON(c, &request); err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		input, err := normalizeShippingProviderCreateInput(request)
		if err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		result, err := repository.CreateShippingProvider(
			c.Request().Context(), input, adminActor(c), requestID(c),
		)
		if response := writeAdminRepositoryError(c, err); response != nil {
			return response
		}
		return c.JSON(http.StatusCreated, adminResponse(c, result))
	}
}

func adminShippingProviderUpdateHandler(repository admin.Repository) echo.HandlerFunc {
	return func(c *echo.Context) error {
		code := strings.ToLower(strings.TrimSpace(c.Param("code")))
		if !validShippingProviderCode.MatchString(code) {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "Kode provider tidak valid.", nil)
		}
		var request updateShippingProviderRequest
		if err := decodeAdminJSON(c, &request); err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		input, err := normalizeShippingProviderUpdateInput(request)
		if err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		result, err := repository.UpdateShippingProvider(
			c.Request().Context(), code, input, adminActor(c), requestID(c),
		)
		if response := writeAdminRepositoryError(c, err); response != nil {
			return response
		}
		return c.JSON(http.StatusOK, adminResponse(c, result))
	}
}

func adminShippingProviderDeleteHandler(repository admin.Repository) echo.HandlerFunc {
	return func(c *echo.Context) error {
		code := strings.ToLower(strings.TrimSpace(c.Param("code")))
		if !validShippingProviderCode.MatchString(code) {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "Kode provider tidak valid.", nil)
		}
		err := repository.DeleteShippingProvider(
			c.Request().Context(), code, adminActor(c), requestID(c),
		)
		if response := writeAdminRepositoryError(c, err); response != nil {
			return response
		}
		return c.NoContent(http.StatusNoContent)
	}
}

func normalizeShippingProviderCreateInput(
	request createShippingProviderRequest,
) (admin.ShippingProviderCreateInput, error) {
	request.Code = strings.ToLower(strings.TrimSpace(request.Code))
	if !validShippingProviderCode.MatchString(request.Code) {
		return admin.ShippingProviderCreateInput{}, errors.New("code hanya boleh berisi huruf kecil, angka, tanda hubung, atau garis bawah")
	}
	metadata, err := normalizeShippingProviderMetadata(
		request.Name, request.Logo, request.Description, request.DisplayOrder,
	)
	if err != nil {
		return admin.ShippingProviderCreateInput{}, err
	}
	integrationType, distributionType, err := normalizeProviderClassification(
		request.IntegrationType, false,
	)
	if err != nil {
		return admin.ShippingProviderCreateInput{}, err
	}
	credentialType, err := normalizeProviderCredentialType(
		request.CredentialType, integrationType, false,
	)
	if err != nil {
		return admin.ShippingProviderCreateInput{}, err
	}
	return admin.ShippingProviderCreateInput{
		Code:             request.Code,
		Name:             metadata.Name,
		Logo:             metadata.Logo,
		Description:      metadata.Description,
		IntegrationType:  integrationType,
		DistributionType: distributionType,
		CredentialType:   credentialType,
		DisplayOrder:     metadata.DisplayOrder,
	}, nil
}

func normalizeShippingProviderUpdateInput(
	request updateShippingProviderRequest,
) (admin.ShippingProviderUpdateInput, error) {
	metadata, err := normalizeShippingProviderMetadata(
		request.Name, request.Logo, request.Description, request.DisplayOrder,
	)
	if err != nil {
		return admin.ShippingProviderUpdateInput{}, err
	}
	integrationType, distributionType, err := normalizeProviderClassification(
		request.IntegrationType, true,
	)
	if err != nil {
		return admin.ShippingProviderUpdateInput{}, err
	}
	credentialType, err := normalizeProviderCredentialType(
		request.CredentialType, integrationType, true,
	)
	if err != nil {
		return admin.ShippingProviderUpdateInput{}, err
	}
	return admin.ShippingProviderUpdateInput{
		Name:             metadata.Name,
		Logo:             metadata.Logo,
		Description:      metadata.Description,
		IntegrationType:  integrationType,
		DistributionType: distributionType,
		CredentialType:   credentialType,
		Available:        request.Available,
		DisplayOrder:     metadata.DisplayOrder,
	}, nil
}

func normalizeProviderCredentialType(
	credentialType string,
	integrationType string,
	allowEmpty bool,
) (string, error) {
	credentialType = strings.ToLower(strings.TrimSpace(credentialType))
	if credentialType == "" && allowEmpty {
		return "", nil
	}
	if integrationType == "built_in" {
		if credentialType == "" || credentialType == providercredentials.CredentialTypeNone {
			return providercredentials.CredentialTypeNone, nil
		}
		return "", errors.New("provider built-in tidak dapat memakai credential seller")
	}
	if credentialType == "" && integrationType == "partner_hosted" {
		return providercredentials.CredentialTypeNone, nil
	}
	if credentialType == "" {
		credentialType = providercredentials.CredentialTypeAPIKey
	}
	normalized, valid := providercredentials.NormalizeCredentialType(credentialType)
	if !valid || (integrationType == "managed_upstream" && normalized == providercredentials.CredentialTypeNone) {
		return "", errors.New("model credential provider tidak valid")
	}
	return normalized, nil
}

func normalizeProviderClassification(
	integrationType string,
	allowEmpty bool,
) (string, string, error) {
	integrationType = strings.ToLower(strings.TrimSpace(integrationType))
	if !allowEmpty {
		if integrationType == "" {
			integrationType = "partner_hosted"
		}
	}
	validIntegrationTypes := map[string]bool{
		"": allowEmpty, "managed_upstream": true, "partner_hosted": true,
	}
	if allowEmpty {
		validIntegrationTypes["built_in"] = true
	}
	if !validIntegrationTypes[integrationType] {
		return "", "", errors.New("jenis integrasi wajib managed_upstream atau partner_hosted")
	}
	if integrationType == "" {
		return "", "", nil
	}
	if integrationType == "built_in" {
		return integrationType, "built_in", nil
	}
	return integrationType, "merchant", nil
}

func normalizeShippingProviderMetadata(
	name string,
	logo string,
	description string,
	displayOrder int,
) (admin.ShippingProviderUpdateInput, error) {
	name = strings.TrimSpace(name)
	logo = strings.TrimSpace(logo)
	description = strings.TrimSpace(description)
	if len(name) < 2 || len(name) > 100 {
		return admin.ShippingProviderUpdateInput{}, errors.New("nama provider harus 2 sampai 100 karakter")
	}
	parsedLogo, err := url.ParseRequestURI(logo)
	if err != nil || parsedLogo.Scheme != "https" || parsedLogo.Host == "" || parsedLogo.User != nil || len(logo) > 2048 {
		return admin.ShippingProviderUpdateInput{}, errors.New("logo wajib berupa URL HTTPS publik yang valid")
	}
	if len(description) < 10 || len(description) > 500 {
		return admin.ShippingProviderUpdateInput{}, errors.New("deskripsi provider harus 10 sampai 500 karakter")
	}
	if strings.ContainsAny(description, "<>") {
		return admin.ShippingProviderUpdateInput{}, errors.New("deskripsi provider tidak boleh berisi HTML")
	}
	if displayOrder < 1 || displayOrder > 9999 {
		return admin.ShippingProviderUpdateInput{}, errors.New("urutan tampil harus antara 1 dan 9999")
	}
	return admin.ShippingProviderUpdateInput{
		Name: name, Logo: logo, Description: description, DisplayOrder: displayOrder,
	}, nil
}
