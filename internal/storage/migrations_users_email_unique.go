package storage

import "github.com/ifnodoraemon/airoute/internal/telemetry"

// usersEmailUniqueIndex is the name of the partial unique index created by
// migration 27. The registration path also matches on it when classifying a
// driver unique-violation error.
const usersEmailUniqueIndex = "uq_users_email_lower"

// createUsersEmailUniqueIndex adds case-insensitive uniqueness for the email
// login identifier.
//
// users.email used to be a plain column with a non-unique index, while
// registration did a check-then-insert (GetUserByEmail, then CreateUser). Two
// concurrent registrations carrying the same address could therefore both pass
// the check and create accounts that share an email; GetUserByEmail has no
// ORDER BY, so email login and OAuth account binding could then resolve either
// row. The unique index makes the database the authority, and the repository
// maps the resulting violation to ErrEmailAlreadyRegistered.
//
// The index is partial: accounts without an email address (NULL or ”) keep
// working and are not constrained, including the many legacy rows created before
// email was collected.
//
// Existing deployments that already contain duplicates would fail the DDL. That
// must not abort startup (it would leave the gateway down after an upgrade), so
// the failure is reported with the remediation instead; registration keeps its
// own conflict check as a fallback until the data is cleaned up.
func createUsersEmailUniqueIndex(db *DB) error {
	statement := `CREATE UNIQUE INDEX IF NOT EXISTS ` + usersEmailUniqueIndex + ` ON users (LOWER(email)) WHERE email <> ''`
	if _, err := db.DB.Exec(statement); err != nil {
		telemetry.ScopedLogger("system").Warn(
			"could not create the case-insensitive unique index on users.email; duplicate email addresses may still exist",
			"error", err.Error(),
			"remediation", "merge or clear duplicate users.email values, then restart airoute to enforce uniqueness",
		)
		return nil
	}
	return nil
}
