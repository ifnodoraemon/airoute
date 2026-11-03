package storage

import (
	"strings"
	"time"
)

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

// dialectTime binds a timestamp value for the active dialect.
//
// SQLite stores DATETIME as fixed-width UTC text, while PostgreSQL columns are
// real TIMESTAMPTZ values. Binding UTC text to a TIMESTAMPTZ column does not
// mean "this instant": PostgreSQL parses a zone-less literal in the session's
// TimeZone, so on any server that is not UTC the stored instant shifts by the
// offset. Every timestamp written to (or compared against) such a column must
// therefore go through this helper.
//
// The layout is second-precision because that is the historical on-disk shape of
// every table except usage_logs; use dialectTimePrecise where microseconds
// matter.
func (r *Repository) dialectTime(at time.Time) any {
	if r != nil && r.db != nil && r.db.Dialect() == "postgres" {
		return at.UTC()
	}
	return at.UTC().Format(legacySecondTimestampLayout)
}

// dialectTimePrecise is dialectTime with the microsecond layout used by
// usage_logs.created_at (and the ledger rows copied from it), so a burst of rows
// keeps a strict order instead of collapsing onto the same second.
func (r *Repository) dialectTimePrecise(at time.Time) any {
	if r != nil && r.db != nil && r.db.Dialect() == "postgres" {
		return at.UTC()
	}
	return at.UTC().Format(sqliteTimestampLayout)
}
