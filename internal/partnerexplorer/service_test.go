package partnerexplorer

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/emisell/api-kurir/internal/providercredentials"
)

type explorerRepository struct {
	artifact       SubmissionArtifact
	credential     StoredCredential
	credentials    map[string]StoredCredential
	credentialSave CredentialInput
	runs           []Run
	reserveError   error
}

func (r *explorerRepository) SubmissionArtifact(context.Context, string, string) (SubmissionArtifact, error) {
	return r.artifact, nil
}

func (r *explorerRepository) Credential(_ context.Context, _, credentialCode string) (StoredCredential, error) {
	if credential, ok := r.credentials[credentialCode]; ok {
		return credential, nil
	}
	if len(r.credential.SecretCiphertext) == 0 ||
		(r.credential.CredentialCode != "" && r.credential.CredentialCode != credentialCode) {
		return StoredCredential{}, ErrCredentialNotFound
	}
	return r.credential, nil
}

func (r *explorerRepository) UpsertCredential(_ context.Context, input CredentialInput) (Credential, error) {
	r.credentialSave = input
	r.credential = StoredCredential{
		Credential: Credential{
			ProviderCode: input.ProviderCode, CredentialCode: input.CredentialCode,
			DisplayKey: input.DisplayKey,
			AuthHeader: input.AuthHeader,
			AuthPrefix: input.AuthPrefix,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		},
		SecretCiphertext: input.SecretCiphertext,
	}
	return r.credential.Credential, nil
}

func (r *explorerRepository) DeleteCredential(context.Context, string, string, string, string) error {
	r.credential = StoredCredential{}
	return nil
}

func (r *explorerRepository) ReserveRunAttempt(context.Context, string, int) error {
	return r.reserveError
}

func (r *explorerRepository) CreateRun(_ context.Context, input RunInput) (Run, error) {
	run := Run{
		ID: "run-1", SubmissionID: input.SubmissionID,
		ProviderCode: input.ProviderCode, OperationID: input.OperationID,
		Method: input.Method, Path: input.Path, Environment: "official",
		ResponseStatus: input.ResponseStatus, DurationMS: input.DurationMS,
		Outcome: input.Outcome, ErrorCode: input.ErrorCode,
		ResponsePreview: input.ResponsePreview, CreatedBy: input.CreatedBy,
		CreatedAt: time.Now().UTC(),
	}
	r.runs = append(r.runs, run)
	return run, nil
}

func (r *explorerRepository) ListRuns(context.Context, string, string, int) ([]Run, error) {
	return r.runs, nil
}

type explorerExecutor struct {
	request ExecuteRequest
	called  bool
}

func (e *explorerExecutor) Do(_ context.Context, request ExecuteRequest) (ExecuteResponse, error) {
	e.called = true
	e.request = request
	if request.Body != nil {
		_, _ = io.ReadAll(request.Body)
	}
	return ExecuteResponse{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       []byte(`{"ok":true,"echoed_secret":"official-secret-123"}`),
		Duration:   25 * time.Millisecond,
		Truncated:  false,
	}, nil
}

func TestExecuteGetSendsTrulyNilBody(t *testing.T) {
	repository := &explorerRepository{
		artifact: SubmissionArtifact{
			ID: "submission-1", ProviderCode: "mengantar", ProviderName: "Mengantar",
			Version: "1.0.0", ScanPassed: true,
			ProductionURL: "https://api.provider.test/partner/v1",
			Payload:       testExplorerArchive(t),
		},
	}
	executor := &explorerExecutor{}
	service := NewService(repository, testExplorerCipher(t), executor)
	result, err := service.Execute(context.Background(), ExecuteInput{
		SubmissionID: "submission-1", ProviderCode: "mengantar", KeyID: "key-1",
		OperationID: "get-health", Actor: "partner:mengantar:key", RequestID: "req-get",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !executor.called || executor.request.Body != nil {
		t.Fatalf("GET request body=%#v, want nil", executor.request.Body)
	}
	if !result.Success || result.ResponseStatus != http.StatusOK {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestManagedConnectorUsesPublicURLForCatalogAndRuntimeURLForExecution(t *testing.T) {
	repository := &explorerRepository{
		artifact: SubmissionArtifact{
			ID: "submission-1", ProviderCode: "rajaongkir", ProviderName: "RajaOngkir",
			Version: "1.0.1", ScanPassed: true,
			ProductionURL: "https://uploaded-provider.example/partner/v1",
			Payload:       testExplorerArchive(t),
		},
	}
	executor := &explorerExecutor{}
	service := NewService(
		repository,
		testExplorerCipher(t),
		executor,
		WithManagedConnector(ManagedConnector{
			ProviderCode:   "rajaongkir",
			PublicBaseURL:  "http://127.0.0.1:5174/connectors/rajaongkir/v1/",
			RuntimeBaseURL: "http://rajaongkir-hosted:8080/partner/v1/",
		}),
	)
	catalog, err := service.Catalog(context.Background(), "submission-1", "rajaongkir")
	if err != nil {
		t.Fatal(err)
	}
	if catalog.BaseURL != "http://127.0.0.1:5174/connectors/rajaongkir/v1" ||
		len(catalog.Operations) == 0 || catalog.Operations[0].BaseURL != catalog.BaseURL {
		t.Fatalf("managed catalog URL was not applied: %#v", catalog)
	}
	result, err := service.Execute(context.Background(), ExecuteInput{
		SubmissionID: "submission-1", ProviderCode: "rajaongkir", KeyID: "key-1",
		OperationID: "get-health", Actor: "partner:rajaongkir:key", RequestID: "req-managed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success || !executor.request.TrustedInternal ||
		executor.request.URL != "http://rajaongkir-hosted:8080/partner/v1/health" ||
		result.Operation.BaseURL != catalog.BaseURL {
		t.Fatalf("managed execution did not use isolated URLs: result=%#v request=%#v", result, executor.request)
	}
}

func TestExecuteRejectsPlaceholderConnectorWithoutCreatingFailedRun(t *testing.T) {
	repository := &explorerRepository{
		artifact: SubmissionArtifact{
			ID: "submission-1", ProviderCode: "mengantar", ProviderName: "Mengantar",
			Version: "1.0.0", ScanPassed: true,
			ProductionURL: "https://sandbox.partner.example/partner/v1",
			Payload:       testExplorerArchive(t),
		},
	}
	executor := &explorerExecutor{}
	service := NewService(repository, testExplorerCipher(t), executor)
	_, err := service.Execute(context.Background(), ExecuteInput{
		SubmissionID: "submission-1", ProviderCode: "mengantar", KeyID: "key-1",
		OperationID: "get-health", Actor: "partner:mengantar:key", RequestID: "req-placeholder",
	})
	if !errors.Is(err, ErrConnectorNotConfigured) {
		t.Fatalf("error=%v want connector not configured", err)
	}
	if executor.called || len(repository.runs) != 0 {
		t.Fatalf("placeholder connector executed=%v runs=%d", executor.called, len(repository.runs))
	}
}

func TestParseOperationsMarksOfficialTransactionEndpointsLocked(t *testing.T) {
	operations, err := parseOperations([]byte(testExplorerOpenAPI))
	if err != nil {
		t.Fatal(err)
	}
	assertSafety := func(id, want string) {
		t.Helper()
		operation, found := findOperation(operations, id)
		if !found || operation.Safety != want {
			t.Fatalf("operation %s=%#v want safety %s", id, operation, want)
		}
	}
	assertSafety("get-health", "read_only")
	assertSafety("calculate-rates", "read_only")
	assertSafety("create-shipment", "transactional_locked")
	rate, _ := findOperation(operations, "calculate-rates")
	if len(rate.RequestExample) == 0 || !json.Valid(rate.RequestExample) {
		t.Fatalf("rate example is missing: %s", rate.RequestExample)
	}
}

func TestParseOperationsSeparatesRajaOngkirCredentialsAndBaseURLs(t *testing.T) {
	operations, err := parseOperations([]byte(`openapi: 3.0.3
info:
  title: RajaOngkir connector
  version: 1.0.1
servers:
  - url: https://connector.partner.example/v1
components:
  securitySchemes:
    shipping_cost:
      type: apiKey
      in: header
      name: key
      x-emisell-credential-code: shipping_cost
      x-emisell-label: RajaOngkir Shipping Cost
    shipping_delivery:
      type: apiKey
      in: header
      name: x-api-key
      x-emisell-credential-code: shipping_delivery
      x-emisell-label: RajaOngkir Shipping Delivery
paths:
  /rates:
    post:
      operationId: calculate-rates
      security:
        - shipping_cost: []
  /shipments:
    post:
      operationId: create-shipment
      security:
        - shipping_delivery: []
`))
	if err != nil {
		t.Fatal(err)
	}
	rate, found := findOperation(operations, "calculate-rates")
	if !found || rate.CredentialCode != "shipping_cost" || rate.AuthHeader != "key" || rate.AuthPrefix != "" {
		t.Fatalf("unexpected Shipping Cost operation: %#v", rate)
	}
	shipment, found := findOperation(operations, "create-shipment")
	if !found || shipment.CredentialCode != "shipping_delivery" || shipment.AuthHeader != "x-api-key" || shipment.AuthPrefix != "" {
		t.Fatalf("unexpected Shipping Delivery operation: %#v", shipment)
	}
	if rate.BaseURL != "https://connector.partner.example/v1" || shipment.BaseURL != rate.BaseURL {
		t.Fatalf("unexpected connector base URLs: rate=%q shipment=%q", rate.BaseURL, shipment.BaseURL)
	}
	profiles := credentialStatesFromOperations(operations)
	if len(profiles) != 2 || profiles[0].Code != "shipping_cost" || profiles[1].Code != "shipping_delivery" {
		t.Fatalf("unexpected credential profiles: %#v", profiles)
	}
}

func TestSaveCredentialEncryptsAndMasksOfficialKey(t *testing.T) {
	repository := &explorerRepository{}
	cipher := testExplorerCipher(t)
	service := NewService(repository, cipher, &explorerExecutor{})
	state, err := service.SaveCredential(context.Background(), CredentialInput{
		ProviderCode: "mengantar", CredentialCode: "shipping_cost", Secret: "official-secret-123",
		AuthHeader: "Authorization", AuthPrefix: "Bearer",
		Actor: "partner:mengantar:key", RequestID: "req-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !state.Configured || state.Credential.DisplayKey != "offi********-123" {
		t.Fatalf("credential state=%#v", state)
	}
	if bytes.Contains(repository.credentialSave.SecretCiphertext, []byte("official-secret-123")) {
		t.Fatal("credential was stored as plaintext")
	}
}

func TestExecuteUsesOfficialCredentialAndRedactsEchoedSecret(t *testing.T) {
	cipher := testExplorerCipher(t)
	secret := "official-secret-123"
	ciphertext, err := cipher.Encrypt([]byte(secret), credentialAssociatedData("mengantar", "shipping_cost"))
	if err != nil {
		t.Fatal(err)
	}
	repository := &explorerRepository{
		artifact: SubmissionArtifact{
			ID: "submission-1", ProviderCode: "mengantar", ProviderName: "Mengantar",
			Version: "1.0.0", Status: "technical_review", ScanPassed: true,
			ProductionURL: "https://api.provider.test/partner/v1",
			Payload:       testExplorerArchive(t),
		},
		credential: StoredCredential{
			Credential: Credential{
				ProviderCode: "mengantar", CredentialCode: "shipping_cost", DisplayKey: "offi********-123",
				AuthHeader: "Key", AuthPrefix: "",
			},
			SecretCiphertext: ciphertext,
		},
	}
	executor := &explorerExecutor{}
	service := NewService(repository, cipher, executor)
	result, err := service.Execute(context.Background(), ExecuteInput{
		SubmissionID: "submission-1", ProviderCode: "mengantar", KeyID: "key-1",
		OperationID: "calculate-rates", Body: json.RawMessage(`{"weight":1000}`),
		Actor: "partner:mengantar:key", RequestID: "req-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !executor.called || executor.request.AuthHeader != "Key" || executor.request.AuthValue != secret {
		t.Fatalf("official credential not used: %#v", executor.request)
	}
	if result.ResponseStatus != http.StatusOK || !result.Success ||
		bytes.Contains([]byte(result.ResponseBody), []byte(secret)) {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(repository.runs) != 1 || repository.runs[0].Outcome != "passed" {
		t.Fatalf("run evidence=%#v", repository.runs)
	}
}

func TestExecuteBlocksTransactionalOperationBeforeProviderCall(t *testing.T) {
	cipher := testExplorerCipher(t)
	repository := &explorerRepository{
		artifact: SubmissionArtifact{
			ID: "submission-1", ProviderCode: "mengantar", ProviderName: "Mengantar",
			Version: "1.0.0", ScanPassed: true,
			ProductionURL: "https://api.provider.test/partner/v1",
			Payload:       testExplorerArchive(t),
		},
	}
	executor := &explorerExecutor{}
	service := NewService(repository, cipher, executor)
	_, err := service.Execute(context.Background(), ExecuteInput{
		SubmissionID: "submission-1", ProviderCode: "mengantar", KeyID: "key-1",
		OperationID: "create-shipment", Body: json.RawMessage(`{}`),
		Actor: "partner:mengantar:key", RequestID: "req-3",
	})
	if !errors.Is(err, ErrOperationLocked) {
		t.Fatalf("error=%v want operation locked", err)
	}
	if executor.called {
		t.Fatal("transactional endpoint reached provider")
	}
}

func testExplorerCipher(t *testing.T) *providercredentials.Cipher {
	t.Helper()
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	cipher, err := providercredentials.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	return cipher
}

func testExplorerArchive(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte(testExplorerOpenAPI))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

const testExplorerOpenAPI = `openapi: 3.0.3
info:
  title: Partner
  version: 1.0.0
components:
  securitySchemes:
    shipping_cost:
      type: apiKey
      in: header
      name: key
      x-emisell-credential-code: shipping_cost
      x-emisell-label: Shipping Cost
paths:
  /health:
    get:
      operationId: get-health
      summary: Health
  /rates:
    post:
      operationId: calculate-rates
      summary: Rates
      security:
        - shipping_cost: []
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                weight:
                  type: integer
                  example: 1000
  /shipments:
    post:
      operationId: create-shipment
      summary: Create shipment
`
