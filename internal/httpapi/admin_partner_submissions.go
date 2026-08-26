package httpapi

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/emisell/api-kurir/internal/partnerpackages"
	"github.com/labstack/echo/v5"
)

const maxPartnerPackageRequestBytes = partnerpackages.MaxArtifactBytes + 64*1024

type partnerSubmissionStatusRequest struct {
	Status     string `json:"status"`
	ReviewNote string `json:"review_note"`
}

func adminPartnerSubmissionListHandler(service *partnerpackages.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		limit, offset, err := adminPagination(c)
		if err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		result, err := service.List(c.Request().Context(), partnerpackages.Filter{
			ProviderCode: c.QueryParam("provider_code"),
			Status:       c.QueryParam("status"),
			Limit:        limit,
			Offset:       offset,
		})
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, adminListResponse(c, result, limit, offset))
	}
}

func adminPartnerSubmissionGetHandler(service *partnerpackages.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		id := strings.TrimSpace(c.Param("id"))
		if !validUUID.MatchString(id) {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "ID submission tidak valid.", nil)
		}
		result, err := service.Get(c.Request().Context(), id)
		if err != nil {
			return writePartnerPackageError(c, err)
		}
		return c.JSON(http.StatusOK, adminResponse(c, result))
	}
}

func adminPartnerSubmissionArtifactHandler(service *partnerpackages.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		id := strings.TrimSpace(c.Param("id"))
		if !validUUID.MatchString(id) {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "ID submission tidak valid.", nil)
		}
		artifact, err := service.Artifact(
			c.Request().Context(), id, adminActor(c), requestID(c),
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

func adminPartnerSubmissionStatusHandler(service *partnerpackages.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		id := strings.TrimSpace(c.Param("id"))
		if !validUUID.MatchString(id) {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "ID submission tidak valid.", nil)
		}
		var request partnerSubmissionStatusRequest
		if err := decodeAdminJSON(c, &request); err != nil {
			return writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
		}
		result, err := service.UpdateStatus(c.Request().Context(), id, partnerpackages.StatusUpdate{
			Status: request.Status, ReviewNote: request.ReviewNote,
			ReviewedBy: adminActor(c), RequestID: requestID(c),
		})
		if err != nil {
			return writePartnerPackageError(c, err)
		}
		return c.JSON(http.StatusOK, adminResponse(c, result))
	}
}

func readPartnerPackageMultipart(c *echo.Context) (partnerpackages.UploadInput, error) {
	reader, err := c.Request().MultipartReader()
	if err != nil {
		return partnerpackages.UploadInput{}, errors.New("request wajib multipart/form-data")
	}

	var input partnerpackages.UploadInput
	seen := make(map[string]bool)
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return partnerpackages.UploadInput{}, fmt.Errorf("multipart package tidak dapat dibaca: %w", nextErr)
		}
		fieldName := part.FormName()
		if fieldName == "" {
			_ = part.Close()
			return partnerpackages.UploadInput{}, errors.New("multipart field tidak memiliki nama")
		}
		if seen[fieldName] {
			_ = part.Close()
			return partnerpackages.UploadInput{}, fmt.Errorf("field %s hanya boleh dikirim satu kali", fieldName)
		}
		seen[fieldName] = true

		switch fieldName {
		case "provider_code":
			err = errors.New("field provider_code tidak didukung; provider ditentukan oleh partner access key")
		case "version":
			input.Version, err = readSmallMultipartField(part)
		case "package":
			input.FileName = filepath.Base(strings.TrimSpace(part.FileName()))
			if input.FileName == "." || input.FileName == "" {
				err = errors.New("package wajib berupa file ZIP")
				break
			}
			input.Payload, err = io.ReadAll(io.LimitReader(part, partnerpackages.MaxArtifactBytes+1))
			if err == nil && len(input.Payload) > partnerpackages.MaxArtifactBytes {
				err = errors.New("ukuran package melebihi 25 MB")
			}
		default:
			err = fmt.Errorf("field %s tidak didukung", fieldName)
		}
		_ = part.Close()
		if err != nil {
			return partnerpackages.UploadInput{}, err
		}
	}
	if !seen["version"] || !seen["package"] {
		return partnerpackages.UploadInput{}, errors.New("version dan package wajib diisi")
	}
	return input, nil
}

func adminPartnerAccessKeyListHandler(service *partnerpackages.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		items, err := service.ListAccessKeys(c.Request().Context(), c.Param("code"))
		if err != nil {
			return writePartnerPackageError(c, err)
		}
		return c.JSON(http.StatusOK, adminResponse(c, items))
	}
}

func adminPartnerAccessKeyCreateHandler(service *partnerpackages.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		generated, err := service.GenerateAccessKey(
			c.Request().Context(), c.Param("code"), adminActor(c), requestID(c),
		)
		if err != nil {
			return writePartnerPackageError(c, err)
		}
		return c.JSON(http.StatusCreated, adminResponse(c, generated))
	}
}

func adminPartnerAccessKeyRevokeHandler(service *partnerpackages.Service) echo.HandlerFunc {
	return func(c *echo.Context) error {
		item, err := service.RevokeAccessKey(
			c.Request().Context(), c.Param("code"), c.Param("id"), adminActor(c), requestID(c),
		)
		if err != nil {
			return writePartnerPackageError(c, err)
		}
		return c.JSON(http.StatusOK, adminResponse(c, item))
	}
}

func readSmallMultipartField(reader io.Reader) (string, error) {
	payload, err := io.ReadAll(io.LimitReader(reader, 1025))
	if err != nil {
		return "", fmt.Errorf("multipart field tidak dapat dibaca: %w", err)
	}
	if len(payload) > 1024 {
		return "", errors.New("multipart field terlalu panjang")
	}
	return strings.TrimSpace(string(payload)), nil
}

func writePartnerPackageError(c *echo.Context, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, partnerpackages.ErrInvalidInput):
		return writeError(c, http.StatusBadRequest, "INVALID_PARTNER_PACKAGE", "Metadata atau file ZIP tidak valid.", nil)
	case errors.Is(err, partnerpackages.ErrNotFound):
		return writeError(c, http.StatusNotFound, "PARTNER_SUBMISSION_NOT_FOUND", "Provider atau submission tidak ditemukan.", nil)
	case errors.Is(err, partnerpackages.ErrConflict):
		return writeError(c, http.StatusConflict, "PARTNER_VERSION_EXISTS", "Versi package untuk provider tersebut sudah pernah dikirim.", nil)
	case errors.Is(err, partnerpackages.ErrStatusConflict):
		return writeError(c, http.StatusConflict, "PARTNER_STATUS_CHANGED", "Status submission sudah berubah. Muat ulang sebelum melanjutkan review.", nil)
	case errors.Is(err, partnerpackages.ErrInvalidTransition):
		return writeError(c, http.StatusConflict, "INVALID_SUBMISSION_TRANSITION", "Perubahan status tidak sesuai lifecycle review.", nil)
	case errors.Is(err, partnerpackages.ErrUnauthorized):
		return writeError(c, http.StatusUnauthorized, "PARTNER_UNAUTHORIZED", "Partner access key tidak valid atau sudah dicabut.", nil)
	case errors.Is(err, partnerpackages.ErrProviderType):
		return writeError(c, http.StatusConflict, "PARTNER_PROVIDER_TYPE_INVALID", "Portal package hanya tersedia untuk provider partner-hosted.", nil)
	case errors.Is(err, partnerpackages.ErrUploadRateLimit):
		c.Response().Header().Set("Retry-After", "3600")
		return writeError(c, http.StatusTooManyRequests, "PARTNER_UPLOAD_RATE_LIMITED", "Batas 10 percobaan upload per jam untuk access key ini telah tercapai.", nil)
	case errors.Is(err, partnerpackages.ErrUploadInProgress):
		return writeError(c, http.StatusConflict, "PARTNER_UPLOAD_IN_PROGRESS", "Masih ada upload lain yang sedang diproses untuk access key ini.", nil)
	case errors.Is(err, partnerpackages.ErrStorageQuota):
		return writeError(c, http.StatusConflict, "PARTNER_STORAGE_QUOTA_EXCEEDED", "Kuota package provider telah tercapai. Hubungi staff Emisell untuk meninjau versi lama.", nil)
	default:
		return err
	}
}
