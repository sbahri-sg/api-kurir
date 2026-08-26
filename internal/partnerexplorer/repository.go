package partnerexplorer

import (
	"context"
	"errors"
)

var (
	ErrInvalidInput           = errors.New("invalid partner explorer input")
	ErrSubmissionNotFound     = errors.New("partner explorer submission not found")
	ErrCredentialNotFound     = errors.New("partner explorer credential not found")
	ErrOperationNotFound      = errors.New("partner explorer operation not found")
	ErrOperationLocked        = errors.New("partner explorer operation requires transactional approval")
	ErrCredentialMismatch     = errors.New("partner explorer credential does not match operation security")
	ErrRateLimit              = errors.New("partner explorer rate limit exceeded")
	ErrTargetBlocked          = errors.New("partner explorer target blocked")
	ErrConnectorNotConfigured = errors.New("partner explorer connector URL is still a placeholder")
)

type Repository interface {
	SubmissionArtifact(ctx context.Context, id, providerCode string) (SubmissionArtifact, error)
	Credential(ctx context.Context, providerCode, credentialCode string) (StoredCredential, error)
	UpsertCredential(ctx context.Context, input CredentialInput) (Credential, error)
	DeleteCredential(ctx context.Context, providerCode, credentialCode, actor, requestID string) error
	ReserveRunAttempt(ctx context.Context, keyID string, maximum int) error
	CreateRun(ctx context.Context, input RunInput) (Run, error)
	ListRuns(ctx context.Context, submissionID, providerCode string, limit int) ([]Run, error)
}
