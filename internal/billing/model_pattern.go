package billing

import "github.com/ifnodoraemon/airoute/internal/model"

// MatchModelPattern reports whether a model name matches a pricing pattern.
//
// The implementation moved to internal/model: the validation rules in
// internal/config need the exact same pattern language, but billing (transitively)
// depends on config, so config cannot import billing without creating a cycle.
// internal/model is a leaf both packages already import. Keeping this wrapper means
// pricing call sites and their tests are unchanged.
func MatchModelPattern(pattern, modelName string) bool {
	return model.MatchModelPattern(pattern, modelName)
}

// patternSpecificity ranks wildcard patterns: more literal characters first, then
// fewer stars. The wildcard index is sorted by it so the most specific rule always
// wins regardless of list or map order.
func patternSpecificity(pattern string) (literal, stars int) {
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '*' {
			stars++
			continue
		}
		literal++
	}
	return literal, stars
}
