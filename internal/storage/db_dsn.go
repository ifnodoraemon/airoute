package storage

import (
	"net/url"
	"regexp"
	"strings"
)

// keywordDSNPasswordPattern matches the credential keys of a libpq keyword/value
// DSN, in both the `password=secret` and `password='secret with spaces'` forms.
var keywordDSNPasswordPattern = regexp.MustCompile(`(?i)\b((?:ssl)?password\s*=\s*)(?:'[^']*'|"[^"]*"|[^\s]+)`)

// redactDSN removes credentials from a database DSN before it is logged.
//
// It handles both supported forms. The previous implementation kept only the
// text after the last "@", which works for postgres://user:pass@host/db but
// leaves the whole string intact for a libpq keyword/value DSN — that form has no
// "@", so "source=host=… password=…" logged the password verbatim.
func redactDSN(dsn string) string {
	if dsn == "" {
		return dsn
	}
	lower := strings.ToLower(dsn)
	if strings.HasPrefix(lower, "postgres://") || strings.HasPrefix(lower, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return keywordDSNPasswordPattern.ReplaceAllString(dsn, "${1}REDACTED")
		}
		if u.User != nil {
			if _, hasPassword := u.User.Password(); hasPassword {
				u.User = url.UserPassword(u.User.Username(), "REDACTED")
			}
		}
		if q := u.Query(); q.Get("password") != "" {
			q.Set("password", "REDACTED")
			u.RawQuery = q.Encode()
		}
		return u.String()
	}
	return keywordDSNPasswordPattern.ReplaceAllString(dsn, "${1}REDACTED")
}
