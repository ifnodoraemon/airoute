package api

import (
	"strings"
	"testing"
)

func TestValidateMCPReadQueryAllowsAllowlistedSelects(t *testing.T) {
	allowed := []string{
		"SELECT * FROM usage_logs LIMIT 10",
		"select model, count(*) from usage_logs group by model",
		"SELECT * FROM model_prices",
		"SELECT * FROM model_fallbacks;",
		"EXPLAIN SELECT * FROM usage_logs",
		"WITH recent AS (SELECT * FROM usage_logs) SELECT * FROM recent",
		"SELECT u.model FROM usage_logs u JOIN model_prices p ON p.model = u.model",
	}
	for _, q := range allowed {
		if _, err := validateMCPReadQuery(q); err != nil {
			t.Fatalf("expected %q to be allowed, got error: %v", q, err)
		}
	}
}

func TestValidateMCPReadQueryRejectsSensitiveTables(t *testing.T) {
	denied := []string{
		"SELECT api_key FROM channels",
		"SELECT * FROM api_keys",
		"SELECT username, password_hash FROM users",
		"SELECT code FROM redemption_codes",
		"SELECT * FROM verification_codes",
		"SELECT * FROM recharge_orders",
		"SELECT * FROM sqlite_master",
		"SELECT tablename FROM pg_tables",
		"SELECT * FROM information_schema.columns",
		// Comma join: the second table never appears after FROM/JOIN.
		"SELECT * FROM usage_logs, channels",
		// Case and quoting variants.
		"select * from CHANNELS",
		`SELECT * FROM "channels"`,
		// A CTE may not be used to reintroduce a sensitive table name.
		"WITH channels AS (SELECT * FROM usage_logs) SELECT * FROM channels",
	}
	for _, q := range denied {
		if _, err := validateMCPReadQuery(q); err == nil {
			t.Fatalf("expected %q to be rejected", q)
		}
	}
}

func TestValidateMCPReadQueryRejectsBypassAttempts(t *testing.T) {
	denied := []string{
		"DELETE\tFROM usage_logs",
		"DELETE\nFROM usage_logs",
		"/* harmless */ DELETE FROM usage_logs",
		"UPDATE usage_logs SET model = 'x'",
		"INSERT INTO usage_logs (model) VALUES ('x')",
		"WITH x AS (DELETE FROM usage_logs RETURNING *) SELECT * FROM x",
		"SELECT 1; DROP TABLE usage_logs",
		"SELECT * FROM usage_logs; DELETE FROM usage_logs",
		"PRAGMA table_info(users)",
		"ATTACH DATABASE '/tmp/x.db' AS x",
		"SELECT * FROM usage_logs -- trailing\n; DROP TABLE usage_logs",
		"SELECT * FROM not_a_real_table",
		"SELECT 1",
		"",
	}
	for _, q := range denied {
		if _, err := validateMCPReadQuery(q); err == nil {
			t.Fatalf("expected %q to be rejected", q)
		}
	}
}

func TestValidateMCPReadQueryRejectsOverlongQuery(t *testing.T) {
	q := "SELECT * FROM usage_logs WHERE model = '" + strings.Repeat("a", 9000) + "'"
	if _, err := validateMCPReadQuery(q); err == nil {
		t.Fatal("expected an overlong query to be rejected")
	}
}
