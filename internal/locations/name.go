package locations

import (
	"strings"
	"unicode"
)

var regionNameUppercaseWords = map[string]string{
	"di":   "DI",
	"dki":  "DKI",
	"nad":  "NAD",
	"ntb":  "NTB",
	"ntt":  "NTT",
	"i":    "I",
	"ii":   "II",
	"iii":  "III",
	"iv":   "IV",
	"v":    "V",
	"vi":   "VI",
	"vii":  "VII",
	"viii": "VIII",
	"ix":   "IX",
	"x":    "X",
}

// normalizeRegionDisplayName converts provider and official dataset names to
// a stable display form without changing the identity used by location IDs.
// Apostrophes remain inside a word, while hyphens, slashes, parentheses, and
// periods begin a new title-cased segment.
func normalizeRegionDisplayName(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if value == "" {
		return ""
	}

	runes := []rune(strings.ToLower(value))
	capitalizeNext := true
	for index, character := range runes {
		if unicode.IsLetter(character) {
			if capitalizeNext {
				runes[index] = unicode.ToUpper(character)
			}
			capitalizeNext = false
			continue
		}
		if unicode.IsDigit(character) {
			capitalizeNext = false
			continue
		}
		capitalizeNext = character != '\'' && character != '’'
	}

	return restoreRegionNameUppercaseWords(string(runes))
}

func restoreRegionNameUppercaseWords(value string) string {
	var result strings.Builder
	result.Grow(len(value))
	runes := []rune(value)
	for index := 0; index < len(runes); {
		if !unicode.IsLetter(runes[index]) {
			result.WriteRune(runes[index])
			index++
			continue
		}

		end := index + 1
		for end < len(runes) && unicode.IsLetter(runes[end]) {
			end++
		}
		word := string(runes[index:end])
		if replacement, exists := regionNameUppercaseWords[strings.ToLower(word)]; exists {
			result.WriteString(replacement)
		} else {
			result.WriteString(word)
		}
		index = end
	}
	return result.String()
}
