package storage

import "strings"

// inClause builds the placeholder list and bind arguments for a `column IN (...)`
// list of variable length.
//
// Batch statements in this package take a variable number of ids (or names), and
// building the list by hand in each of them invited off-by-one mistakes: several
// sites prepend a value (the new status) and had to shift every index by one.
// prefix holds those leading arguments; values follow it.
func inClause[T any](prefix []any, values []T) (string, []any) {
	args := make([]any, 0, len(prefix)+len(values))
	args = append(args, prefix...)
	for _, v := range values {
		args = append(args, v)
	}
	if len(values) == 0 {
		return "", args
	}
	return strings.TrimSuffix(strings.Repeat("?,", len(values)), ","), args
}
