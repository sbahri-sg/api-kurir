package partnerpackages

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestValidateArchiveAcceptsCanonicalPackage(t *testing.T) {
	report := ValidateArchive(testArchive(t, map[string]string{
		"emisell-extension.yaml": testManifest("mengantar"),
		"openapi.yaml":           testOpenAPI(),
		"README.md":              "# Mengantar connector",
	}), "mengantar")

	if !report.Passed {
		t.Fatalf("expected package to pass: %#v", report.Checks)
	}
	if report.Manifest.ProviderCode != "mengantar" {
		t.Fatalf("unexpected provider code: %q", report.Manifest.ProviderCode)
	}
	if len(report.RequiredOpenAPIPaths) != 8 {
		t.Fatalf("expected 8 canonical paths, got %d", len(report.RequiredOpenAPIPaths))
	}
}

func TestValidateArchiveRejectsTraversalAndSecret(t *testing.T) {
	report := ValidateArchive(testArchive(t, map[string]string{
		"emisell-extension.yaml": testManifest("mengantar"),
		"openapi.yaml":           testOpenAPI(),
		"../outside.txt":         "unsafe",
		"config/.env":            "RAJAONGKIR_API_KEY=secret-value",
	}), "mengantar")

	if report.Passed {
		t.Fatal("expected unsafe package to fail")
	}
	if !hasFailedCheck(report, "archive_safety") || !hasFailedCheck(report, "secret_scan") {
		t.Fatalf("expected archive and secret failures: %#v", report.Checks)
	}
}

func TestValidateArchiveRejectsProviderMismatch(t *testing.T) {
	report := ValidateArchive(testArchive(t, map[string]string{
		"emisell-extension.yaml": testManifest("provider-lain"),
		"openapi.yaml":           testOpenAPI(),
	}), "mengantar")

	if report.Passed || !hasFailedCheck(report, "manifest") {
		t.Fatalf("expected manifest failure: %#v", report.Checks)
	}
}

func TestValidateArchiveRejectsPrivateConnectorURL(t *testing.T) {
	manifest := strings.ReplaceAll(
		testManifest("mengantar"),
		"https://api.partner.example/partner/v1",
		"https://127.0.0.1/partner/v1",
	)
	report := ValidateArchive(testArchive(t, map[string]string{
		"emisell-extension.yaml": manifest,
		"openapi.yaml":           testOpenAPI(),
	}), "mengantar")

	if report.Passed || !hasFailedCheck(report, "manifest") {
		t.Fatalf("expected private connector URL failure: %#v", report.Checks)
	}
}

func TestValidateArchiveRejectsLegacyConnectorURLs(t *testing.T) {
	manifest := strings.Replace(
		testManifest("mengantar"),
		"  base_url: https://api.partner.example/partner/v1",
		"  sandbox_url: https://sandbox.partner.example/partner/v1\n  production_url: https://api.partner.example/partner/v1",
		1,
	)
	report := ValidateArchive(testArchive(t, map[string]string{
		"emisell-extension.yaml": manifest,
		"openapi.yaml":           testOpenAPI(),
	}), "mengantar")

	if report.Passed || !hasFailedCheck(report, "manifest") {
		t.Fatalf("expected legacy connector fields to be rejected: %#v", report)
	}
}

func TestValidateArchiveRejectsWrongCanonicalMethod(t *testing.T) {
	openAPI := strings.Replace(testOpenAPI(), "/rates: {post:", "/rates: {get:", 1)
	report := ValidateArchive(testArchive(t, map[string]string{
		"emisell-extension.yaml": testManifest("mengantar"),
		"openapi.yaml":           openAPI,
	}), "mengantar")

	if report.Passed || !hasFailedCheck(report, "openapi") {
		t.Fatalf("expected openapi method failure: %#v", report.Checks)
	}
}

func TestValidateArchiveRejectsUnknownCapability(t *testing.T) {
	manifest := strings.Replace(
		testManifest("mengantar"),
		"capabilities: [rates, shipments, pickup, tracking, balance]",
		"capabilities: [rates, instant_teleport]",
		1,
	)
	report := ValidateArchive(testArchive(t, map[string]string{
		"emisell-extension.yaml": manifest,
		"openapi.yaml":           testOpenAPI(),
	}), "mengantar")

	if report.Passed || !hasFailedCheck(report, "manifest") {
		t.Fatalf("expected unknown capability failure: %#v", report.Checks)
	}
}

func TestValidateArchiveRejectsUnknownServiceGroup(t *testing.T) {
	manifest := strings.Replace(
		testManifest("mengantar"),
		"services: [regular, cargo]",
		"services: [regular, same_day]",
		1,
	)
	report := ValidateArchive(testArchive(t, map[string]string{
		"emisell-extension.yaml": manifest,
		"openapi.yaml":           testOpenAPI(),
	}), "mengantar")

	if report.Passed || !hasFailedCheck(report, "manifest") {
		t.Fatalf("expected unknown service failure: %#v", report.Checks)
	}
}

func testArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var payload bytes.Buffer
	writer := zip.NewWriter(&payload)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return payload.Bytes()
}

func testManifest(providerCode string) string {
	return `schema_version: "1"
provider:
  code: ` + providerCode + `
  name: Test Partner
connector:
  contract_version: v1
  base_url: https://api.partner.example/partner/v1
capabilities: [rates, shipments, pickup, tracking, balance]
services: [regular, cargo]
`
}

func testOpenAPI() string {
	return `openapi: 3.0.3
info:
  title: Test Partner
  version: 1.0.0
paths:
  /health: {get: {responses: {"200": {description: OK}}}}
  /capabilities: {get: {responses: {"200": {description: OK}}}}
  /services: {get: {responses: {"200": {description: OK}}}}
  /rates: {post: {responses: {"200": {description: OK}}}}
  /shipments: {post: {responses: {"200": {description: OK}}}}
  /pickups: {post: {responses: {"200": {description: OK}}}}
  /tracking/waybills: {post: {responses: {"200": {description: OK}}}}
  /account/balance: {get: {responses: {"200": {description: OK}}}}
`
}

func hasFailedCheck(report ScanReport, code string) bool {
	for _, check := range report.Checks {
		if check.Code == code && check.Status == "failed" {
			return true
		}
	}
	return false
}
