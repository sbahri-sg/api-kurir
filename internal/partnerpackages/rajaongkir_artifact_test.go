package partnerpackages

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRajaOngkirHostedArtifactPassesPackageValidator(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test path")
	}
	root := filepath.Clean(filepath.Join(
		filepath.Dir(currentFile), "..", "..", "artifacts", "rajaongkir-hosted-v1.0.8",
	))
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() == ".DS_Store" {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		writer, err := archive.Create(filepath.ToSlash(relative))
		if err != nil {
			return err
		}
		_, err = writer.Write(payload)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	report := ValidateArchive(buffer.Bytes(), "rajaongkir")
	if !report.Passed {
		messages := make([]string, 0, len(report.Checks))
		for _, check := range report.Checks {
			if check.Status == "failed" {
				messages = append(messages, check.Code+": "+check.Message)
			}
		}
		t.Fatalf("artifact failed validation: %s", strings.Join(messages, "; "))
	}
	if report.Manifest.ProviderCode != "rajaongkir" {
		t.Fatalf("provider code=%q", report.Manifest.ProviderCode)
	}
	if len(report.Manifest.CredentialFields) != 2 ||
		len(report.Manifest.Environments) != 2 ||
		len(report.Manifest.CapabilityPolicies) != 8 {
		t.Fatalf("credential/environment contract incomplete: %#v", report.Manifest)
	}
}
