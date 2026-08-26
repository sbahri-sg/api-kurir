package partnerexplorer

import (
	"encoding/json"
	"time"
)

const (
	MaxRequestBodyBytes = 64 * 1024
	MaxResponseBytes    = 1024 * 1024
	MaxRunsPerHour      = 120
)

type Credential struct {
	ProviderCode   string    `json:"provider_code"`
	CredentialCode string    `json:"credential_code"`
	DisplayKey     string    `json:"display_key"`
	AuthHeader     string    `json:"auth_header"`
	AuthPrefix     string    `json:"auth_prefix"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type CredentialState struct {
	Code        string      `json:"code"`
	Label       string      `json:"label"`
	Description string      `json:"description"`
	AuthHeader  string      `json:"auth_header"`
	AuthPrefix  string      `json:"auth_prefix"`
	Configured  bool        `json:"configured"`
	Credential  *Credential `json:"credential"`
}

type StoredCredential struct {
	Credential
	SecretCiphertext []byte
}

type CredentialInput struct {
	ProviderCode     string
	CredentialCode   string
	Secret           string
	AuthHeader       string
	AuthPrefix       string
	DisplayKey       string
	SecretCiphertext []byte
	Actor            string
	RequestID        string
}

type Parameter struct {
	Name        string `json:"name"`
	In          string `json:"in"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Example     string `json:"example"`
}

type Operation struct {
	ID                    string          `json:"id"`
	Method                string          `json:"method"`
	Path                  string          `json:"path"`
	Summary               string          `json:"summary"`
	Description           string          `json:"description"`
	Capability            string          `json:"capability"`
	Safety                string          `json:"safety"`
	BaseURL               string          `json:"base_url"`
	CredentialCode        string          `json:"credential_code"`
	CredentialLabel       string          `json:"credential_label"`
	CredentialDescription string          `json:"credential_description"`
	AuthHeader            string          `json:"auth_header"`
	AuthPrefix            string          `json:"auth_prefix"`
	Parameters            []Parameter     `json:"parameters"`
	RequestExample        json.RawMessage `json:"request_example,omitempty"`
}

type Catalog struct {
	SubmissionID string            `json:"submission_id"`
	ProviderCode string            `json:"provider_code"`
	ProviderName string            `json:"provider_name"`
	Version      string            `json:"version"`
	Environment  string            `json:"environment"`
	BaseURL      string            `json:"base_url"`
	Credential   CredentialState   `json:"credential"`
	Credentials  []CredentialState `json:"credentials"`
	Operations   []Operation       `json:"operations"`
}

// ManagedConnector is an operator-owned connector endpoint. PublicBaseURL is
// safe to expose in the Partner Portal, while RuntimeBaseURL may use an
// internal Docker/service address that must never come from an uploaded ZIP.
type ManagedConnector struct {
	ProviderCode   string
	PublicBaseURL  string
	RuntimeBaseURL string
}

type ExecuteInput struct {
	SubmissionID string
	ProviderCode string
	KeyID        string
	OperationID  string
	PathParams   map[string]string
	Query        map[string]string
	Body         json.RawMessage
	Actor        string
	RequestID    string
}

type ValidationResult struct {
	HTTPPassed bool `json:"http_passed"`
	JSONPassed bool `json:"json_passed"`
}

type ExecutionResult struct {
	RunID          string            `json:"run_id"`
	Operation      Operation         `json:"operation"`
	Environment    string            `json:"environment"`
	ResponseStatus int               `json:"response_status"`
	DurationMS     int64             `json:"duration_ms"`
	Success        bool              `json:"success"`
	ContentType    string            `json:"content_type"`
	ResponseBody   string            `json:"response_body"`
	ResponseHeader map[string]string `json:"response_headers"`
	Truncated      bool              `json:"truncated"`
	ErrorCode      string            `json:"error_code,omitempty"`
	Validation     ValidationResult  `json:"validation"`
	ExecutedAt     time.Time         `json:"executed_at"`
}

type Run struct {
	ID              string    `json:"id"`
	SubmissionID    string    `json:"submission_id"`
	ProviderCode    string    `json:"provider_code"`
	OperationID     string    `json:"operation_id"`
	Method          string    `json:"method"`
	Path            string    `json:"path"`
	Environment     string    `json:"environment"`
	ResponseStatus  int       `json:"response_status"`
	DurationMS      int64     `json:"duration_ms"`
	Outcome         string    `json:"outcome"`
	ErrorCode       string    `json:"error_code"`
	ResponsePreview string    `json:"response_preview"`
	CreatedBy       string    `json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
}

type RunInput struct {
	SubmissionID    string
	ProviderCode    string
	OperationID     string
	Method          string
	Path            string
	ResponseStatus  int
	DurationMS      int64
	Outcome         string
	ErrorCode       string
	ResponsePreview string
	CreatedBy       string
	RequestID       string
}

type SubmissionArtifact struct {
	ID            string
	ProviderCode  string
	ProviderName  string
	Version       string
	Status        string
	ScanPassed    bool
	ProductionURL string
	Payload       []byte
}
