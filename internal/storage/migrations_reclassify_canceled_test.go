package storage

import "testing"

func TestReclassifyClientCanceledUsageLogs(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows := []struct {
		trace  string
		status int
		errMsg string
		want   int
	}{
		{"tr-canceled", 502, "context canceled", 499},
		{"tr-upstream", 502, "connection reset by peer", 502},
		{"tr-ok", 200, "", 200},
	}
	for _, row := range rows {
		if _, err := db.Exec(`INSERT INTO usage_logs (trace_id, model, status_code, error_message) VALUES (?, 'm', ?, ?)`,
			row.trace, row.status, row.errMsg); err != nil {
			t.Fatal(err)
		}
	}

	if err := reclassifyClientCanceledUsageLogs(db); err != nil {
		t.Fatalf("reclassify: %v", err)
	}
	for _, row := range rows {
		var got int
		if err := db.QueryRow(`SELECT status_code FROM usage_logs WHERE trace_id = ?`, row.trace).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != row.want {
			t.Fatalf("%s status = %d, want %d", row.trace, got, row.want)
		}
	}
}
