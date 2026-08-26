package partnerpackages

import (
	"context"
	"errors"
	"testing"
)

type recordingRepository struct {
	createdStatus string
	createdDigest string
	authHash      string
	reserveError  error
	reservedKeyID string
}

func (r *recordingRepository) List(context.Context, Filter) ([]Submission, error) {
	return nil, nil
}

func (r *recordingRepository) Get(context.Context, string) (Submission, error) {
	return Submission{}, ErrNotFound
}

func (r *recordingRepository) Create(
	_ context.Context,
	input UploadInput,
	digest string,
	report ScanReport,
	requiredScopes []string,
	status string,
) (Submission, error) {
	r.createdStatus = status
	r.createdDigest = digest
	return Submission{
		ProviderCode:   input.ProviderCode,
		Version:        input.Version,
		Status:         status,
		ScanReport:     report,
		RequiredScopes: requiredScopes,
	}, nil
}

func (r *recordingRepository) UpdateStatus(context.Context, string, StatusUpdate) (Submission, error) {
	return Submission{}, nil
}

func (r *recordingRepository) Artifact(context.Context, string, string, string) (Artifact, error) {
	return Artifact{}, nil
}

func (r *recordingRepository) ListAccessKeys(context.Context, string) ([]AccessKey, error) {
	return nil, nil
}

func (r *recordingRepository) CreateAccessKey(_ context.Context, input AccessKeyCreateInput) (AccessKey, error) {
	return AccessKey{ProviderCode: input.ProviderCode, DisplayKey: input.DisplayKey, Active: true}, nil
}

func (r *recordingRepository) RevokeAccessKey(context.Context, AccessKeyRevokeInput) (AccessKey, error) {
	return AccessKey{}, nil
}

func (r *recordingRepository) AuthenticateAccessKey(_ context.Context, secretHash string) (AccessIdentity, error) {
	r.authHash = secretHash
	return AccessIdentity{ProviderCode: "mengantar", ProviderName: "Mengantar"}, nil
}

func (r *recordingRepository) ReserveUploadAttempt(_ context.Context, keyID string, _ int) error {
	r.reservedKeyID = keyID
	return r.reserveError
}

func TestServiceUploadSelectsInitialStatusFromScan(t *testing.T) {
	repository := &recordingRepository{}
	service := NewService(repository)
	result, err := service.Upload(context.Background(), UploadInput{
		ProviderCode: "Mengantar",
		Version:      "1.0.0",
		FileName:     "connector.zip",
		Payload: testArchive(t, map[string]string{
			"emisell-extension.yaml": testManifest("mengantar"),
			"openapi.yaml":           testOpenAPI(),
		}),
		SubmittedBy: "reviewer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "technical_review" || repository.createdStatus != "technical_review" {
		t.Fatalf("unexpected initial status: %q", result.Status)
	}
	if len(repository.createdDigest) != 64 {
		t.Fatalf("unexpected sha256: %q", repository.createdDigest)
	}
	wantScopes := []string{
		"balance:read", "pickups:write", "rates:read",
		"shipments:write", "tracking:read",
	}
	if len(result.RequiredScopes) != len(wantScopes) {
		t.Fatalf("required scopes=%#v want=%#v", result.RequiredScopes, wantScopes)
	}
	for index := range wantScopes {
		if result.RequiredScopes[index] != wantScopes[index] {
			t.Fatalf("required scopes=%#v want=%#v", result.RequiredScopes, wantScopes)
		}
	}
}

func TestAllowedTransitionProtectsCertificationLifecycle(t *testing.T) {
	if !allowedTransition("technical_review", "sandbox_testing") {
		t.Fatal("expected technical review to advance to sandbox")
	}
	if allowedTransition("technical_review", "published") {
		t.Fatal("technical review must not jump directly to published")
	}
	if allowedTransition("rejected", "published") {
		t.Fatal("rejected version must remain terminal")
	}
	if !allowedTransition("superseded", "published") {
		t.Fatal("superseded release must support an explicit rollback")
	}
}

func TestPartnerAccessKeyIsScopedAndStoredAsHash(t *testing.T) {
	repository := &recordingRepository{}
	service := NewService(repository)
	generated, err := service.GenerateAccessKey(
		context.Background(), "Mengantar", "staff", "req_test",
	)
	if err != nil {
		t.Fatal(err)
	}
	if generated.AccessKey.ProviderCode != "mengantar" {
		t.Fatalf("provider scope: got %q", generated.AccessKey.ProviderCode)
	}
	if generated.Secret == "" || generated.AccessKey.DisplayKey == generated.Secret {
		t.Fatal("secret must be returned once and never stored as display value")
	}
	identity, err := service.AuthenticateAccessKey(context.Background(), generated.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if identity.ProviderCode != "mengantar" || len(repository.authHash) != 64 {
		t.Fatalf("unexpected identity/hash: %#v %q", identity, repository.authHash)
	}
}

func TestPartnerAccessKeyRejectsUnknownFormatBeforeRepository(t *testing.T) {
	repository := &recordingRepository{}
	service := NewService(repository)
	_, err := service.AuthenticateAccessKey(context.Background(), "admin-key")
	if err != ErrUnauthorized {
		t.Fatalf("got %v want %v", err, ErrUnauthorized)
	}
	if repository.authHash != "" {
		t.Fatal("invalid token format must not reach repository")
	}
}

func TestBeginUploadReservesRateLimitAndRejectsConcurrentAttempt(t *testing.T) {
	repository := &recordingRepository{}
	service := NewService(repository)
	release, err := service.BeginUpload(context.Background(), "key-1")
	if err != nil {
		t.Fatal(err)
	}
	if repository.reservedKeyID != "key-1" {
		t.Fatalf("reserved key=%q want key-1", repository.reservedKeyID)
	}
	if _, err := service.BeginUpload(context.Background(), "key-1"); !errors.Is(err, ErrUploadInProgress) {
		t.Fatalf("concurrent error=%v want %v", err, ErrUploadInProgress)
	}
	release()
	secondRelease, err := service.BeginUpload(context.Background(), "key-1")
	if err != nil {
		t.Fatalf("slot was not released: %v", err)
	}
	secondRelease()
}

func TestBeginUploadReleasesSlotWhenRateLimited(t *testing.T) {
	repository := &recordingRepository{reserveError: ErrUploadRateLimit}
	service := NewService(repository)
	if _, err := service.BeginUpload(context.Background(), "key-1"); !errors.Is(err, ErrUploadRateLimit) {
		t.Fatalf("rate limit error=%v want %v", err, ErrUploadRateLimit)
	}
	repository.reserveError = nil
	release, err := service.BeginUpload(context.Background(), "key-1")
	if err != nil {
		t.Fatalf("slot remained locked after rate limit: %v", err)
	}
	release()
}
