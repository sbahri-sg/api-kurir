package partnerpackages

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/emisell/api-kurir/internal/providercredentials"
	"gopkg.in/yaml.v3"
)

var sensitiveContentPatterns = []struct {
	code    string
	pattern *regexp.Regexp
}{
	{code: "private_key", pattern: regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`)},
	{code: "biteship_token", pattern: regexp.MustCompile(`biteship_(?:live|test)\.[A-Za-z0-9._-]{20,}`)},
	{code: "rajaongkir_key", pattern: regexp.MustCompile(`(?i)RAJAONGKIR_API_KEY\s*[:=]\s*\S+`)},
	{code: "capsolver_key", pattern: regexp.MustCompile(`CAP-[A-Z0-9]{16,}`)},
}

var blockedArchiveExtensions = map[string]struct{}{
	".exe": {}, ".dll": {}, ".so": {}, ".dylib": {}, ".bin": {}, ".app": {},
}

var allowedCapabilities = map[string]struct{}{
	"rates": {}, "shipments": {}, "pickup": {}, "tracking": {}, "balance": {},
}

var allowedServiceGroups = map[string]struct{}{
	"regular": {}, "next_day": {}, "economy": {}, "cargo": {},
}

var allowedCapabilityBehaviors = map[string]struct{}{
	"live": {}, "provider_sandbox": {}, "simulated": {}, "live_read_only": {}, "unavailable": {},
}

var allowedBillingBehaviors = map[string]struct{}{
	"provider_charged": {}, "no_charge": {}, "provider_defined": {},
}

type integrationManifest struct {
	SchemaVersion string `yaml:"schema_version"`
	Provider      struct {
		Code string `yaml:"code"`
		Name string `yaml:"name"`
	} `yaml:"provider"`
	Connector struct {
		ContractVersion string `yaml:"contract_version"`
		BaseURL         string `yaml:"base_url,omitempty"`
	} `yaml:"connector"`
	Capabilities []string `yaml:"capabilities"`
	Services     []string `yaml:"services"`
	Credentials struct {
		Fields []providercredentials.FieldDefinition `yaml:"fields"`
	} `yaml:"credentials,omitempty"`
	Environments       []providercredentials.EnvironmentDefinition       `yaml:"environments,omitempty"`
	CapabilityPolicies []providercredentials.CapabilityEnvironmentPolicy `yaml:"capability_policies,omitempty"`
}

func ValidateArchive(payload []byte, providerCode string) ScanReport {
	report := ScanReport{
		Checks:   make([]ScanCheck, 0, 8),
		Warnings: []string{"Malware signature scanner eksternal belum terhubung; artifact tetap dikarantina dan tidak pernah dieksekusi."},
		Manifest: ManifestSummary{
			DeclaredCapability: make([]string, 0),
			DeclaredServices:   make([]string, 0),
		},
		RequiredOpenAPIPaths: make([]string, 0),
	}
	fail := func(code, message string) {
		report.Checks = append(report.Checks, ScanCheck{Code: code, Status: "failed", Message: message})
	}
	pass := func(code, message string) {
		report.Checks = append(report.Checks, ScanCheck{Code: code, Status: "passed", Message: message})
	}

	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		fail("archive", "File bukan ZIP yang valid atau central directory rusak.")
		return report
	}
	if len(reader.File) == 0 || len(reader.File) > MaxArchiveFiles {
		fail("archive_file_count", fmt.Sprintf("Jumlah entry ZIP harus 1 sampai %d.", MaxArchiveFiles))
		return report
	}

	files := make(map[string][]byte, len(reader.File))
	seenEntries := make(map[string]struct{}, len(reader.File))
	var expanded int64
	unsafeArchive := false
	secretFindings := make([]string, 0)
	for _, entry := range reader.File {
		cleanName := path.Clean(strings.ReplaceAll(entry.Name, "\\", "/"))
		if cleanName == "." || strings.HasPrefix(cleanName, "../") || strings.HasPrefix(cleanName, "/") || entry.Mode()&os.ModeSymlink != 0 {
			unsafeArchive = true
			continue
		}
		if _, duplicate := seenEntries[cleanName]; duplicate {
			unsafeArchive = true
			continue
		}
		seenEntries[cleanName] = struct{}{}
		if entry.FileInfo().IsDir() {
			continue
		}
		if _, blocked := blockedArchiveExtensions[strings.ToLower(path.Ext(cleanName))]; blocked {
			unsafeArchive = true
			continue
		}
		expanded += int64(entry.UncompressedSize64)
		if expanded > MaxExpandedBytes || entry.UncompressedSize64 > 20*1024*1024 {
			unsafeArchive = true
			continue
		}
		if strings.EqualFold(path.Base(cleanName), ".env") || strings.Contains(strings.ToLower(cleanName), "id_rsa") {
			secretFindings = append(secretFindings, cleanName)
		}
		if entry.UncompressedSize64 > 4*1024*1024 {
			continue
		}
		content, readErr := readZipEntry(entry, 4*1024*1024)
		if readErr != nil {
			unsafeArchive = true
			continue
		}
		files[cleanName] = content
		for _, finding := range sensitiveContentPatterns {
			if finding.pattern.Match(content) {
				secretFindings = append(secretFindings, cleanName+":"+finding.code)
			}
		}
	}
	report.FileCount = len(reader.File)
	report.ExpandedSize = expanded
	if expanded > int64(len(payload))*100 && expanded > 10*1024*1024 {
		unsafeArchive = true
	}
	if unsafeArchive {
		fail("archive_safety", "ZIP mengandung path, symlink, binary, ukuran, atau rasio kompresi yang tidak aman.")
	} else {
		pass("archive_safety", "Path, symlink, binary, ukuran, dan rasio kompresi ZIP aman.")
	}
	if len(secretFindings) > 0 {
		sort.Strings(secretFindings)
		fail("secret_scan", "Secret atau file sensitif terdeteksi: "+strings.Join(secretFindings, ", "))
	} else {
		pass("secret_scan", "Tidak ditemukan pola secret yang dilarang.")
	}

	manifestBytes, manifestFound := files["emisell-extension.yaml"]
	openAPIBytes, openAPIFound := files["openapi.yaml"]
	if !manifestFound || !openAPIFound {
		fail("required_files", "ZIP wajib memiliki emisell-extension.yaml dan openapi.yaml pada root.")
		return finalizeReport(report)
	}
	pass("required_files", "Manifest dan OpenAPI ditemukan pada root ZIP.")

	manifest, manifestErr := validateManifest(manifestBytes, providerCode)
	if manifestErr != nil {
		fail("manifest", manifestErr.Error())
	} else {
		report.Manifest = manifest
		pass("manifest", "Manifest schema v1 dan endpoint connector HTTPS valid.")
	}

	requiredPaths, openAPIErr := validateOpenAPI(openAPIBytes, manifest.DeclaredCapability)
	report.RequiredOpenAPIPaths = requiredPaths
	if openAPIErr != nil {
		fail("openapi", openAPIErr.Error())
	} else {
		pass("openapi", "OpenAPI 3.x menyediakan seluruh path canonical yang dideklarasikan.")
	}
	return finalizeReport(report)
}

func readZipEntry(entry *zip.File, maximum int64) ([]byte, error) {
	reader, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	content, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maximum {
		return nil, errors.New("entry exceeds scan limit")
	}
	return content, nil
}

func validateManifest(payload []byte, providerCode string) (ManifestSummary, error) {
	var manifest integrationManifest
	decoder := yaml.NewDecoder(bytes.NewReader(payload))
	decoder.KnownFields(true)
	if err := decoder.Decode(&manifest); err != nil {
		return ManifestSummary{}, errors.New("emisell-extension.yaml bukan YAML yang valid")
	}
	manifest.Provider.Code = strings.ToLower(strings.TrimSpace(manifest.Provider.Code))
	if manifest.SchemaVersion != "1" {
		return ManifestSummary{}, errors.New("schema_version manifest wajib bernilai 1")
	}
	if manifest.Provider.Code != providerCode {
		return ManifestSummary{}, errors.New("provider.code pada manifest tidak sama dengan provider submission")
	}
	if strings.TrimSpace(manifest.Provider.Name) == "" {
		return ManifestSummary{}, errors.New("provider.name wajib diisi")
	}
	if manifest.Connector.ContractVersion != "v1" {
		return ManifestSummary{}, errors.New("connector.contract_version wajib bernilai v1")
	}
	baseURL := strings.TrimSpace(manifest.Connector.BaseURL)
	if !validHTTPSURL(baseURL) {
		return ManifestSummary{}, errors.New("connector.base_url wajib URL HTTPS publik")
	}
	capabilities := normalizedUnique(manifest.Capabilities)
	services := normalizedUnique(manifest.Services)
	if len(capabilities) == 0 {
		return ManifestSummary{}, errors.New("capabilities wajib berisi minimal satu capability")
	}
	if err := validateManifestValues("capability", capabilities, allowedCapabilities); err != nil {
		return ManifestSummary{}, err
	}
	if len(services) == 0 {
		return ManifestSummary{}, errors.New("services wajib berisi minimal satu group layanan")
	}
	if err := validateManifestValues("service", services, allowedServiceGroups); err != nil {
		return ManifestSummary{}, err
	}
	environments, err := normalizeEnvironments(manifest.Environments)
	if err != nil {
		return ManifestSummary{}, err
	}
	credentialFields := providercredentials.NormalizeFieldDefinitions(manifest.Credentials.Fields)
	if err := providercredentials.ValidateFieldDefinitions(credentialFields, environments); err != nil {
		return ManifestSummary{}, fmt.Errorf("credential schema tidak valid: %w", err)
	}
	capabilityPolicies, err := normalizeCapabilityPolicies(
		manifest.CapabilityPolicies,
		capabilities,
		environments,
	)
	if err != nil {
		return ManifestSummary{}, err
	}
	return ManifestSummary{
		SchemaVersion:      manifest.SchemaVersion,
		ProviderCode:       manifest.Provider.Code,
		ProviderName:       strings.TrimSpace(manifest.Provider.Name),
		ContractVersion:    manifest.Connector.ContractVersion,
		BaseURL:            baseURL,
		DeclaredCapability: capabilities,
		DeclaredServices:   services,
		CredentialFields:   credentialFields,
		Environments:       environments,
		CapabilityPolicies: capabilityPolicies,
	}, nil
}

func normalizeEnvironments(
	values []providercredentials.EnvironmentDefinition,
) ([]providercredentials.EnvironmentDefinition, error) {
	if len(values) == 0 {
		return []providercredentials.EnvironmentDefinition{{
			Code: providercredentials.EnvironmentLive,
			Label: "Live",
			Description: "Operasi provider production.",
		}}, nil
	}
	result := make([]providercredentials.EnvironmentDefinition, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value.Code = strings.ToLower(strings.TrimSpace(value.Code))
		value.Label = strings.TrimSpace(value.Label)
		value.Description = strings.TrimSpace(value.Description)
		if value.Code != providercredentials.EnvironmentLive &&
			value.Code != providercredentials.EnvironmentSandbox {
			return nil, errors.New("environment hanya boleh live atau sandbox")
		}
		if value.Label == "" || len(value.Label) > 80 || len(value.Description) > 300 {
			return nil, errors.New("label atau deskripsi environment tidak valid")
		}
		if _, exists := seen[value.Code]; exists {
			return nil, errors.New("environment tidak boleh duplikat")
		}
		seen[value.Code] = struct{}{}
		result = append(result, value)
	}
	if _, exists := seen[providercredentials.EnvironmentLive]; !exists {
		return nil, errors.New("environment live wajib dideklarasikan")
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Code < result[j].Code })
	return result, nil
}

func normalizeCapabilityPolicies(
	values []providercredentials.CapabilityEnvironmentPolicy,
	capabilities []string,
	environments []providercredentials.EnvironmentDefinition,
) ([]providercredentials.CapabilityEnvironmentPolicy, error) {
	declaredCapabilities := make(map[string]struct{}, len(capabilities))
	for _, capability := range capabilities {
		declaredCapabilities[capability] = struct{}{}
	}
	declaredEnvironments := make(map[string]struct{}, len(environments))
	for _, environment := range environments {
		declaredEnvironments[environment.Code] = struct{}{}
	}
	if len(values) == 0 {
		values = make([]providercredentials.CapabilityEnvironmentPolicy, 0, len(capabilities))
		for _, capability := range capabilities {
			values = append(values, providercredentials.CapabilityEnvironmentPolicy{
				Capability: capability,
				Environment: providercredentials.EnvironmentLive,
				Behavior: "live",
				CredentialEnvironment: providercredentials.EnvironmentLive,
				Billing: "provider_defined",
			})
		}
	}
	result := make([]providercredentials.CapabilityEnvironmentPolicy, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	liveCapabilities := make(map[string]struct{}, len(capabilities))
	for _, value := range values {
		value.Capability = strings.ToLower(strings.TrimSpace(value.Capability))
		value.Environment = strings.ToLower(strings.TrimSpace(value.Environment))
		value.Behavior = strings.ToLower(strings.TrimSpace(value.Behavior))
		value.CredentialEnvironment = strings.ToLower(strings.TrimSpace(value.CredentialEnvironment))
		value.Billing = strings.ToLower(strings.TrimSpace(value.Billing))
		if _, exists := declaredCapabilities[value.Capability]; !exists {
			return nil, fmt.Errorf("capability policy tidak dideklarasikan: %s", value.Capability)
		}
		if _, exists := declaredEnvironments[value.Environment]; !exists {
			return nil, fmt.Errorf("environment policy tidak dideklarasikan: %s", value.Environment)
		}
		if _, exists := allowedCapabilityBehaviors[value.Behavior]; !exists {
			return nil, fmt.Errorf("behavior capability tidak didukung: %s", value.Behavior)
		}
		if _, exists := allowedBillingBehaviors[value.Billing]; !exists {
			return nil, fmt.Errorf("billing capability tidak didukung: %s", value.Billing)
		}
		if value.Behavior == "unavailable" {
			value.CredentialEnvironment = ""
		} else if _, exists := declaredEnvironments[value.CredentialEnvironment]; !exists {
			return nil, errors.New("credential_environment wajib mengacu ke environment yang dideklarasikan")
		}
		if value.Behavior == "live_read_only" &&
			value.Capability != "rates" && value.Capability != "tracking" && value.Capability != "balance" {
			return nil, errors.New("live_read_only hanya boleh dipakai capability baca")
		}
		key := value.Capability + ":" + value.Environment
		if _, exists := seen[key]; exists {
			return nil, errors.New("capability policy tidak boleh duplikat")
		}
		seen[key] = struct{}{}
		if value.Environment == providercredentials.EnvironmentLive && value.Behavior != "unavailable" {
			liveCapabilities[value.Capability] = struct{}{}
		}
		result = append(result, value)
	}
	for _, capability := range capabilities {
		if _, exists := liveCapabilities[capability]; !exists {
			return nil, fmt.Errorf("capability %s wajib mempunyai policy live", capability)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Capability != result[j].Capability {
			return result[i].Capability < result[j].Capability
		}
		return result[i].Environment < result[j].Environment
	})
	return result, nil
}

func validateManifestValues(
	label string,
	values []string,
	allowed map[string]struct{},
) error {
	unknown := make([]string, 0)
	for _, value := range values {
		if _, exists := allowed[value]; !exists {
			unknown = append(unknown, value)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("%s tidak didukung: %s", label, strings.Join(unknown, ", "))
	}
	return nil
}

func validateOpenAPI(payload []byte, capabilities []string) ([]string, error) {
	var document struct {
		OpenAPI string                    `yaml:"openapi"`
		Paths   map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(payload, &document); err != nil {
		return nil, errors.New("openapi.yaml bukan YAML yang valid")
	}
	if !strings.HasPrefix(document.OpenAPI, "3.") {
		return nil, errors.New("openapi.yaml wajib menggunakan OpenAPI 3.x")
	}
	requiredMethods := map[string]string{
		"/health":       "get",
		"/capabilities": "get",
		"/services":     "get",
		"/rates":        "post",
	}
	for _, capability := range capabilities {
		switch capability {
		case "shipments":
			requiredMethods["/shipments"] = "post"
		case "pickup":
			requiredMethods["/pickups"] = "post"
		case "tracking":
			requiredMethods["/tracking/waybills"] = "post"
		case "balance":
			requiredMethods["/account/balance"] = "get"
		}
	}
	required := make([]string, 0, len(requiredMethods))
	for requiredPath := range requiredMethods {
		required = append(required, requiredPath)
	}
	sort.Strings(required)
	missing := make([]string, 0)
	for _, requiredPath := range required {
		operations, exists := document.Paths[requiredPath]
		method := requiredMethods[requiredPath]
		if !exists {
			missing = append(missing, strings.ToUpper(method)+" "+requiredPath)
			continue
		}
		if _, exists := operations[method]; !exists {
			missing = append(missing, strings.ToUpper(method)+" "+requiredPath)
		}
	}
	if len(missing) > 0 {
		return required, errors.New("operation canonical belum tersedia: " + strings.Join(missing, ", "))
	}
	return required, nil
}

func validHTTPSURL(value string) bool {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return false
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "localhost" || strings.HasSuffix(hostname, ".localhost") ||
		strings.HasSuffix(hostname, ".local") || strings.HasSuffix(hostname, ".internal") {
		return false
	}
	if ip := net.ParseIP(hostname); ip != nil {
		return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsUnspecified() &&
			!ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast()
	}
	return strings.Contains(hostname, ".")
}

func normalizedUnique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func finalizeReport(report ScanReport) ScanReport {
	report.Passed = true
	for _, check := range report.Checks {
		if check.Status == "failed" {
			report.Passed = false
			break
		}
	}
	return report
}
