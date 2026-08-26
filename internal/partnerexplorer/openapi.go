package partnerexplorer

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var operationIDCleaner = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

var supportedMethods = map[string]struct{}{
	"get": {}, "head": {}, "options": {}, "post": {}, "put": {}, "patch": {}, "delete": {},
}

type openAPIDocument struct {
	OpenAPI    string                    `yaml:"openapi"`
	Servers    []map[string]any          `yaml:"servers"`
	Paths      map[string]map[string]any `yaml:"paths"`
	Components struct {
		Schemas         map[string]map[string]any `yaml:"schemas"`
		SecuritySchemes map[string]map[string]any `yaml:"securitySchemes"`
	} `yaml:"components"`
}

func operationsFromArchive(payload []byte, fallbackBaseURL string) ([]Operation, error) {
	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return nil, errors.New("package ZIP tidak dapat dibaca")
	}
	var openAPI []byte
	for _, entry := range reader.File {
		if path.Clean(strings.ReplaceAll(entry.Name, "\\", "/")) != "openapi.yaml" {
			continue
		}
		handle, err := entry.Open()
		if err != nil {
			return nil, errors.New("openapi.yaml tidak dapat dibaca")
		}
		openAPI, err = io.ReadAll(io.LimitReader(handle, 4*1024*1024+1))
		_ = handle.Close()
		if err != nil || len(openAPI) > 4*1024*1024 {
			return nil, errors.New("openapi.yaml melebihi batas pengujian")
		}
		break
	}
	if len(openAPI) == 0 {
		return nil, errors.New("openapi.yaml tidak ditemukan pada root package")
	}
	return parseOperations(openAPI, fallbackBaseURL)
}

func parseOperations(payload []byte, fallbackBaseURLs ...string) ([]Operation, error) {
	var document openAPIDocument
	if err := yaml.Unmarshal(payload, &document); err != nil || !strings.HasPrefix(document.OpenAPI, "3.") {
		return nil, errors.New("OpenAPI 3.x tidak valid")
	}
	operations := make([]Operation, 0)
	fallbackBaseURL := ""
	if len(fallbackBaseURLs) > 0 {
		fallbackBaseURL = strings.TrimSpace(fallbackBaseURLs[0])
	}
	documentBaseURL := firstServerURL(document.Servers)
	if documentBaseURL == "" {
		documentBaseURL = fallbackBaseURL
	}
	seenIDs := make(map[string]struct{})
	for endpointPath, pathItem := range document.Paths {
		if !strings.HasPrefix(endpointPath, "/") || len(endpointPath) > 512 {
			continue
		}
		for method, rawValue := range pathItem {
			method = strings.ToLower(strings.TrimSpace(method))
			if _, supported := supportedMethods[method]; !supported {
				continue
			}
			raw, ok := rawValue.(map[string]any)
			if !ok {
				continue
			}
			operation := operationFromMap(
				method, endpointPath, raw, document.Components.Schemas,
				document.Components.SecuritySchemes, documentBaseURL,
			)
			baseID := operation.ID
			for suffix := 2; ; suffix++ {
				if _, exists := seenIDs[operation.ID]; !exists {
					break
				}
				operation.ID = fmt.Sprintf("%s-%d", baseID, suffix)
			}
			seenIDs[operation.ID] = struct{}{}
			operations = append(operations, operation)
		}
	}
	if len(operations) == 0 {
		return nil, errors.New("OpenAPI tidak memiliki operation yang dapat diuji")
	}
	sort.Slice(operations, func(i, j int) bool {
		if operations[i].Capability != operations[j].Capability {
			return operations[i].Capability < operations[j].Capability
		}
		if operations[i].Path != operations[j].Path {
			return operations[i].Path < operations[j].Path
		}
		return operations[i].Method < operations[j].Method
	})
	return operations, nil
}

func operationFromMap(
	method, endpointPath string,
	raw map[string]any,
	schemas map[string]map[string]any,
	securitySchemes map[string]map[string]any,
	documentBaseURL string,
) Operation {
	operationID := stringValue(raw["operationId"])
	if operationID == "" {
		operationID = strings.ToLower(method) + "-" + operationIDCleaner.ReplaceAllString(strings.Trim(endpointPath, "/"), "-")
		operationID = strings.Trim(operationID, "-")
	}
	operation := Operation{
		ID:          operationID,
		Method:      strings.ToUpper(method),
		Path:        endpointPath,
		Summary:     stringValue(raw["summary"]),
		Description: stringValue(raw["description"]),
		Capability:  capabilityForPath(endpointPath),
		Safety:      safetyForOperation(method, endpointPath),
		BaseURL:     operationBaseURL(raw, documentBaseURL),
		Parameters:  make([]Parameter, 0),
	}
	credential := operationCredential(raw, securitySchemes)
	operation.CredentialCode = credential.Code
	operation.CredentialLabel = credential.Label
	operation.CredentialDescription = credential.Description
	operation.AuthHeader = credential.AuthHeader
	operation.AuthPrefix = credential.AuthPrefix
	if operation.Summary == "" {
		operation.Summary = operation.Method + " " + endpointPath
	}
	if parameters, ok := raw["parameters"].([]any); ok {
		for _, value := range parameters {
			parameterMap, ok := value.(map[string]any)
			if !ok {
				continue
			}
			schema, _ := parameterMap["schema"].(map[string]any)
			parameter := Parameter{
				Name:        stringValue(parameterMap["name"]),
				In:          stringValue(parameterMap["in"]),
				Required:    boolValue(parameterMap["required"]),
				Description: stringValue(parameterMap["description"]),
				Type:        stringValue(schema["type"]),
				Example:     exampleString(schema),
			}
			if parameter.Name != "" && (parameter.In == "path" || parameter.In == "query") {
				operation.Parameters = append(operation.Parameters, parameter)
			}
		}
	}
	if requestBody, ok := raw["requestBody"].(map[string]any); ok {
		if content, ok := requestBody["content"].(map[string]any); ok {
			if media, ok := content["application/json"].(map[string]any); ok {
				example := media["example"]
				if example == nil {
					if schema, ok := media["schema"].(map[string]any); ok {
						example = exampleFromSchema(schema, schemas, 0, map[string]bool{})
					}
				}
				if example != nil {
					if encoded, err := json.MarshalIndent(example, "", "  "); err == nil {
						operation.RequestExample = encoded
					}
				}
			}
		}
	}
	return operation
}

func operationBaseURL(raw map[string]any, fallback string) string {
	if servers, ok := raw["servers"].([]any); ok {
		converted := make([]map[string]any, 0, len(servers))
		for _, value := range servers {
			if server, ok := value.(map[string]any); ok {
				converted = append(converted, server)
			}
		}
		if value := firstServerURL(converted); value != "" {
			return value
		}
	}
	return strings.TrimSpace(fallback)
}

func firstServerURL(servers []map[string]any) string {
	for _, server := range servers {
		if value := stringValue(server["url"]); value != "" && !strings.Contains(value, "{") {
			return value
		}
	}
	return ""
}

func operationCredential(raw map[string]any, schemes map[string]map[string]any) CredentialState {
	security, ok := raw["security"].([]any)
	if !ok || len(security) == 0 {
		return CredentialState{}
	}
	for _, requirementValue := range security {
		requirement, ok := requirementValue.(map[string]any)
		if !ok {
			continue
		}
		keys := make([]string, 0, len(requirement))
		for key := range requirement {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if profile, ok := credentialProfile(key, schemes[key]); ok {
				return profile
			}
		}
	}
	return CredentialState{}
}

func credentialStatesFromOperations(operations []Operation) []CredentialState {
	profiles := make(map[string]CredentialState)
	for _, operation := range operations {
		if operation.CredentialCode == "" {
			continue
		}
		profiles[operation.CredentialCode] = CredentialState{
			Code: operation.CredentialCode, Label: operation.CredentialLabel,
			Description: operation.CredentialDescription,
			AuthHeader:  operation.AuthHeader, AuthPrefix: operation.AuthPrefix,
		}
	}
	result := make([]CredentialState, 0, len(profiles))
	for _, profile := range profiles {
		result = append(result, profile)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Code < result[j].Code })
	return result
}

func credentialProfile(schemeName string, scheme map[string]any) (CredentialState, bool) {
	code := normalizeCredentialCode(stringValue(scheme["x-emisell-credential-code"]))
	if code == "" {
		code = normalizeCredentialCode(schemeName)
	}
	if code == "" {
		return CredentialState{}, false
	}
	profile := CredentialState{
		Code: code, Label: stringValue(scheme["x-emisell-label"]),
		Description: stringValue(scheme["description"]),
	}
	if profile.Label == "" {
		profile.Label = schemeName
	}
	switch strings.ToLower(stringValue(scheme["type"])) {
	case "apikey":
		if strings.EqualFold(stringValue(scheme["in"]), "header") {
			profile.AuthHeader = stringValue(scheme["name"])
		}
	case "http":
		if strings.EqualFold(stringValue(scheme["scheme"]), "bearer") {
			profile.AuthHeader = "Authorization"
			profile.AuthPrefix = "Bearer"
		}
	}
	if !validHeaderName.MatchString(profile.AuthHeader) {
		return CredentialState{}, false
	}
	if _, blocked := blockedAuthHeaders[strings.ToLower(profile.AuthHeader)]; blocked {
		return CredentialState{}, false
	}
	return profile, true
}

func normalizeCredentialCode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = operationIDCleaner.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_")
	if !validCredentialCode.MatchString(value) {
		return ""
	}
	return value
}

func exampleFromSchema(
	schema map[string]any,
	schemas map[string]map[string]any,
	depth int,
	visiting map[string]bool,
) any {
	if depth > 5 {
		return nil
	}
	if value, exists := schema["example"]; exists {
		return value
	}
	if value, exists := schema["default"]; exists {
		return value
	}
	if values, ok := schema["enum"].([]any); ok && len(values) > 0 {
		return values[0]
	}
	if reference := stringValue(schema["$ref"]); reference != "" {
		const prefix = "#/components/schemas/"
		if !strings.HasPrefix(reference, prefix) || visiting[reference] {
			return nil
		}
		resolved, ok := schemas[strings.TrimPrefix(reference, prefix)]
		if !ok {
			return nil
		}
		visiting[reference] = true
		defer delete(visiting, reference)
		return exampleFromSchema(resolved, schemas, depth+1, visiting)
	}
	if combined, ok := schema["allOf"].([]any); ok {
		result := make(map[string]any)
		for _, entry := range combined {
			part, _ := entry.(map[string]any)
			if generated, ok := exampleFromSchema(part, schemas, depth+1, visiting).(map[string]any); ok {
				for key, value := range generated {
					result[key] = value
				}
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	if variants, ok := schema["oneOf"].([]any); ok && len(variants) > 0 {
		variant, _ := variants[0].(map[string]any)
		return exampleFromSchema(variant, schemas, depth+1, visiting)
	}
	typeName := stringValue(schema["type"])
	properties, _ := schema["properties"].(map[string]any)
	if typeName == "object" || len(properties) > 0 {
		result := make(map[string]any)
		keys := make([]string, 0, len(properties))
		for key := range properties {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			property, _ := properties[key].(map[string]any)
			if value := exampleFromSchema(property, schemas, depth+1, visiting); value != nil {
				result[key] = value
			}
		}
		return result
	}
	switch typeName {
	case "array":
		item, _ := schema["items"].(map[string]any)
		if value := exampleFromSchema(item, schemas, depth+1, visiting); value != nil {
			return []any{value}
		}
		return []any{}
	case "integer", "number":
		return 0
	case "boolean":
		return false
	case "string":
		switch stringValue(schema["format"]) {
		case "date-time":
			return "2026-08-26T10:00:00+07:00"
		case "date":
			return "2026-08-26"
		case "uuid":
			return "00000000-0000-4000-8000-000000000000"
		default:
			return "string"
		}
	default:
		return nil
	}
}

func capabilityForPath(endpointPath string) string {
	value := strings.ToLower(endpointPath)
	switch {
	case strings.Contains(value, "/rate"):
		return "rates"
	case strings.Contains(value, "/tracking") || strings.Contains(value, "/waybill"):
		return "tracking"
	case strings.Contains(value, "/pickup"):
		return "pickup"
	case strings.Contains(value, "/shipment") || strings.Contains(value, "/order"):
		return "shipments"
	case strings.Contains(value, "/balance"):
		return "balance"
	case strings.Contains(value, "/service"):
		return "services"
	default:
		return "platform"
	}
}

func safetyForOperation(method, endpointPath string) string {
	method = strings.ToLower(method)
	if method == "get" || method == "head" || method == "options" {
		return "read_only"
	}
	endpointPath = strings.ToLower(strings.TrimRight(endpointPath, "/"))
	if method == "post" && (endpointPath == "/rates" || endpointPath == "/tracking/waybills") {
		return "read_only"
	}
	return "transactional_locked"
}

func stringValue(value any) string {
	result, _ := value.(string)
	return strings.TrimSpace(result)
}

func boolValue(value any) bool {
	result, _ := value.(bool)
	return result
}

func exampleString(schema map[string]any) string {
	for _, key := range []string{"example", "default"} {
		if value, exists := schema[key]; exists {
			return fmt.Sprint(value)
		}
	}
	if values, ok := schema["enum"].([]any); ok && len(values) > 0 {
		return fmt.Sprint(values[0])
	}
	return ""
}
