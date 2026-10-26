package storage

// reclassifyClientCanceled repairs audit rows that were labelled as upstream
// failures before cancellation was understood.
//
// The dispatcher returns ctx.Err() when the downstream goes away, and that error
// used to fall through to the generic 502 "upstream_failure" label. On the live
// cluster that mislabelled 259 rows: clients hanging up (or their HTTP timeouts
// firing) looked like provider outages, and they counted against the gateway's
// error rate. Recording them as 499 (client closed request) matches what
// actually happened; the charge for those rows is already zero because no tokens
// were consumed.
func reclassifyClientCanceledUsageLogs(db *DB) error {
	_, err := db.DB.Exec(`UPDATE usage_logs SET status_code = 499 WHERE status_code = 502 AND error_message = 'context canceled'`)
	return err
}
