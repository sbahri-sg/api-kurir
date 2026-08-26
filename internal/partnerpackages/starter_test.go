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

	reader, err := zip.NewReader(bytes.NewReader(artifact.Payload), int64(len(artifact.Payload)))
	if err != nil {
		t.Fatal(err)
	}
	foundREADME := false
	for _, file := range reader.File {
		if file.Name != "README.md" {
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
		foundREADME = bytes.Contains(content, []byte("Mengantar"))
	}
	if !foundREADME {
		t.Fatal("starter README must contain provider identity")
	}
}
