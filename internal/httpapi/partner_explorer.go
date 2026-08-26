package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/emisell/api-kurir/internal/partnerexplorer"
	"github.com/labstack/echo/v5"
)

type partnerExplorerCredentialRequest struct {
	OfficialAPIKey string `json:"official_api_key"`
	AuthHeader     string `json:"auth_header"`
	AuthPrefix     string `json:"auth_prefix"`
}

type partnerExplorerExecuteRequest struct {
	OperationID string            `json:"operation_id"`
	PathParams  map[string]string `json:"path_params"`
	Query       map[string]string `json:"query"`
	Body        json.RawMessage   `json:"body"`
}

func partnerExplorerCredentialGetHandler(service *partnerexplorer.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, ok := partnerIdentity(c)
		if !ok {
			return writeError(c, http.StatusUnauthorized, "PARTNER_UNAUTHORIZED", "Konteks partner tidak tersedia.", nil)
		}
		state, err := service.CredentialState(c.Request().Context(), identity.ProviderCode, "default")
		if err != nil {
			return writePartnerExplorerError(c, err)
		}
		c.Response().Header().Set("Cache-Control", "private, no-store")
		return c.JSON(http.StatusOK, adminResponse(c, state))
	}
}

func partnerExplorerCredentialPutHandler(service *partnerexplorer.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, ok := partnerIdentity(c)
		if !ok {
			return writeError(c, http.StatusUnauthorized, "PARTNER_UNAUTHORIZED", "Konteks partner tidak tersedia.", nil)
		}
		var request partnerExplorerCredentialRequest
		if err := decodeAdminJSON(c, &request); err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		state, err := service.SaveCredential(c.Request().Context(), partnerexplorer.CredentialInput{
			ProviderCode:   identity.ProviderCode,
			CredentialCode: "default",
			Secret:         request.OfficialAPIKey,
			AuthHeader:     request.AuthHeader,
			AuthPrefix:     request.AuthPrefix,
			Actor:          partnerActor(identity),
			RequestID:      requestID(c),
		})
		if err != nil {
			return writePartnerExplorerError(c, err)
		}
		c.Response().Header().Set("Cache-Control", "private, no-store")
		return c.JSON(http.StatusOK, adminResponse(c, state))
	}
}

func partnerExplorerCredentialDeleteHandler(service *partnerexplorer.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, ok := partnerIdentity(c)
		if !ok {
			return writeError(c, http.StatusUnauthorized, "PARTNER_UNAUTHORIZED", "Konteks partner tidak tersedia.", nil)
		}
		err := service.DeleteCredential(
			c.Request().Context(), identity.ProviderCode,
			"default",
			partnerActor(identity), requestID(c),
		)
		if err != nil {
			return writePartnerExplorerError(c, err)
		}
		return c.NoContent(http.StatusNoContent)
	}
}

func partnerExplorerCredentialProfilePutHandler(service *partnerexplorer.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, ok := partnerIdentity(c)
		if !ok {
			return writeError(c, http.StatusUnauthorized, "PARTNER_UNAUTHORIZED", "Konteks partner tidak tersedia.", nil)
		}
		var request partnerExplorerCredentialRequest
		if err := decodeAdminJSON(c, &request); err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		state, err := service.SaveCredential(c.Request().Context(), partnerexplorer.CredentialInput{
			ProviderCode: identity.ProviderCode, CredentialCode: c.Param("code"),
			Secret: request.OfficialAPIKey, AuthHeader: request.AuthHeader,
			AuthPrefix: request.AuthPrefix, Actor: partnerActor(identity), RequestID: requestID(c),
		})
		if err != nil {
			return writePartnerExplorerError(c, err)
		}
		c.Response().Header().Set("Cache-Control", "private, no-store")
		return c.JSON(http.StatusOK, adminResponse(c, state))
	}
}

func partnerExplorerCredentialProfileDeleteHandler(service *partnerexplorer.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, ok := partnerIdentity(c)
		if !ok {
			return writeError(c, http.StatusUnauthorized, "PARTNER_UNAUTHORIZED", "Konteks partner tidak tersedia.", nil)
		}
		err := service.DeleteCredential(
			c.Request().Context(), identity.ProviderCode, c.Param("code"),
			partnerActor(identity), requestID(c),
		)
		if err != nil {
			return writePartnerExplorerError(c, err)
		}
		return c.NoContent(http.StatusNoContent)
	}
}

func partnerExplorerCatalogHandler(service *partnerexplorer.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, ok := partnerIdentity(c)
		if !ok {
			return writeError(c, http.StatusUnauthorized, "PARTNER_UNAUTHORIZED", "Konteks partner tidak tersedia.", nil)
		}
		catalog, err := service.Catalog(
			c.Request().Context(), c.Param("id"), identity.ProviderCode,
		)
		if err != nil {
			return writePartnerExplorerError(c, err)
		}
		c.Response().Header().Set("Cache-Control", "private, no-store")
		return c.JSON(http.StatusOK, adminResponse(c, catalog))
	}
}

func partnerExplorerExecuteHandler(service *partnerexplorer.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, ok := partnerIdentity(c)
		if !ok {
			return writeError(c, http.StatusUnauthorized, "PARTNER_UNAUTHORIZED", "Konteks partner tidak tersedia.", nil)
		}
		var request partnerExplorerExecuteRequest
		if err := decodeAdminJSON(c, &request); err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		result, err := service.Execute(c.Request().Context(), partnerexplorer.ExecuteInput{
			SubmissionID: c.Param("id"), ProviderCode: identity.ProviderCode,
			KeyID: identity.KeyID, OperationID: request.OperationID,
			PathParams: request.PathParams, Query: request.Query, Body: request.Body,
			Actor: partnerActor(identity), RequestID: requestID(c),
		})
		if err != nil {
			return writePartnerExplorerError(c, err)
		}
		c.Response().Header().Set("Cache-Control", "private, no-store")
		return c.JSON(http.StatusOK, adminResponse(c, result))
	}
}

func partnerExplorerRunsHandler(service *partnerexplorer.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, ok := partnerIdentity(c)
		if !ok {
			return writeError(c, http.StatusUnauthorized, "PARTNER_UNAUTHORIZED", "Konteks partner tidak tersedia.", nil)
		}
		limit := 25
		if raw := strings.TrimSpace(c.QueryParam("limit")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 1 || parsed > 100 {
				return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "limit harus antara 1 dan 100.", nil)
			}
			limit = parsed
		}
		items, err := service.ListRuns(
			c.Request().Context(), c.Param("id"), identity.ProviderCode, limit,
		)
		if err != nil {
			return writePartnerExplorerError(c, err)
		}
		c.Response().Header().Set("Cache-Control", "private, no-store")
		return c.JSON(http.StatusOK, adminResponse(c, items))
	}
}

func writePartnerExplorerError(c *echo.Context, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, partnerexplorer.ErrInvalidInput):
		return writeError(c, http.StatusBadRequest, "INVALID_EXPLORER_REQUEST", "Konfigurasi atau payload pengujian tidak valid.", nil)
	case errors.Is(err, partnerexplorer.ErrSubmissionNotFound):
		return writeError(c, http.StatusNotFound, "PARTNER_SUBMISSION_NOT_FOUND", "Submission tidak ditemukan untuk provider ini.", nil)
	case errors.Is(err, partnerexplorer.ErrCredentialNotFound):
		return writeError(c, http.StatusConflict, "OFFICIAL_CREDENTIAL_REQUIRED", "Hubungkan API key resmi provider sebelum menjalankan pengujian.", nil)
	case errors.Is(err, partnerexplorer.ErrOperationNotFound):
		return writeError(c, http.StatusNotFound, "EXPLORER_OPERATION_NOT_FOUND", "Endpoint tidak ditemukan pada OpenAPI package.", nil)
	case errors.Is(err, partnerexplorer.ErrOperationLocked):
		return writeError(c, http.StatusForbidden, "OFFICIAL_OPERATION_LOCKED", "Endpoint transaksi dikunci untuk mencegah shipment atau pickup live tanpa persetujuan.", nil)
	case errors.Is(err, partnerexplorer.ErrRateLimit):
		c.Response().Header().Set("Retry-After", "3600")
		return writeError(c, http.StatusTooManyRequests, "EXPLORER_RATE_LIMITED", "Batas 120 pengujian per jam untuk Partner Access Key ini telah tercapai.", nil)
	case errors.Is(err, partnerexplorer.ErrTargetBlocked):
		return writeError(c, http.StatusUnprocessableEntity, "EXPLORER_TARGET_BLOCKED", "Host atau jaringan tujuan diblokir oleh kebijakan keamanan Explorer.", nil)
	case errors.Is(err, partnerexplorer.ErrConnectorNotConfigured):
		return writeError(c, http.StatusConflict, "CONNECTOR_NOT_CONFIGURED", "URL connector masih berupa placeholder. Pasang URL HTTPS connector yang aktif sebelum menjalankan pengujian.", nil)
	case errors.Is(err, partnerexplorer.ErrCredentialMismatch):
		return writeError(c, http.StatusConflict, "EXPLORER_CREDENTIAL_MISMATCH", "Credential tidak sesuai dengan security scheme endpoint pada OpenAPI.", nil)
	default:
		return err
	}
}
