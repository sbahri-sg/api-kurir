package partnerpackages

import (
	"context"
	"errors"
)

var (
	ErrNotFound         = errors.New("partner integration submission not found")
	ErrConflict         = errors.New("partner integration submission conflict")
	ErrStatusConflict   = errors.New("partner integration submission status changed")
	ErrProviderType     = errors.New("provider does not use partner-hosted integration")
	ErrUploadRateLimit  = errors.New("partner upload rate limit exceeded")
	ErrUploadInProgress = errors.New("partner upload already in progress")
	ErrStorageQuota     = errors.New("partner package storage quota exceeded")
)

type Repository interface {
	List(ctx context.Context, filter Filter) ([]Submission, error)
	Get(ctx context.Context, id string) (Submission, error)
	Create(
		ctx context.Context,
		input UploadInput,
		sha256 string,
		report ScanReport,
		requiredScopes []string,
		status string,
	) (Submission, error)
	UpdateStatus(ctx context.Context, id string, update StatusUpdate) (Submission, error)
	Artifact(ctx context.Context, id, actor, requestID string) (Artifact, error)
	ListAccessKeys(ctx context.Context, providerCode string) ([]AccessKey, error)
	CreateAccessKey(ctx context.Context, input AccessKeyCreateInput) (AccessKey, error)
	RevokeAccessKey(ctx context.Context, input AccessKeyRevokeInput) (AccessKey, error)
	AuthenticateAccessKey(ctx context.Context, secretHash string) (AccessIdentity, error)
	ReserveUploadAttempt(ctx context.Context, keyID string, maximum int) error
}
