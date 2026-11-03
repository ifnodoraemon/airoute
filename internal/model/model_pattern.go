package model

import "strings"

// MatchModelPattern reports whether a model name matches a glob pattern.
//
// This is the single implementation of the model-pattern language, shared by the
// pricing rules (internal/billing) and the format-validation rules
// (internal/config.ModelValidationRule.Model). It lives in this leaf package
// because config and billing already import model but billing (transitively)
// imports config, so neither of them may import the other:
//
//	exact            "gpt-4o"          matches only "gpt-4o"
//	prefix wildcard  "gpt-4o*"         matches "gpt-4o", "gpt-4o-mini"
//	suffix wildcard  "*o1"             matches "o1", "openai/o1"
//	infix wildcard   "gpt-4o*mini"     matches "gpt-4o-mini"
//	catch-all        "*"               matches everything
//
// A "*" stands for any run of characters, including the "/" that separates a
// provider prefix from the upstream model name. Pricing used to accept only a
// trailing "*" and stored every other pattern as an exact model name, so "*o1" or
// "gpt-4o*mini" could be saved, displayed and never match anything - the rule
// existed but silently fell through to the default rate. The validation rules
// carried a second, subtly different implementation of the same language.
//
// Matching is case-insensitive because stored rates and requested model names are
// both lowercased before lookup.
func MatchModelPattern(pattern, matchModel string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	matchModel = strings.ToLower(strings.TrimSpace(matchModel))
	if pattern == matchModel {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return false
	}
	return globMatch(pattern, matchModel)
}

// globMatch matches s against a pattern where "*" matches any sequence of bytes
// (including the empty one). It walks both strings once and backtracks to the last
// star instead of recursing, so a pathological pattern cannot blow the stack.
func globMatch(pattern, s string) bool {
	patternByte, textByte := 0, 0
	lastStar, starText := -1, 0
	for textByte < len(s) {
		if patternByte < len(pattern) && pattern[patternByte] == '*' {
			lastStar, starText = patternByte, textByte
			patternByte++
			continue
		}
		if patternByte < len(pattern) && pattern[patternByte] == s[textByte] {
			patternByte++
			textByte++
			continue
		}
		if lastStar < 0 {
			return false
		}
		// Let the last star absorb one more byte and retry the rest of the pattern.
		starText++
		patternByte, textByte = lastStar+1, starText
	}
	for patternByte < len(pattern) && pattern[patternByte] == '*' {
		patternByte++
	}
	return patternByte == len(pattern)
}
