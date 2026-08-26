package partnerpackages

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
)

var (
	ErrInvalidInput      = errors.New("invalid partner integration package")
	ErrInvalidTransition = errors.New("invalid partner submission status transition")
	ErrUnauthorized      = errors.New("invalid partner access key")
)

const partnerAccessKeyPrefix = "epk_live_"

var (
	validProviderCode = regexp.MustCompile(`^[a-z0-9_-]{2,48}$`)
	validVersion      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
)

type Service struct {
	repository    Repository
	uploadMutex   sync.Mutex
	activeUploads map[string]struct{}
}

func NewService(repository Repository) *Service {
	return &Service{
		repository:    repository,
		activeUploads: make(map[string]struct{}),
	}
}

// BeginUpload reserves one local upload slot and one persisted hourly attempt.
// The local slot avoids duplicate work on the same instance, while the database
// reservation keeps the hourly limit consistent across replicas and restarts.
func (s *Service) BeginUpload(ctx context.Context, keyID string) (func(), error) {
	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		return nil, ErrInvalidInput
	}
	s.uploadMutex.Lock()
	if _, active := s.activeUploads[keyID]; active {
		s.uploadMutex.Unlock()
		return nil, ErrUploadInProgress
	}
	s.activeUploads[keyID] = struct{}{}
	s.uploadMutex.Unlock()

	var once sync.Once
	release := func() {
		once.Do(func() {
			s.uploadMutex.Lock()
			delete(s.activeUploads, keyID)
			s.uploadMutex.Unlock()
		})
	}
	if err := s.repository.ReserveUploadAttempt(ctx, keyID, MaxUploadsPerHour); err != nil {
		release()
		return nil, err
	}
	return release, nil
}

func (s *Service) List(ctx context.Context, filter Filter) ([]Submission, error) {
	filter.ProviderCode = strings.ToLower(strings.TrimSpace(filter.ProviderCode))
	filter.Status = strings.ToLower(strings.TrimSpace(filter.Status))
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 100
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return s.repository.List(ctx, filter)
}

func (s *Service) Get(ctx context.Context, id string) (Submission, error) {
	return s.repository.Get(ctx, strings.TrimSpace(id))
}

func (s *Service) ListForProvider(
	ctx context.Context,
	providerCode, status string,
	limit, offset int,
) ([]Submission, error) {
	return s.List(ctx, Filter{
		ProviderCode: providerCode,
		Status:       status,
		Limit:        limit,
		Offset:       offset,
	})
}

func (s *Service) GetForProvider(
	ctx context.Context,
	id, providerCode string,
) (Submission, error) {
	item, err := s.Get(ctx, id)
	if err != nil {
		return Submission{}, err
	}
	if item.ProviderCode != strings.ToLower(strings.TrimSpace(providerCode)) {
		return Submission{}, ErrNotFound
	}
	return item, nil
}

func (s *Service) Upload(ctx context.Context, input UploadInput) (Submission, error) {
	input.ProviderCode = strings.ToLower(strings.TrimSpace(input.ProviderCode))
	input.Version = strings.TrimSpace(input.Version)
	input.FileName = filepath.Base(strings.TrimSpace(input.FileName))
	input.SubmittedBy = strings.TrimSpace(input.SubmittedBy)
	if !validProviderCode.MatchString(input.ProviderCode) || !validVersion.MatchString(input.Version) {
		return Submission{}, ErrInvalidInput
	}
	if !strings.HasSuffix(strings.ToLower(input.FileName), ".zip") || len(input.Payload) == 0 || len(input.Payload) > MaxArtifactBytes {
		return Submission{}, ErrInvalidInput
	}
	if input.SubmittedBy == "" {
		input.SubmittedBy = "unknown"
	}
	input.ContentType = "application/zip"
	report := ValidateArchive(input.Payload, input.ProviderCode)
	status := "technical_review"
	if !report.Passed {
		status = "changes_requested"
	}
	digest := sha256.Sum256(input.Payload)
	return s.repository.Create(
		ctx,
		input,
		hex.EncodeToString(digest[:]),
		report,
		requiredScopes(report.Manifest.DeclaredCapability),
		status,
	)
}

func (s *Service) UpdateStatus(
	ctx context.Context,
	id string,
	update StatusUpdate,
) (Submission, error) {
	current, err := s.repository.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return Submission{}, err
	}
	update.Status = strings.ToLower(strings.TrimSpace(update.Status))
	update.ReviewNote = strings.TrimSpace(update.ReviewNote)
	update.ReviewedBy = strings.TrimSpace(update.ReviewedBy)
	if len(update.ReviewNote) > 4000 || update.ReviewedBy == "" {
		return Submission{}, ErrInvalidInput
	}
	if !allowedTransition(current.Status, update.Status) {
		return Submission{}, ErrInvalidTransition
	}
	if (update.Status == "approved" || update.Status == "published") && !current.ScanReport.Passed {
		return Submission{}, ErrInvalidTransition
	}
	update.ExpectedStatus = current.Status
	return s.repository.UpdateStatus(ctx, current.ID, update)
}

func (s *Service) Artifact(
	ctx context.Context,
	id, actor, requestID string,
) (Artifact, error) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return Artifact{}, ErrInvalidInput
	}
	return s.repository.Artifact(
		ctx,
		strings.TrimSpace(id),
		actor,
		strings.TrimSpace(requestID),
	)
}

func (s *Service) ArtifactForProvider(
	ctx context.Context,
	id, providerCode, actor, requestID string,
) (Artifact, error) {
	if _, err := s.GetForProvider(ctx, id, providerCode); err != nil {
		return Artifact{}, err
	}
	return s.Artifact(ctx, id, actor, requestID)
}

func (s *Service) ListAccessKeys(
	ctx context.Context,
	providerCode string,
) ([]AccessKey, error) {
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	if !validProviderCode.MatchString(providerCode) {
		return nil, ErrInvalidInput
	}
	return s.repository.ListAccessKeys(ctx, providerCode)
}

func (s *Service) GenerateAccessKey(
	ctx context.Context,
	providerCode, createdBy, requestID string,
) (GeneratedAccessKey, error) {
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	createdBy = strings.TrimSpace(createdBy)
	if !validProviderCode.MatchString(providerCode) || createdBy == "" {
		return GeneratedAccessKey{}, ErrInvalidInput
	}
	randomValue := make([]byte, 32)
	if _, err := rand.Read(randomValue); err != nil {
		return GeneratedAccessKey{}, err
	}
	secret := partnerAccessKeyPrefix + base64.RawURLEncoding.EncodeToString(randomValue)
	digest := sha256.Sum256([]byte(secret))
	item, err := s.repository.CreateAccessKey(ctx, AccessKeyCreateInput{
		ProviderCode: providerCode,
		DisplayKey:   partnerAccessKeyPrefix + "********" + secret[len(secret)-4:],
		SecretHash:   hex.EncodeToString(digest[:]),
		CreatedBy:    createdBy,
		RequestID:    strings.TrimSpace(requestID),
	})
	if err != nil {
		return GeneratedAccessKey{}, err
	}
	return GeneratedAccessKey{AccessKey: item, Secret: secret}, nil
}

func (s *Service) RevokeAccessKey(
	ctx context.Context,
	providerCode, keyID, revokedBy, requestID string,
) (AccessKey, error) {
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	keyID = strings.TrimSpace(keyID)
	revokedBy = strings.TrimSpace(revokedBy)
	if !validProviderCode.MatchString(providerCode) || keyID == "" || revokedBy == "" {
		return AccessKey{}, ErrInvalidInput
	}
	return s.repository.RevokeAccessKey(ctx, AccessKeyRevokeInput{
		ProviderCode: providerCode,
		KeyID:        keyID,
		RevokedBy:    revokedBy,
		RequestID:    strings.TrimSpace(requestID),
	})
}

func (s *Service) AuthenticateAccessKey(
	ctx context.Context,
	secret string,
) (AccessIdentity, error) {
	secret = strings.TrimSpace(secret)
	if !strings.HasPrefix(secret, partnerAccessKeyPrefix) || len(secret) <= len(partnerAccessKeyPrefix)+16 {
		return AccessIdentity{}, ErrUnauthorized
	}
	digest := sha256.Sum256([]byte(secret))
	identity, err := s.repository.AuthenticateAccessKey(ctx, hex.EncodeToString(digest[:]))
	if errors.Is(err, ErrNotFound) {
		return AccessIdentity{}, ErrUnauthorized
	}
	return identity, err
}

func allowedTransition(current, target string) bool {
	if current == target {
		return true
	}
	allowed := map[string]map[string]struct{}{
		"technical_review":  {"sandbox_testing": {}, "changes_requested": {}, "rejected": {}},
		"sandbox_testing":   {"security_review": {}, "changes_requested": {}, "rejected": {}},
		"security_review":   {"uat": {}, "changes_requested": {}, "rejected": {}},
		"uat":               {"approved": {}, "changes_requested": {}, "rejected": {}},
		"approved":          {"published": {}, "technical_review": {}},
		"published":         {"suspended": {}},
		"suspended":         {"published": {}, "technical_review": {}},
		"changes_requested": {"rejected": {}},
		"superseded":        {"published": {}},
	}
	_, ok := allowed[current][target]
	return ok
}

func requiredScopes(capabilities []string) []string {
	scopes := make(map[string]struct{})
	add := func(values ...string) {
		for _, value := range values {
			scopes[value] = struct{}{}
		}
	}
	for _, capability := range capabilities {
		switch strings.ToLower(strings.TrimSpace(capability)) {
		case "rates", "rate", "coverage":
			add("rates:read")
		case "shipments", "shipment_create":
			add("shipments:write")
		case "shipment_cancel":
			add("shipments:cancel")
		case "pickup", "scheduled_pickup", "on_demand_pickup":
			add("pickups:write")
		case "tracking", "external_tracking":
			add("tracking:read")
		case "label", "batch_label":
			add("labels:read")
		case "webhook":
			add("webhooks:write")
		case "balance", "payment_inquiry":
			add("balance:read")
		}
	}
	result := make([]string, 0, len(scopes))
	for scope := range scopes {
		result = append(result, scope)
	}
	slices.Sort(result)
	return result
}
