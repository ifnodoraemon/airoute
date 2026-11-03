package storage

import (
	"math"
	"strings"
	"testing"
)

// TestRewriteLegacyUsageLogReasons covers the two history repairs: the
// "%!w(<nil>)" formulation artefact and the synthetic 499 with no reason.
// Rows that already carry a reason, or a different status, must not be touched.
func TestRewriteLegacyUsageLogReasons(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	fixtures := []struct {
		match  string
		status int
		errMsg string
		want   string
	}{
		{"candidate", 502, "all 1 candidate channels failed for model deepseek-v4.1-flash. Last error: %!w(<nil>)", "all 1 candidate channels failed for model deepseek-v4.1-flash. Last error: " + legacyNilWrapReason},
		{"stream", 502, "all channels failed for stream request on model aliyun/glm-5.3. Last error: %!w(<nil>)", "all channels failed for stream request on model aliyun/glm-5.3. Last error: " + legacyNilWrapReason},
		{"empty-499", 499, "", legacySynthetic499Reason},
		{"kept-499", 499, "client closed request: real reason", "client closed request: real reason"},
		{"kept-502", 502, "upstream 罗总 returned status 503", "upstream 罗总 returned status 503"},
		{"empty-200", 200, "", ""},
	}
	for _, f := range fixtures {
		if _, err := db.Exec(`INSERT INTO usage_logs (trace_id, model, status_code, error_message) VALUES (?, 'm', ?, ?)`,
			f.match, f.status, f.errMsg); err != nil {
			t.Fatal(err)
		}
	}

	// Twice: a replayed migration must be a no-op, not a second rewrite.
	for i := 0; i < 2; i++ {
		if err := rewriteLegacyUsageLogReasons(db); err != nil {
			t.Fatalf("rewrite pass %d: %v", i, err)
		}
		for _, f := range fixtures {
			var got string
			if err := db.QueryRow(`SELECT error_message FROM usage_logs WHERE trace_id = ?`, f.match).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != f.want {
				t.Fatalf("pass %d %s: error_message = %q, want %q", i, f.match, got, f.want)
			}
		}
	}

	// The reconstructed text must never appear twice from a replay.
	var doubled int
	if err := db.QueryRow(`SELECT COUNT(*) FROM usage_logs WHERE error_message LIKE ?`, "%"+legacySynthetic499Reason+"%"+legacySynthetic499Reason+"%").Scan(&doubled); err != nil {
		t.Fatal(err)
	}
	if doubled != 0 {
		t.Fatalf("%d rows carry the reason twice", doubled)
	}
}

// TestRepairDeletedAuditCounterDrift pins the write-off to its exact fingerprint:
// the matching key is repaired, an unrelated drift is left alone, and a replay
// changes nothing.
func TestRepairDeletedAuditCounterDrift(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const (
		matchKey = "sk-f9-match"
		otherKey = "sk-unrelated-drift"
	)
	settledUnits := int64(1000000000) // ¥1.000000 settled on the ledger
	if _, err := db.Exec(`INSERT INTO api_keys (id, key, tenant_id, used_cost, used_cost_units) VALUES (1, ?, 'tenant-a', ?, ?)`,
		matchKey, 1.0+f9DeletedAuditArtifactCNY(), settledUnits+f9DeletedAuditArtifactUnits); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO billing_settlements (billing_id, api_key, tenant_id, cost_units) VALUES ('b-1', ?, 'tenant-a', ?)`,
		matchKey, settledUnits); err != nil {
		t.Fatal(err)
	}

	// A key that is merely out of balance by an unrelated amount must survive.
	if _, err := db.Exec(`INSERT INTO api_keys (id, key, tenant_id, used_cost, used_cost_units) VALUES (2, ?, 'tenant-b', 5.5, 5500000000)`,
		otherKey); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO billing_settlements (billing_id, api_key, tenant_id, cost_units) VALUES ('b-2', ?, 'tenant-b', 5000000000)`,
		otherKey); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		if err := repairDeletedAuditCounterDrift(db); err != nil {
			t.Fatalf("repair pass %d: %v", i, err)
		}
	}

	var units int64
	var cny float64
	if err := db.QueryRow(`SELECT used_cost_units, used_cost FROM api_keys WHERE id = 1`).Scan(&units, &cny); err != nil {
		t.Fatal(err)
	}
	if units != settledUnits {
		t.Fatalf("repaired units = %d, want %d", units, settledUnits)
	}
	if math.Abs(cny-1.0) > 1e-9 {
		t.Fatalf("repaired used_cost = %v, want 1.0", cny)
	}

	var otherUnits int64
	var otherCNY float64
	if err := db.QueryRow(`SELECT used_cost_units, used_cost FROM api_keys WHERE id = 2`).Scan(&otherUnits, &otherCNY); err != nil {
		t.Fatal(err)
	}
	if otherUnits != 5500000000 || math.Abs(otherCNY-5.5) > 1e-9 {
		t.Fatalf("unrelated drift was modified: units=%d cny=%v", otherUnits, otherCNY)
	}

	var note string
	if err := db.QueryRow(`SELECT value FROM system_settings WHERE key = ?`, f9DeletedAuditWriteOffKey).Scan(&note); err != nil {
		t.Fatalf("write-off note missing: %v", err)
	}
	if !strings.Contains(note, "F9") || !strings.Contains(note, "units=2647492000") {
		t.Fatalf("write-off note does not describe the artefact: %q", note)
	}

	// The ledger invariant the reconciler checks must now hold.
	var drift int64
	if err := db.QueryRow(`SELECT COALESCE(SUM(CASE WHEN COALESCE(used_cost_units,0) >= COALESCE(s.settled,0)
			THEN COALESCE(used_cost_units,0) - COALESCE(s.settled,0)
			ELSE COALESCE(s.settled,0) - COALESCE(used_cost_units,0) END), 0)
		FROM api_keys k
		LEFT JOIN (SELECT api_key, SUM(cost_units) AS settled FROM billing_settlements GROUP BY api_key) s ON s.api_key = k.key
		WHERE k.id = 1`).Scan(&drift); err != nil {
		t.Fatal(err)
	}
	if drift != 0 {
		t.Fatalf("key 1 still drifts by %d units", drift)
	}
}
