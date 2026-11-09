package storage

import (
	"fmt"
	"strings"
)

// Rebind adapts ANSI SQL queries containing '?' parameters to target engine syntax ($1, $2 for Postgres).
func (db *DB) Rebind(query string) string {
	if db == nil || db.Dialect() != "postgres" {
		return query
	}

	var buf strings.Builder
	argIdx := 1
	inString := false

	for i := 0; i < len(query); i++ {
		c := query[i]
		if c == '\'' {
			inString = !inString
			buf.WriteByte(c)
		} else if c == '?' && !inString {
			buf.WriteString(fmt.Sprintf("$%d", argIdx))
			argIdx++
		} else {
			buf.WriteByte(c)
		}
	}
	s := buf.String()
	// PostgreSQL is case-insensitive only with ILIKE. Rewrite the LIKE operator
	// explicitly (never inside string literals, which the scanner above already
	// left untouched because they contain no '?' placeholders).
	s = rewriteLikeToILike(s)
	if strings.Contains(s, "INSERT OR IGNORE INTO") {
		s = strings.ReplaceAll(s, "INSERT OR IGNORE INTO", "INSERT INTO")
		if !strings.Contains(strings.ToUpper(s), "ON CONFLICT") {
			s = s + " ON CONFLICT DO NOTHING"
		}
	}
	return s
}

// withReturningID appends "RETURNING id" to a statement, tolerating a trailing
// semicolon and skipping statements that already ask for a returning clause.
func withReturningID(query string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(query), ";")
	if strings.Contains(strings.ToUpper(trimmed), " RETURNING ") {
		return trimmed
	}
	return trimmed + " RETURNING id"
}

// rewriteLikeToILike converts the SQL LIKE operator to PostgreSQL's ILIKE while
// leaving string literals intact.
func rewriteLikeToILike(query string) string {
	var out strings.Builder
	out.Grow(len(query) + 8)
	inString := false
	for i := 0; i < len(query); i++ {
		ch := query[i]
		if ch == '\'' {
			inString = !inString
			out.WriteByte(ch)
			continue
		}
		if !inString && (ch == 'L' || ch == 'l') && i+4 <= len(query) {
			candidate := query[i : i+4]
			if strings.EqualFold(candidate, "LIKE") {
				prevBoundary := i == 0 || isSQLBoundary(query[i-1])
				nextBoundary := i+4 == len(query) || isSQLBoundary(query[i+4])
				if prevBoundary && nextBoundary {
					out.WriteString("ILIKE")
					i += 3
					continue
				}
			}
		}
		out.WriteByte(ch)
	}
	return out.String()
}

func isSQLBoundary(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '(', ')', ',', '=', '<', '>', '!', '+', '-', '*', '/', ';':
		return true
	}
	return false
}
