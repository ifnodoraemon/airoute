package storage

import (
	"context"
	"testing"
)

func TestBillingJournalSurvivesAuditFailureAfterDebit(t *testing.T) {
	r, key, user := reservationTestRepo(t, 1, 0)
	hold := reserveForTest(t, r, key, 0.1)
	rec := billingRecord(key, user.Username, "audit-retry", "audit-retry", 0.1)
	rec.ReservationID = hold.ID
	ctx := context.Background()
	if err := r.PendBillingRecord(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`CREATE TRIGGER fail_journal_audit BEFORE INSERT ON usage_logs BEGIN SELECT RAISE(ABORT, 'synthetic audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := r.BatchRecordUsageLogsContext(ctx, []*UsageLogRecord{rec}); err == nil {
		t.Fatal("expected audit failure")
	}
	u, _ := r.GetUserByID(user.ID)
	if u.BalanceUnits != ToUnits(0.9) {
		t.Fatalf("debit should already be committed: %d", u.BalanceUnits)
	}
	var pending int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_usage_journal WHERE billing_id = ?`, rec.BillingID).Scan(&pending); err != nil || pending != 1 {
		t.Fatalf("audit failure lost the durable evidence: count=%d err=%v", pending, err)
	}
	if _, err := r.db.Exec(`DROP TRIGGER fail_journal_audit`); err != nil {
		t.Fatal(err)
	}
	if err := r.recoverBillingJournal(ctx); err != nil {
		t.Fatal(err)
	}
	u, _ = r.GetUserByID(user.ID)
	if u.BalanceUnits != ToUnits(0.9) {
		t.Fatalf("recovery charged twice: %d", u.BalanceUnits)
	}
	var audits int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM usage_logs WHERE billing_id = ?`, rec.BillingID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audit did not recover: count=%d err=%v", audits, err)
	}
}
