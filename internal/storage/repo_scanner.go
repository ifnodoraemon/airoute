package storage

// rowScanner is shared by database/sql single-row and cursor reads.
type rowScanner interface{ Scan(...any) error }
