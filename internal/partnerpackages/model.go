package partnerpackages

import (
	"time"

	"github.com/emisell/api-kurir/internal/providercredentials"
)

const (
	MaxArtifactBytes        = 25 * 1024 * 1024
	MaxExpandedBytes        = 100 * 1024 * 1024
	MaxArchiveFiles         = 250
	MaxUploadsPerHour       = 10
	MaxProviderSubmissions  = 25
	MaxProviderStorageBytes = 500 * 1024 * 1024
)

type ScanCheck struct {
	Code    string `json:"code"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type ManifestSummary struct {
	SchemaVersion      string   `json:"schema_version"`
	ProviderCode       string   `json:"provider_code"`
	ProviderName       string   `json:"provider_name"`
	ContractVersion    string   `json:"contract_version"`
	BaseURL            string   `json:"base_url"`
	DeclaredCapability []string `json:"declared_capabilities"`
	DeclaredServices   []string `json:"declared_services"`
	CredentialFields   []providercredentials.FieldDefinition `json:"credential_fields"`
	Environments       []providercredentials.EnvironmentDefinition `json:"environments"`
	CapabilityPolicies []providercredentials.CapabilityEnvironmentPolicy `json:"capability_policies"`
}

type ScanReport struct {
	Passed               bool            `json:"passed"`
	FileCount            int             `json:"file_count"`
	ExpandedSize         int64           `json:"expanded_size"`
	Checks               []ScanCheck     `json:"checks"`
	Warnings             []string        `json:"warnings"`
	Manifest             ManifestSummary `json:"manifest"`
	RequiredOpenAPIPaths []string        `json:"required_openapi_paths"`
}

type Submission struct {
	ID              string     `json:"id"`
	ProviderCode    string     `json:"provider_code"`
	ProviderName    string     `json:"provider_name"`
	Version         string     `json:"version"`
	Status          string     `json:"status"`
	IsActiveRelease bool       `json:"is_active_release"`
	FileName        string     `json:"file_name"`
	ContentType     string     `json:"content_type"`
	ArtifactSize    int64      `json:"artifact_size"`
	ArtifactSHA256  string     `json:"artifact_sha256"`
	ScanReport      ScanReport `json:"scan_report"`
	RequiredScopes  []string   `json:"required_scopes"`
	ReviewNote      string     `json:"review_note"`
	SubmittedBy     string     `json:"submitted_by"`
	ReviewedBy      string     `json:"reviewed_by"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type Artifact struct {
	FileName    string
	ContentType string
	Payload     []byte
}

type Filter struct {
	ProviderCode string
	Status       string
	Limit        int
	Offset       int
}

type UploadInput struct {
	ProviderCode string
	Version      string
	FileName     string
	ContentType  string
	Payload      []byte
	SubmittedBy  string
	RequestID    string
}

type StatusUpdate struct {
	Status         string
	ExpectedStatus string
	ReviewNote     string
	ReviewedBy     string
	RequestID      string
}

type AccessKey struct {
	ID           string     `json:"id"`
	ProviderCode string     `json:"provider_code"`
	ProviderName string     `json:"provider_name"`
	DisplayKey   string     `json:"display_key"`
	Active       bool       `json:"active"`
	CreatedBy    string     `json:"created_by"`
	CreatedAt    time.Time  `json:"created_at"`
	LastUsedAt   *time.Time `json:"last_used_at"`
	RevokedBy    string     `json:"revoked_by"`
	RevokedAt    *time.Time `json:"revoked_at"`
}

type GeneratedAccessKey struct {
	AccessKey AccessKey `json:"access_key"`
	Secret    string    `json:"secret"`
}

type AccessIdentity struct {
	KeyID        string `json:"-"`
	ProviderCode string `json:"provider_code"`
	ProviderName string `json:"provider_name"`
}

type AccessKeyCreateInput struct {
	ProviderCode string
	DisplayKey   string
	SecretHash   string
	CreatedBy    string
	RequestID    string
}

type AccessKeyRevokeInput struct {
	ProviderCode string
	KeyID        string
	RevokedBy    string
	RequestID    string
}
