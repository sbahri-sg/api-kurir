package tracking

import (
	"regexp"
	"sort"
	"strings"
)

type waybillPattern struct {
	courier string
	pattern *regexp.Regexp
	score   int
}

// These patterns are soft hints, not a source of truth. Providers can introduce
// new formats without a release, therefore an unknown format is still checked
// against the courier selected by the seller.
var waybillPatterns = []waybillPattern{
	{courier: "jnt", pattern: regexp.MustCompile(`^J[A-Z0-9]{11}$`), score: 100},
	{courier: "jne", pattern: regexp.MustCompile(`^[A-Z]{1,3}[0-9]{9,13}$`), score: 82},
	{courier: "jne", pattern: regexp.MustCompile(`^[0-9]{13,16}$`), score: 78},
	{courier: "sicepat", pattern: regexp.MustCompile(`^[0-9]{12}$`), score: 75},
	{courier: "tiki", pattern: regexp.MustCompile(`^[0-9]{12}$`), score: 72},
	{courier: "pos", pattern: regexp.MustCompile(`^[A-Z]{2}[0-9]{9}[A-Z]{2}$`), score: 100},
	{courier: "pos", pattern: regexp.MustCompile(`^[A-Z0-9]{11,14}$`), score: 65},
	{courier: "lion", pattern: regexp.MustCompile(`^[A-Z0-9-]{8,17}$`), score: 55},
	{courier: "anteraja", pattern: regexp.MustCompile(`^[0-9]{14}$`), score: 92},
	{courier: "wahana", pattern: regexp.MustCompile(`^[A-Z0-9]{8}$`), score: 95},
	{courier: "ninja", pattern: regexp.MustCompile(`^NLID[A-Z0-9]{12}$`), score: 100},
	{courier: "ide", pattern: regexp.MustCompile(`^ID[A-Z0-9]{13}$`), score: 95},
	{courier: "rpx", pattern: regexp.MustCompile(`^[A-Z0-9]{6,12}$`), score: 50},
	{courier: "sap", pattern: regexp.MustCompile(`^[A-Z0-9]{16}$`), score: 70},
	{courier: "sentral", pattern: regexp.MustCompile(`^[A-Z0-9]{6,12}$`), score: 45},
}

func rankedCourierCandidates(waybill, requested string, supported []string) ([]string, string) {
	waybill = strings.ToUpper(strings.TrimSpace(waybill))
	requested = strings.ToLower(strings.TrimSpace(requested))
	supportedSet := make(map[string]struct{}, len(supported))
	for _, code := range supported {
		supportedSet[strings.ToLower(strings.TrimSpace(code))] = struct{}{}
	}
	type scored struct {
		code  string
		score int
	}
	scores := make(map[string]int)
	for _, rule := range waybillPatterns {
		if _, ok := supportedSet[rule.courier]; !ok || !rule.pattern.MatchString(waybill) {
			continue
		}
		if rule.score > scores[rule.courier] {
			scores[rule.courier] = rule.score
		}
	}
	items := make([]scored, 0, len(scores))
	for code, score := range scores {
		items = append(items, scored{code: code, score: score})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].score == items[j].score {
			return items[i].code < items[j].code
		}
		return items[i].score > items[j].score
	})
	result := make([]string, 0, len(items)+1)
	if _, ok := supportedSet[requested]; ok && requested != "" {
		result = append(result, requested)
	}
	for _, item := range items {
		if item.code != requested {
			result = append(result, item.code)
		}
	}
	formatStatus := "unclassified"
	if len(items) > 0 {
		formatStatus = "possible"
	}
	if scores[requested] > 0 {
		formatStatus = "matched"
	}
	return result, formatStatus
}
