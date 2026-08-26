package httpapi

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/emisell/api-kurir/internal/partnerpackages"
	"github.com/labstack/echo/v5"
)

const validPartnerTestKey = "epk_live_abcdefghijklmnopqrstuvwxyz0123456789"

type partnerPortalRepository struct {
	listedProvider string
	reserveError   error
}

func (r *partnerPortalRepository) List(_ context.Context, filter partnerpackages.Filter) ([]partnerpackages.Submission, error) {
	r.listedProvider = filter.ProviderCode
	return []partnerpackages.Submission{{ProviderCode: filter.ProviderCode}}, nil
}

func (r *partnerPortalRepository) Get(context.Context, string) (partnerpackages.Submission, error) {
	return partnerpackages.Submission{}, partnerpackages.ErrNotFound
}

func (r *partnerPortalRepository) Create(context.Context, partnerpackages.UploadInput, string, partnerpackages.ScanReport, []string, string) (partnerpackages.Submission, error) {
	return partnerpackages.Submission{}, nil
}

func (r *partnerPortalRepository) UpdateStatus(context.Context, string, partnerpackages.StatusUpdate) (partnerpackages.Submission, error) {
	return partnerpackages.Submission{}, nil
}

func (r *partnerPortalRepository) Artifact(context.Context, string, string, string) (partnerpackages.Artifact, error) {
	return partnerpackages.Artifact{}, partnerpackages.ErrNotFound
}

func (r *partnerPortalRepository) ListAccessKeys(context.Context, string) ([]partnerpackages.AccessKey, error) {
	return nil, nil
}

func (r *partnerPortalRepository) CreateAccessKey(context.Context, partnerpackages.AccessKeyCreateInput) (partnerpackages.AccessKey, error) {
	return partnerpackages.AccessKey{}, nil
}

func (r *partnerPortalRepository) RevokeAccessKey(context.Context, partnerpackages.AccessKeyRevokeInput) (partnerpackages.AccessKey, error) {
	return partnerpackages.AccessKey{}, nil
}

func (r *partnerPortalRepository) AuthenticateAccessKey(context.Context, string) (partnerpackages.AccessIdentity, error) {
	return partnerpackages.AccessIdentity{
		KeyID:        "12345678-1234-1234-9234-123456789012",
		ProviderCode: "mengantar",
		ProviderName: "Mengantar",
	}, nil
}

func (r *partnerPortalRepository) ReserveUploadAttempt(context.Context, string, int) error {
	return r.reserveError
}

func TestPartnerPortalListAlwaysUsesProviderFromAccessKey(t *testing.T) {
	repository := &partnerPortalRepository{}
	service := partnerpackages.NewService(repository)
	e := echo.New()
	registerPartnerPortalRoutes(e.Group("/partner/v1"), service)

	request := httptest.NewRequest(http.MethodGet, "/partner/v1/submissions?provider_code=other", nil)
	request.Header.Set("Authorization", "Bearer "+validPartnerTestKey)
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if repository.listedProvider != "mengantar" {
		t.Fatalf("provider scope=%q want mengantar", repository.listedProvider)
	}
}

func TestPartnerPortalUploadRejectsProviderCodeField(t *testing.T) {
	repository := &partnerPortalRepository{}
	service := partnerpackages.NewService(repository)
	e := echo.New()
	registerPartnerPortalRoutes(e.Group("/partner/v1"), service)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("provider_code", "other")
	_ = writer.WriteField("version", "1.0.0")
	file, err := writer.CreateFormFile("package", "connector.zip")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("not important for rejected request"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/partner/v1/submissions", &body)
	request.Header.Set("Authorization", "Bearer "+validPartnerTestKey)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestPartnerPortalRequiresScopedAccessKey(t *testing.T) {
	repository := &partnerPortalRepository{}
	service := partnerpackages.NewService(repository)
	e := echo.New()
	registerPartnerPortalRoutes(e.Group("/partner/v1"), service)

	request := httptest.NewRequest(http.MethodGet, "/partner/v1/me", nil)
	request.Header.Set("Authorization", "Bearer admin-key")
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestPartnerPortalDownloadsScopedStarterPackage(t *testing.T) {
	repository := &partnerPortalRepository{}
	service := partnerpackages.NewService(repository)
	e := echo.New()
	registerPartnerPortalRoutes(e.Group("/partner/v1"), service)

	request := httptest.NewRequest(http.MethodGet, "/partner/v1/starter-package", nil)
	request.Header.Set("Authorization", "Bearer "+validPartnerTestKey)
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("content type=%q", response.Header().Get("Content-Type"))
	}
	report := partnerpackages.ValidateArchive(response.Body.Bytes(), "mengantar")
	if !report.Passed || report.Manifest.ProviderCode != "mengantar" {
		t.Fatalf("invalid scoped starter package: %#v", report)
	}
}

func TestPartnerPortalUploadReturnsRateLimitBeforeReadingBody(t *testing.T) {
	repository := &partnerPortalRepository{reserveError: partnerpackages.ErrUploadRateLimit}
	service := partnerpackages.NewService(repository)
	e := echo.New()
	registerPartnerPortalRoutes(e.Group("/partner/v1"), service)

	request := httptest.NewRequest(http.MethodPost, "/partner/v1/submissions", nil)
	request.Header.Set("Authorization", "Bearer "+validPartnerTestKey)
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)

	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Retry-After") != "3600" {
		t.Fatalf("retry-after=%q", response.Header().Get("Retry-After"))
	}
}
