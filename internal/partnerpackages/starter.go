package partnerpackages

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"

	"github.com/emisell/api-kurir/internal/providercredentials"
	"gopkg.in/yaml.v3"
)

func StarterPackage(providerCode, providerName string) (Artifact, error) {
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	providerName = strings.TrimSpace(providerName)
	if !validProviderCode.MatchString(providerCode) || providerName == "" {
		return Artifact{}, ErrInvalidInput
	}

	var manifest integrationManifest
	manifest.SchemaVersion = "1"
	manifest.Provider.Code = providerCode
	manifest.Provider.Name = providerName
	manifest.Connector.ContractVersion = "v1"
	hostedBaseURL := fmt.Sprintf("https://api-kurir.emisell.com/connectors/%s/v1", providerCode)
	manifest.Connector.BaseURL = hostedBaseURL
	manifest.Capabilities = []string{"rates"}
	manifest.Services = []string{"regular"}
	manifest.Environments = []providercredentials.EnvironmentDefinition{{
		Code: providercredentials.EnvironmentLive,
		Label: "Live",
		Description: "Operasi provider production.",
	}}
	manifest.Credentials.Fields = []providercredentials.FieldDefinition{{
		Code: "api_key", Label: "API key", InputType: "password",
		Secret: true, Required: true,
		Placeholder: "Masukkan API key provider",
		Help: "Credential resmi milik merchant yang diterbitkan provider.",
		Capabilities: []string{"rates:read"},
		Environments: []string{providercredentials.EnvironmentLive},
	}}
	manifest.CapabilityPolicies = []providercredentials.CapabilityEnvironmentPolicy{{
		Capability: "rates", Environment: providercredentials.EnvironmentLive,
		Behavior: "live", CredentialEnvironment: providercredentials.EnvironmentLive,
		Billing: "provider_defined",
	}}
	manifestPayload, err := yaml.Marshal(manifest)
	if err != nil {
		return Artifact{}, fmt.Errorf("encode starter manifest: %w", err)
	}

	files := []struct {
		name    string
		content string
	}{
		{name: "emisell-extension.yaml", content: string(manifestPayload)},
		{name: "openapi.yaml", content: fmt.Sprintf(starterOpenAPI, hostedBaseURL)},
		{name: "README.md", content: fmt.Sprintf(starterREADME, providerName, providerCode)},
		{name: "examples/rates-request.json", content: starterRatesRequest},
		{name: "examples/rates-response.json", content: starterRatesResponse},
		{name: "contract-tests/test-cases.yaml", content: starterContractTests},
	}

	var payload bytes.Buffer
	writer := zip.NewWriter(&payload)
	for _, file := range files {
		entry, createErr := writer.Create(file.name)
		if createErr != nil {
			_ = writer.Close()
			return Artifact{}, fmt.Errorf("create starter package entry %s: %w", file.name, createErr)
		}
		if _, writeErr := entry.Write([]byte(file.content)); writeErr != nil {
			_ = writer.Close()
			return Artifact{}, fmt.Errorf("write starter package entry %s: %w", file.name, writeErr)
		}
	}
	if err := writer.Close(); err != nil {
		return Artifact{}, fmt.Errorf("finalize starter package: %w", err)
	}

	return Artifact{
		FileName:    providerCode + "-partner-starter.zip",
		ContentType: "application/zip",
		Payload:     payload.Bytes(),
	}, nil
}

const starterOpenAPI = `openapi: 3.0.3
info:
  title: Emisell Partner Connector
  version: 1.0.0
servers:
  - url: %s
paths:
  /health:
    get:
      summary: Kesiapan connector
      responses:
        "200": {description: Connector siap}
  /capabilities:
    get:
      summary: Capability connector
      responses:
        "200": {description: Capability aktif}
  /services:
    get:
      summary: Katalog layanan
      responses:
        "200": {description: Daftar layanan}
  /rates:
    post:
      summary: Mengambil quote tarif
      requestBody:
        required: true
        content:
          application/json:
            schema: {type: object}
      responses:
        "200": {description: Daftar quote}
`

const starterREADME = `# Connector %s

Package sertifikasi untuk provider \x60%s\x60.

1. connector.base_url sudah diarahkan ke endpoint hosted API Kurir untuk provider ini.
2. Deklarasikan hanya capability dan service yang sudah tersedia.
3. Lengkapi openapi.yaml mengikuti kontrak Partner API v1.
4. Gunakan data sintetis pada examples dan contract-tests.
5. Jangan memasukkan API key, token, private key, .env, atau data customer.

Source code tidak wajib berada dalam ZIP dan tidak pernah dieksekusi API Kurir.
`

const starterRatesRequest = `{
  "origin": {"district_id": "origin-example"},
  "destination": {"district_id": "destination-example"},
  "weight_grams": 1000,
  "service_groups": ["regular"]
}
`

const starterRatesResponse = `{
  "data": [
    {
      "service_code": "REG",
      "service_name": "Regular",
      "service_group": "regular",
      "price": 10000,
      "currency": "IDR"
    }
  ]
}
`

const starterContractTests = `schema_version: "1"
cases:
  - name: regular-rate-success
    operation: POST /rates
    request: ../examples/rates-request.json
    expect_status: 200
  - name: invalid-location
    operation: POST /rates
    expect_status: 422
`
