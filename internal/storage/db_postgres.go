package storage

import (
	"fmt"
	"net/url"
	"strings"
)

// PostgresStatementTimeoutMS bounds any single server-side statement.
//
// The repository layer is mostly context-free (a full ExecContext/QueryContext
// migration is a much larger refactor), so a runaway query on the control plane
// would otherwise pin a pooled connection indefinitely. Setting the limit in the
// connection string makes the *server* abort it, which covers every call site
// including ones that never had a deadline.
const PostgresStatementTimeoutMS = 30000

// PostgresLockTimeoutMS bounds how long a statement may wait for a row/table
// lock before the server aborts it with 55P03 (lock_not_available).
//
// Settlement is a short write transaction on a hot wallet row: when two flushes
// contend, waiting the full statement_timeout pins the goroutine (and its
// connection) for 30s while the request-side wait expires anyway. Failing fast
// at 500ms instead turns a serialisation delay into a retryable error, which
// settleCharges retries with jitter.
const PostgresLockTimeoutMS = 500

// ApplyPostgresStatementTimeouts injects statement_timeout (and an idle
// transaction timeout) into a libpq DSN unless the operator already configured
// `options` themselves. Both the URL form and the keyword/value form are
// supported; anything unrecognised is returned unchanged.
func ApplyPostgresStatementTimeouts(dsn string) string {
	if strings.TrimSpace(dsn) == "" {
		return dsn
	}
	opts := fmt.Sprintf("-c statement_timeout=%d -c lock_timeout=%d -c idle_in_transaction_session_timeout=%d",
		PostgresStatementTimeoutMS, PostgresLockTimeoutMS, PostgresStatementTimeoutMS)

	lower := strings.ToLower(dsn)
	if strings.HasPrefix(lower, "postgres://") || strings.HasPrefix(lower, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return dsn
		}
		q := u.Query()
		if q.Get("options") != "" {
			return dsn // operator-provided options win
		}
		q.Set("options", opts)
		u.RawQuery = q.Encode()
		return u.String()
	}

	// Keyword/value DSN: host=... dbname=... [options='...']
	if strings.Contains(lower, "options=") {
		return dsn
	}
	return dsn + " options='" + opts + "'"
}
