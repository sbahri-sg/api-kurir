package partnerpackages

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"
)

func TestStarterPackageIsScopedAndPassesValidator(t *testing.T) {
	artifact, err := StarterPackage("Mengantar", "Mengantar")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.FileName != "mengantar-partner-starter.zip" || artifact.ContentType != "application/zip" {
		t.Fatalf("unexpected artifact metadata: %#v", artifact)
	}
	report := ValidateArchive(artifact.Payload, "mengantar")
	if !report.Passed {
		t.Fatalf("starter package must pass its own validator: %#v", report.Checks)
	}
	if report.Manifest.ProviderCode != "mengantar" {
		t.Fatalf("provider=%q want mengantar", report.Manifest.ProviderCode)
	}
	if report.Manifest.BaseURL != "https://api-kurir.emisell.com/connectors/mengantar/v1" {
		t.Fatalf("starter package must use one connector base URL: %#v", report.Manifest)
	}

	reader, err := zip.NewReader(bytes.NewReader(artifact.Payload), int64(len(artifact.Payload)))
	if err != nil {
		t.Fatal(err)
	}
	foundREADME := false
	foundSingleEndpointManifest := false
	for _, file := range reader.File {
		if file.Name != "README.md" && file.Name != "emisell-extension.yaml" {
			continue
		}
		entry, openErr := file.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		content, readErr := io.ReadAll(entry)
		_ = entry.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if file.Name == "README.md" {
			foundREADME = bytes.Contains(content, []byte("Mengantar"))
		}
		if file.Name == "emisell-extension.yaml" {
			foundSingleEndpointManifest = bytes.Contains(content, []byte("base_url:")) &&
				!bytes.Contains(content, []byte("sandbox_url:")) &&
				!bytes.Contains(content, []byte("production_url:"))
		}
	}
	if !foundREADME {
		t.Fatal("starter README must contain provider identity")
	}
	if !foundSingleEndpointManifest {
		t.Fatal("starter manifest must contain only connector.base_url")
	}
}
