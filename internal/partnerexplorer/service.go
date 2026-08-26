package partnerexplorer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/emisell/api-kurir/internal/providercredentials"
)

var (
	validProviderCode   = regexp.MustCompile(`^[a-z0-9_-]{2,48}$`)
	validCredentialCode = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,47}$`)
	validHeaderName     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]{1,63}$`)
	validPathValue      = regexp.MustCompile(`^[A-Za-z0-9._~-]{1,128}$`)
	validAuthPrefix     = regexp.MustCompile(`^[A-Za-z0-9._~-]{0,32}$`)
)

var blockedAuthHeaders = map[string]struct{}{
	"host": {}, "connection": {}, "cookie": {}, "set-cookie": {},
	"proxy-authorization": {}, "proxy-authenticate": {}, "forwarded": {},
	"x-forwarded-for": {}, "x-forwarded-host": {}, "content-length": {},
}

type Service struct {
	repository        Repository
	cipher            *providercredentials.Cipher
	executor          Executor
	managedConnectors map[string]ManagedConnector
}

type Option func(*Service)

func WithManagedConnector(connector ManagedConnector) Option {
	return func(service *Service) {
		providerCode := normalizeProvider(connector.ProviderCode)
		if !validProviderCode.MatchString(providerCode) {
			return
		}
		connector.ProviderCode = providerCode
		connector.PublicBaseURL = strings.TrimRight(strings.TrimSpace(connector.PublicBaseURL), "/")
		connector.RuntimeBaseURL = strings.TrimRight(strings.TrimSpace(connector.RuntimeBaseURL), "/")
		if connector.PublicBaseURL == "" || connector.RuntimeBaseURL == "" {
			return
		}
		service.managedConnectors[providerCode] = connector
	}
}

func NewService(
	repository Repository,
	cipher *providercredentials.Cipher,
	executor Executor,
	options ...Option,
) *Service {
	service := &Service{
		repository: repository, cipher: cipher, executor: executor,
		managedConnectors: make(map[string]ManagedConnector),
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (s *Service) CredentialState(
	ctx context.Context,
	providerCode, credentialCode string,
) (CredentialState, error) {
	providerCode = normalizeProvider(providerCode)
	credentialCode = normalizeCredentialCode(credentialCode)
	if !validProviderCode.MatchString(providerCode) || credentialCode == "" {
		return CredentialState{}, ErrInvalidInput
	}
	credential, err := s.repository.Credential(ctx, providerCode, credentialCode)
	if errors.Is(err, ErrCredentialNotFound) {
		return CredentialState{Code: credentialCode, Configured: false}, nil
	}
	if err != nil {
		return CredentialState{}, err
	}
	public := credential.Credential
	return CredentialState{
		Code: credentialCode, AuthHeader: public.AuthHeader,
		AuthPrefix: public.AuthPrefix, Configured: true, Credential: &public,
	}, nil
}

func (s *Service) SaveCredential(
	ctx context.Context,
	input CredentialInput,
) (CredentialState, error) {
	input.ProviderCode = normalizeProvider(input.ProviderCode)
	input.CredentialCode = normalizeCredentialCode(input.CredentialCode)
	input.Secret = strings.TrimSpace(input.Secret)
	input.AuthHeader = http.CanonicalHeaderKey(strings.TrimSpace(input.AuthHeader))
	input.AuthPrefix = strings.TrimSpace(input.AuthPrefix)
	input.Actor = strings.TrimSpace(input.Actor)
	if !validProviderCode.MatchString(input.ProviderCode) || input.CredentialCode == "" ||
		len(input.Secret) < 8 || len(input.Secret) > 2048 ||
		!validHeaderName.MatchString(input.AuthHeader) ||
		!validAuthPrefix.MatchString(input.AuthPrefix) || input.Actor == "" ||
		containsControl(input.Secret) || containsControl(input.AuthPrefix) {
		return CredentialState{}, ErrInvalidInput
	}
	if _, blocked := blockedAuthHeaders[strings.ToLower(input.AuthHeader)]; blocked {
		return CredentialState{}, ErrInvalidInput
	}
	ciphertext, err := s.cipher.Encrypt(
		[]byte(input.Secret),
		credentialAssociatedData(input.ProviderCode, input.CredentialCode),
	)
	if err != nil {
		return CredentialState{}, err
	}
	input.SecretCiphertext = ciphertext
	input.DisplayKey = displayKey(input.Secret)
	item, err := s.repository.UpsertCredential(ctx, input)
	if err != nil {
		return CredentialState{}, err
	}
	return CredentialState{
		Code: input.CredentialCode, AuthHeader: item.AuthHeader,
		AuthPrefix: item.AuthPrefix, Configured: true, Credential: &item,
	}, nil
}

func (s *Service) DeleteCredential(
	ctx context.Context,
	providerCode, credentialCode, actor, requestID string,
) error {
	providerCode = normalizeProvider(providerCode)
	credentialCode = normalizeCredentialCode(credentialCode)
	actor = strings.TrimSpace(actor)
	if !validProviderCode.MatchString(providerCode) || credentialCode == "" || actor == "" {
		return ErrInvalidInput
	}
	return s.repository.DeleteCredential(ctx, providerCode, credentialCode, actor, strings.TrimSpace(requestID))
}

func (s *Service) Catalog(
	ctx context.Context,
	submissionID, providerCode string,
) (Catalog, error) {
	providerCode = normalizeProvider(providerCode)
	submission, operations, err := s.submissionOperations(ctx, submissionID, providerCode)
	if err != nil {
		return Catalog{}, err
	}
	baseURL := submission.ProductionURL
	if connector, managed := s.managedConnectors[providerCode]; managed {
		baseURL = connector.PublicBaseURL
		for index := range operations {
			operations[index].BaseURL = connector.PublicBaseURL
		}
	}
	credentials := credentialStatesFromOperations(operations)
	for index := range credentials {
		stored, stateErr := s.CredentialState(ctx, providerCode, credentials[index].Code)
		if stateErr != nil {
			return Catalog{}, stateErr
		}
		stored.Label = credentials[index].Label
		stored.Description = credentials[index].Description
		stored.AuthHeader = credentials[index].AuthHeader
		stored.AuthPrefix = credentials[index].AuthPrefix
		credentials[index] = stored
	}
	legacyCredential := CredentialState{Code: "default", Configured: false}
	if len(credentials) > 0 {
		legacyCredential = credentials[0]
	}
	return Catalog{
		SubmissionID: submission.ID,
		ProviderCode: submission.ProviderCode,
		ProviderName: submission.ProviderName,
		Version:      submission.Version,
		Environment:  "official",
		BaseURL:      baseURL,
		Credential:   legacyCredential,
		Credentials:  credentials,
		Operations:   operations,
	}, nil
}

func (s *Service) ListRuns(
	ctx context.Context,
	submissionID, providerCode string,
	limit int,
) ([]Run, error) {
	providerCode = normalizeProvider(providerCode)
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	if _, _, err := s.submissionOperations(ctx, submissionID, providerCode); err != nil {
		return nil, err
	}
	return s.repository.ListRuns(ctx, strings.TrimSpace(submissionID), providerCode, limit)
}

func (s *Service) Execute(ctx context.Context, input ExecuteInput) (ExecutionResult, error) {
	input.ProviderCode = normalizeProvider(input.ProviderCode)
	input.SubmissionID = strings.TrimSpace(input.SubmissionID)
	input.OperationID = strings.TrimSpace(input.OperationID)
	input.KeyID = strings.TrimSpace(input.KeyID)
	input.Actor = strings.TrimSpace(input.Actor)
	input.RequestID = strings.TrimSpace(input.RequestID)
	if input.OperationID == "" || input.KeyID == "" || input.Actor == "" {
		return ExecutionResult{}, ErrInvalidInput
	}
	submission, operations, err := s.submissionOperations(
		ctx, input.SubmissionID, input.ProviderCode,
	)
	if err != nil {
		return ExecutionResult{}, err
	}
	operation, found := findOperation(operations, input.OperationID)
	if !found {
		return ExecutionResult{}, ErrOperationNotFound
	}
	if operation.Safety != "read_only" {
		return ExecutionResult{}, ErrOperationLocked
	}
	if len(input.Body) > MaxRequestBodyBytes ||
		(len(bytes.TrimSpace(input.Body)) > 0 && !json.Valid(input.Body)) {
		return ExecutionResult{}, ErrInvalidInput
	}
	targetBaseURL := operation.BaseURL
	trustedInternal := false
	if connector, managed := s.managedConnectors[input.ProviderCode]; managed {
		targetBaseURL = connector.RuntimeBaseURL
		operation.BaseURL = connector.PublicBaseURL
		trustedInternal = true
	} else if targetBaseURL == "" {
		targetBaseURL = submission.ProductionURL
	}
	var targetURL string
	if trustedInternal {
		targetURL, err = buildManagedTargetURL(targetBaseURL, operation, input.PathParams, input.Query)
	} else {
		targetURL, err = buildTargetURL(targetBaseURL, operation, input.PathParams, input.Query)
	}
	if err != nil {
		return ExecutionResult{}, err
	}
	if err := s.repository.ReserveRunAttempt(ctx, input.KeyID, MaxRunsPerHour); err != nil {
		return ExecutionResult{}, err
	}
	secret := ""
	authValue := ""
	authHeader := ""
	if operation.CredentialCode != "" {
		credential, credentialErr := s.repository.Credential(
			ctx, input.ProviderCode, operation.CredentialCode,
		)
		if credentialErr != nil {
			return ExecutionResult{}, credentialErr
		}
		if !strings.EqualFold(credential.AuthHeader, operation.AuthHeader) ||
			credential.AuthPrefix != operation.AuthPrefix {
			return ExecutionResult{}, ErrCredentialMismatch
		}
		secretBytes, decryptErr := s.cipher.Decrypt(
			credential.SecretCiphertext,
			credentialAssociatedData(input.ProviderCode, operation.CredentialCode),
		)
		if decryptErr != nil {
			return ExecutionResult{}, decryptErr
		}
		secret = string(secretBytes)
		defer clear(secretBytes)
		authValue = secret
		authHeader = credential.AuthHeader
		if credential.AuthPrefix != "" {
			authValue = credential.AuthPrefix + " " + secret
		}
	}

	var body *bytes.Reader
	if len(bytes.TrimSpace(input.Body)) > 0 && operation.Method != http.MethodGet && operation.Method != http.MethodHead {
		body = bytes.NewReader(input.Body)
	} else {
		body = bytes.NewReader(nil)
	}
	response, executeErr := s.executor.Do(ctx, ExecuteRequest{
		Method: operation.Method, URL: targetURL,
		AuthHeader: authHeader, AuthValue: authValue,
		RequestID: input.RequestID, Body: readerOrNil(body),
		TrustedInternal: trustedInternal,
	})
	executedAt := time.Now().UTC()
	result := ExecutionResult{
		Operation: operation, Environment: "official", DurationMS: response.Duration.Milliseconds(),
		ResponseHeader: map[string]string{}, ExecutedAt: executedAt,
	}
	if executeErr != nil {
		result.ErrorCode = "UPSTREAM_UNREACHABLE"
		if errors.Is(executeErr, ErrTargetBlocked) {
			result.ErrorCode = "TARGET_BLOCKED"
		}
		result.ResponseBody = upstreamErrorMessage(executeErr)
	} else {
		result.ResponseStatus = response.StatusCode
		result.Success = response.StatusCode >= 200 && response.StatusCode < 400
		result.ContentType = response.Header.Get("Content-Type")
		result.Truncated = response.Truncated
		result.ResponseBody = strings.ToValidUTF8(
			redactExactSecrets(string(response.Body), secret, authValue), "�",
		)
		result.Validation.HTTPPassed = result.Success
		trimmed := bytes.TrimSpace(response.Body)
		result.Validation.JSONPassed = len(trimmed) == 0 || json.Valid(trimmed)
		result.ResponseHeader = safeResponseHeaders(response.Header)
		if !result.Success {
			result.ErrorCode = "UPSTREAM_HTTP_ERROR"
		}
	}
	outcome := "failed"
	if result.Success && result.Validation.JSONPassed {
		outcome = "passed"
	}
	run, err := s.repository.CreateRun(ctx, RunInput{
		SubmissionID: input.SubmissionID, ProviderCode: input.ProviderCode,
		OperationID: operation.ID, Method: operation.Method, Path: operation.Path,
		ResponseStatus: result.ResponseStatus, DurationMS: result.DurationMS,
		Outcome: outcome, ErrorCode: result.ErrorCode,
		ResponsePreview: responsePreview(result.ResponseBody),
		CreatedBy:       input.Actor, RequestID: input.RequestID,
	})
	if err != nil {
		return ExecutionResult{}, err
	}
	result.RunID = run.ID
	result.ExecutedAt = run.CreatedAt
	return result, nil
}

func (s *Service) submissionOperations(
	ctx context.Context,
	submissionID, providerCode string,
) (SubmissionArtifact, []Operation, error) {
	providerCode = normalizeProvider(providerCode)
	if submissionID = strings.TrimSpace(submissionID); submissionID == "" ||
		!validProviderCode.MatchString(providerCode) {
		return SubmissionArtifact{}, nil, ErrInvalidInput
	}
	submission, err := s.repository.SubmissionArtifact(ctx, submissionID, providerCode)
	if err != nil {
		return SubmissionArtifact{}, nil, err
	}
	if !submission.ScanPassed || strings.TrimSpace(submission.ProductionURL) == "" {
		return SubmissionArtifact{}, nil, ErrInvalidInput
	}
	operations, err := operationsFromArchive(submission.Payload, submission.ProductionURL)
	if err != nil {
		return SubmissionArtifact{}, nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	return submission, operations, nil
}

func buildTargetURL(
	baseURL string,
	operation Operation,
	pathParams, query map[string]string,
) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" ||
		(parsed.Port() != "" && parsed.Port() != "443") || parsed.User != nil {
		return "", ErrTargetBlocked
	}
	hostname := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if hostname == "example" || strings.HasSuffix(hostname, ".example") {
		return "", ErrConnectorNotConfigured
	}
	return buildParsedTargetURL(parsed, operation, pathParams, query)
}

func buildManagedTargetURL(
	baseURL string,
	operation Operation,
	pathParams, query map[string]string,
) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Hostname() == "" || parsed.User != nil {
		return "", ErrTargetBlocked
	}
	return buildParsedTargetURL(parsed, operation, pathParams, query)
}

func buildParsedTargetURL(
	parsed *url.URL,
	operation Operation,
	pathParams, query map[string]string,
) (string, error) {
	knownPath := make(map[string]Parameter)
	knownQuery := make(map[string]Parameter)
	for _, parameter := range operation.Parameters {
		switch parameter.In {
		case "path":
			knownPath[parameter.Name] = parameter
		case "query":
			knownQuery[parameter.Name] = parameter
		}
	}
	endpointPath := operation.Path
	for name, parameter := range knownPath {
		value := strings.TrimSpace(pathParams[name])
		if value == "" && parameter.Required {
			return "", ErrInvalidInput
		}
		if value != "" {
			if !validPathValue.MatchString(value) {
				return "", ErrInvalidInput
			}
			endpointPath = strings.ReplaceAll(endpointPath, "{"+name+"}", value)
		}
	}
	if strings.ContainsAny(endpointPath, "{}") {
		return "", ErrInvalidInput
	}
	for name := range pathParams {
		if _, exists := knownPath[name]; !exists {
			return "", ErrInvalidInput
		}
	}
	values := make(url.Values)
	for name, value := range query {
		parameter, exists := knownQuery[name]
		value = strings.TrimSpace(value)
		if !exists || len(value) > 512 || containsControl(value) {
			return "", ErrInvalidInput
		}
		if value == "" && parameter.Required {
			return "", ErrInvalidInput
		}
		if value != "" {
			values.Set(name, value)
		}
	}
	for name, parameter := range knownQuery {
		if parameter.Required && strings.TrimSpace(query[name]) == "" {
			return "", ErrInvalidInput
		}
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + strings.TrimLeft(endpointPath, "/")
	parsed.RawPath = ""
	parsed.RawQuery = values.Encode()
	parsed.Fragment = ""
	return parsed.String(), nil
}

func findOperation(operations []Operation, id string) (Operation, bool) {
	for _, operation := range operations {
		if operation.ID == id {
			return operation, true
		}
	}
	return Operation{}, false
}

func credentialAssociatedData(providerCode, credentialCode string) []byte {
	return []byte("partner-explorer:" + providerCode + ":" + credentialCode)
}

func normalizeProvider(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func displayKey(secret string) string {
	runes := []rune(secret)
	if len(runes) <= 8 {
		return "********"
	}
	return string(runes[:4]) + "********" + string(runes[len(runes)-4:])
}

func containsControl(value string) bool {
	return !utf8.ValidString(value) || strings.ContainsAny(value, "\r\n\x00")
}

func readerOrNil(reader *bytes.Reader) io.Reader {
	if reader.Len() == 0 {
		return nil
	}
	return reader
}

func redactExactSecrets(value string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	return value
}

func responsePreview(value string) string {
	const maximum = 4096
	var payload any
	if json.Unmarshal([]byte(value), &payload) == nil {
		redactSensitiveJSON(payload)
		if encoded, err := json.Marshal(payload); err == nil {
			value = string(encoded)
		}
	}
	runes := []rune(value)
	if len(runes) > maximum {
		value = string(runes[:maximum])
	}
	return value
}

func redactSensitiveJSON(value any) {
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			normalized := strings.ToLower(key)
			if strings.Contains(normalized, "token") || strings.Contains(normalized, "secret") ||
				strings.Contains(normalized, "api_key") || strings.Contains(normalized, "authorization") ||
				strings.Contains(normalized, "phone") || strings.Contains(normalized, "email") ||
				strings.Contains(normalized, "address") || strings.Contains(normalized, "waybill") ||
				strings.Contains(normalized, "awb") {
				item[key] = "[REDACTED]"
				continue
			}
			redactSensitiveJSON(child)
		}
	case []any:
		for _, child := range item {
			redactSensitiveJSON(child)
		}
	}
}

func safeResponseHeaders(header http.Header) map[string]string {
	allowed := []string{"Content-Type", "X-Request-Id", "Retry-After", "RateLimit-Limit", "RateLimit-Remaining", "RateLimit-Reset"}
	sort.Strings(allowed)
	result := make(map[string]string)
	for _, name := range allowed {
		if value := strings.TrimSpace(header.Get(name)); value != "" {
			result[name] = value
		}
	}
	return result
}
