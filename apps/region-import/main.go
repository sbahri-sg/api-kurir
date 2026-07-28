package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/emisell/api-kurir/internal/database"
	"github.com/emisell/api-kurir/internal/locations"
)

const (
	wilayahCommit = "f1aad011720e15de4633fefac62b3b14ebb66db8"
	postalCommit  = "ba8497156c5cc9bcbfc527f7b8875d403eda2354"

	defaultWilayahURL = "https://raw.githubusercontent.com/cahyadsn/wilayah/" +
		wilayahCommit + "/db/wilayah.sql"
	defaultPostalURL = "https://raw.githubusercontent.com/cahyadsn/wilayah_kodepos/" +
		postalCommit + "/json/wilayah_kodepos.json"

	expectedWilayahSHA256 = "c4c3396d9380d4edee072af1d9dff83573b574d7cd00a6562cf82e200e954031"
	expectedPostalSHA256  = "fdb972e66e71d37eb20768592d35407b601f0e57db9ffe853bace688734e898e"

	expectedProvinces    = 38
	expectedCities       = 514
	expectedDistricts    = 7_285
	expectedSubdistricts = 83_762
	maxDatasetBytes      = 16 * 1024 * 1024
)

var wilayahTuplePattern = regexp.MustCompile(
	`^\('([0-9.]+)','(.*)'\)[,;]?$`,
)

type options struct {
	wilayahFile string
	postalFile  string
	wilayahURL  string
	postalURL   string
}

type rawRegion struct {
	Code string
	Name string
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("official region import failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	opts := parseOptions()
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	wilayahBytes, err := readDataset(
		ctx,
		opts.wilayahFile,
		opts.wilayahURL,
		expectedWilayahSHA256,
	)
	if err != nil {
		return fmt.Errorf("load Kemendagri region mirror: %w", err)
	}
	postalBytes, err := readDataset(
		ctx,
		opts.postalFile,
		opts.postalURL,
		expectedPostalSHA256,
	)
	if err != nil {
		return fmt.Errorf("load postal code mirror: %w", err)
	}

	regions, err := parseWilayahSQL(wilayahBytes)
	if err != nil {
		return err
	}
	postalCodes := make(map[string]string)
	if err := json.Unmarshal(postalBytes, &postalCodes); err != nil {
		return fmt.Errorf("decode postal code dataset: %w", err)
	}
	records, err := buildOfficialRecords(regions, postalCodes)
	if err != nil {
		return err
	}

	pool, err := database.Open(ctx, databaseURL, 0, 8)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool); err != nil {
		return err
	}

	repository := locations.NewPostgresRepository(pool)
	summary, err := repository.ImportOfficialLocations(
		ctx,
		records,
		[]locations.DatasetMetadata{
			{
				SourceCode:     "kemendagri-wilayah-mirror",
				DatasetVersion: "Kepmendagri-2025-" + wilayahCommit[:12],
				SourceURL:      opts.wilayahURL,
				SourceCommit:   wilayahCommit,
				SHA256:         expectedWilayahSHA256,
				RecordCount:    len(regions),
				Metadata: map[string]any{
					"distribution":         "MIT-licensed GitHub mirror",
					"regulation_reference": "Kepmendagri No. 300.2.2-2430 Tahun 2025",
				},
			},
			{
				SourceCode:     "posindonesia-kodepos-mirror",
				DatasetVersion: "kodepos-2025-" + postalCommit[:12],
				SourceURL:      opts.postalURL,
				SourceCommit:   postalCommit,
				SHA256:         expectedPostalSHA256,
				RecordCount:    len(postalCodes),
				Metadata: map[string]any{
					"distribution": "MIT-licensed GitHub mirror",
					"mapped_by":    "Kemendagri village/kelurahan code",
				},
			},
		},
		opts.postalURL,
	)
	if err != nil {
		return err
	}
	if summary.Provinces != expectedProvinces ||
		summary.Cities != expectedCities ||
		summary.Districts != expectedDistricts ||
		summary.Subdistricts != expectedSubdistricts ||
		summary.PostalLinks != expectedSubdistricts {
		return fmt.Errorf("unexpected imported summary: %#v", summary)
	}

	logger.Info(
		"official region import completed",
		"provinces",
		summary.Provinces,
		"cities",
		summary.Cities,
		"districts",
		summary.Districts,
		"subdistricts",
		summary.Subdistricts,
		"postal_links",
		summary.PostalLinks,
		"wilayah_commit",
		wilayahCommit,
		"postal_commit",
		postalCommit,
	)
	return nil
}

func parseOptions() options {
	wilayahFile := flag.String(
		"wilayah-file",
		"",
		"optional local db/wilayah.sql file",
	)
	postalFile := flag.String(
		"postal-file",
		"",
		"optional local wilayah_kodepos.json file",
	)
	wilayahURL := flag.String(
		"wilayah-url",
		defaultWilayahURL,
		"pinned Kemendagri region mirror URL",
	)
	postalURL := flag.String(
		"postal-url",
		defaultPostalURL,
		"pinned postal code mirror URL",
	)
	flag.Parse()
	return options{
		wilayahFile: strings.TrimSpace(*wilayahFile),
		postalFile:  strings.TrimSpace(*postalFile),
		wilayahURL:  strings.TrimSpace(*wilayahURL),
		postalURL:   strings.TrimSpace(*postalURL),
	}
}

func readDataset(
	ctx context.Context,
	filePath string,
	sourceURL string,
	expectedSHA256 string,
) ([]byte, error) {
	var data []byte
	var err error
	if filePath != "" {
		data, err = os.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
	} else {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
		if err != nil {
			return nil, err
		}
		client := &http.Client{Timeout: 30 * time.Second}
		response, err := client.Do(request)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP %d", response.StatusCode)
		}
		data, err = io.ReadAll(io.LimitReader(response.Body, maxDatasetBytes+1))
		if err != nil {
			return nil, err
		}
	}
	if len(data) == 0 || len(data) > maxDatasetBytes {
		return nil, fmt.Errorf("invalid dataset size: %d bytes", len(data))
	}
	actualHash := sha256.Sum256(data)
	actualSHA256 := hex.EncodeToString(actualHash[:])
	if actualSHA256 != expectedSHA256 {
		return nil, fmt.Errorf(
			"checksum mismatch: got %s expected %s",
			actualSHA256,
			expectedSHA256,
		)
	}
	return data, nil
}

func parseWilayahSQL(data []byte) ([]rawRegion, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	regions := make([]rawRegion, 0, 92_000)
	seen := make(map[string]struct{}, 92_000)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		match := wilayahTuplePattern.FindStringSubmatch(line)
		if len(match) == 0 {
			continue
		}
		code := match[1]
		if _, duplicate := seen[code]; duplicate {
			return nil, fmt.Errorf("duplicate region code %s", code)
		}
		seen[code] = struct{}{}
		regions = append(regions, rawRegion{
			Code: code,
			Name: strings.ReplaceAll(match[2], "''", "'"),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan wilayah SQL: %w", err)
	}
	if len(regions) != expectedProvinces+
		expectedCities+
		expectedDistricts+
		expectedSubdistricts {
		return nil, fmt.Errorf("unexpected region count: %d", len(regions))
	}
	return regions, nil
}

func buildOfficialRecords(
	regions []rawRegion,
	postalCodes map[string]string,
) ([]locations.OfficialLocationRecord, error) {
	byCode := make(map[string]rawRegion, len(regions))
	for _, region := range regions {
		byCode[region.Code] = region
	}
	if len(postalCodes) != expectedSubdistricts {
		return nil, fmt.Errorf("unexpected postal mapping count: %d", len(postalCodes))
	}

	sort.Slice(regions, func(i, j int) bool {
		if len(regions[i].Code) == len(regions[j].Code) {
			return regions[i].Code < regions[j].Code
		}
		return len(regions[i].Code) < len(regions[j].Code)
	})

	records := make([]locations.OfficialLocationRecord, 0, len(regions))
	counts := make(map[string]int, 4)
	for _, region := range regions {
		level, parentCode, err := classifyRegionCode(region.Code)
		if err != nil {
			return nil, err
		}
		counts[level]++
		provinceCode := region.Code[:2]
		province, exists := byCode[provinceCode]
		if !exists {
			return nil, fmt.Errorf("province %s is missing", provinceCode)
		}
		record := locations.OfficialLocationRecord{
			Code:       region.Code,
			ParentCode: parentCode,
			PublicID:   locations.OfficialPublicID(region.Code),
			Level:      level,
			Province:   province.Name,
		}
		switch level {
		case "province":
			record.Province = region.Name
		case "city":
			record.City = region.Name
		case "district":
			city := byCode[region.Code[:5]]
			record.City = city.Name
			record.District = region.Name
		case "subdistrict":
			city := byCode[region.Code[:5]]
			district := byCode[region.Code[:8]]
			record.City = city.Name
			record.District = district.Name
			record.Subdistrict = region.Name
			record.PostalCode = strings.TrimSpace(postalCodes[region.Code])
			if !validPostalCode(record.PostalCode) {
				return nil, fmt.Errorf(
					"invalid or missing postal code for %s",
					region.Code,
				)
			}
		}
		records = append(records, record)
	}

	if counts["province"] != expectedProvinces ||
		counts["city"] != expectedCities ||
		counts["district"] != expectedDistricts ||
		counts["subdistrict"] != expectedSubdistricts {
		return nil, fmt.Errorf("unexpected hierarchy counts: %#v", counts)
	}
	for code, postalCode := range postalCodes {
		region, exists := byCode[code]
		if !exists || len(region.Code) != 13 || !validPostalCode(postalCode) {
			return nil, fmt.Errorf("orphan or invalid postal mapping %s", code)
		}
	}
	return records, nil
}

func classifyRegionCode(code string) (string, string, error) {
	switch len(code) {
	case 2:
		return "province", "", nil
	case 5:
		return "city", code[:2], nil
	case 8:
		return "district", code[:5], nil
	case 13:
		return "subdistrict", code[:8], nil
	default:
		return "", "", fmt.Errorf("invalid Kemendagri region code %q", code)
	}
}

func validPostalCode(value string) bool {
	if len(value) != 5 || value == "00000" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
