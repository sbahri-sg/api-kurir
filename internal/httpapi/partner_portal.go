package httpapi

import (
	"errors"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/emisell/api-kurir/internal/partnerexplorer"
	"github.com/emisell/api-kurir/internal/partnerpackages"
	"github.com/labstack/echo/v5"
)

const partnerIdentityContextKey = "partner_access_identity"

func registerPartnerPortalRoutes(
	group *echo.Group,
	service *partnerpackages.Service,
	explorerServices ...*partnerexplorer.Service,
) {
	group.Use(partnerAccessKeyMiddleware(service))
	group.GET("/me", partnerPortalMeHandler())
	group.GET("/starter-package", partnerStarterPackageHandler())
	group.GET("/submissions", partnerSubmissionListHandler(service))
	group.POST("/submissions", partnerSubmissionUploadHandler(service))
	group.GET("/submissions/:id", partnerSubmissionGetHandler(service))
	group.GET("/submissions/:id/artifact", partnerSubmissionArtifactHandler(service))
	if len(explorerServices) > 0 && explorerServices[0] != nil {
		explorer := explorerServices[0]
		group.GET("/explorer/credential", partnerExplorerCredentialGetHandler(explorer))
		group.PUT("/explorer/credential", partnerExplorerCredentialPutHandler(explorer))
		group.DELETE("/explorer/credential", partnerExplorerCredentialDeleteHandler(explorer))
		group.PUT("/explorer/credentials/:code", partnerExplorerCredentialProfilePutHandler(explorer))
		group.DELETE("/explorer/credentials/:code", partnerExplorerCredentialProfileDeleteHandler(explorer))
		group.GET("/submissions/:id/explorer", partnerExplorerCatalogHandler(explorer))
		group.POST("/submissions/:id/explorer/execute", partnerExplorerExecuteHandler(explorer))
		group.GET("/submissions/:id/explorer/runs", partnerExplorerRunsHandler(explorer))
	}
}

func partnerStarterPackageHandler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, ok := partnerIdentity(c)
		if !ok {
			return writeError(c, http.StatusUnauthorized, "PARTNER_UNAUTHORIZED", "Konteks partner tidak tersedia.", nil)
		}
		artifact, err := partnerpackages.StarterPackage(identity.ProviderCode, identity.ProviderName)
		if err != nil {
			return writePartnerPackageError(c, err)
		}
		contentDisposition := mime.FormatMediaType("attachment", map[string]string{
			"filename": filepath.Base(artifact.FileName),
		})
		c.Response().Header().Set("Content-Disposition", contentDisposition)
		c.Response().Header().Set("Cache-Control", "private, no-store")
		return c.Blob(http.StatusOK, artifact.ContentType, artifact.Payload)
	}
}

func partnerAccessKeyMiddleware(service *partnerpackages.Service) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			token := bearerToken(c.Request().Header.Get("Authorization"))
			if token == "" {
				token = strings.TrimSpace(c.Request().Header.Get("key"))
			}
			identity, err := service.AuthenticateAccessKey(c.Request().Context(), token)
			if errors.Is(err, partnerpackages.ErrUnauthorized) {
				return writeError(c, http.StatusUnauthorized, "PARTNER_UNAUTHORIZED", "Partner access key tidak valid atau sudah dicabut.", nil)
			}
			if err != nil {
				return err
			}
			c.Set(partnerIdentityContextKey, identity)
			return next(c)
		}
	}
}

func partnerIdentity(c *echo.Context) (partnerpackages.AccessIdentity, bool) {
	identity, ok := c.Get(partnerIdentityContextKey).(partnerpackages.AccessIdentity)
	return identity, ok
}

func partnerPortalMeHandler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, ok := partnerIdentity(c)
		if !ok {
			return writeError(c, http.StatusUnauthorized, "PARTNER_UNAUTHORIZED", "Konteks partner tidak tersedia.", nil)
		}
		return c.JSON(http.StatusOK, adminResponse(c, identity))
	}
}

func partnerSubmissionListHandler(service *partnerpackages.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, ok := partnerIdentity(c)
		if !ok {
			return writeError(c, http.StatusUnauthorized, "PARTNER_UNAUTHORIZED", "Konteks partner tidak tersedia.", nil)
		}
		limit, offset, err := adminPagination(c)
		if err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		items, err := service.ListForProvider(
			c.Request().Context(), identity.ProviderCode, c.QueryParam("status"), limit, offset,
		)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, adminListResponse(c, items, limit, offset))
	}
}

func partnerSubmissionUploadHandler(service *partnerpackages.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, ok := partnerIdentity(c)
		if !ok {
			return writeError(c, http.StatusUnauthorized, "PARTNER_UNAUTHORIZED", "Konteks partner tidak tersedia.", nil)
		}
		release, err := service.BeginUpload(c.Request().Context(), identity.KeyID)
		if err != nil {
			return writePartnerPackageError(c, err)
		}
		defer release()
		c.Request().Body = http.MaxBytesReader(
			c.Response(), c.Request().Body, maxPartnerPackageRequestBytes,
		)
		input, err := readPartnerPackageMultipart(c)
		if err != nil {
			var maximumError *http.MaxBytesError
			if errors.As(err, &maximumError) {
				return writeError(c, http.StatusRequestEntityTooLarge, "PARTNER_PACKAGE_TOO_LARGE", "Ukuran request package melebihi 25 MB.", nil)
			}
			return writeError(c, http.StatusBadRequest, "INVALID_PARTNER_PACKAGE", err.Error(), nil)
		}
		input.ProviderCode = identity.ProviderCode
		input.SubmittedBy = partnerActor(identity)
		input.RequestID = requestID(c)
		item, err := service.Upload(c.Request().Context(), input)
		if err != nil {
			return writePartnerPackageError(c, err)
		}
		return c.JSON(http.StatusCreated, adminResponse(c, item))
	}
}

func partnerSubmissionGetHandler(service *partnerpackages.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, ok := partnerIdentity(c)
		if !ok {
			return writeError(c, http.StatusUnauthorized, "PARTNER_UNAUTHORIZED", "Konteks partner tidak tersedia.", nil)
		}
		item, err := service.GetForProvider(c.Request().Context(), c.Param("id"), identity.ProviderCode)
		if err != nil {
			return writePartnerPackageError(c, err)
		}
		return c.JSON(http.StatusOK, adminResponse(c, item))
	}
}

func partnerSubmissionArtifactHandler(service *partnerpackages.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		identity, ok := partnerIdentity(c)
		if !ok {
			return writeError(c, http.StatusUnauthorized, "PARTNER_UNAUTHORIZED", "Konteks partner tidak tersedia.", nil)
		}
		artifact, err := service.ArtifactForProvider(
			c.Request().Context(), c.Param("id"), identity.ProviderCode, partnerActor(identity), requestID(c),
		)
		if err != nil {
			return writePartnerPackageError(c, err)
		}
		contentDisposition := mime.FormatMediaType("attachment", map[string]string{
			"filename": filepath.Base(artifact.FileName),
		})
		c.Response().Header().Set("Content-Disposition", contentDisposition)
		c.Response().Header().Set("Cache-Control", "private, no-store")
		return c.Blob(http.StatusOK, artifact.ContentType, artifact.Payload)
	}
}

func partnerActor(identity partnerpackages.AccessIdentity) string {
	keyID := identity.KeyID
	if len(keyID) > 8 {
		keyID = keyID[:8]
	}
	return "partner:" + identity.ProviderCode + ":" + keyID
}
