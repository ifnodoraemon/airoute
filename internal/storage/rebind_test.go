package storage

import "testing"

func TestRebindPlaceholders(t *testing.T) {
	pg := &DB{dialect: "postgres"}
	sqlite := &DB{dialect: "sqlite"}

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"two placeholders", "SELECT * FROM t WHERE a = ? AND b = ?", "SELECT * FROM t WHERE a = $1 AND b = $2"},
		{"placeholder in string literal untouched", "SELECT '?' AS q, a FROM t WHERE a = ?", "SELECT '?' AS q, a FROM t WHERE a = $1"},
		{"like becomes ilike", "SELECT * FROM t WHERE name LIKE ?", "SELECT * FROM t WHERE name ILIKE $1"},
		{"literal like untouched", "SELECT * FROM t WHERE name = 'LIKE'", "SELECT * FROM t WHERE name = 'LIKE'"},
		{"insert or ignore", "INSERT OR IGNORE INTO t (a) VALUES (?)", "INSERT INTO t (a) VALUES ($1) ON CONFLICT DO NOTHING"},
		{"existing on conflict kept", "INSERT OR IGNORE INTO t (a) VALUES (?) ON CONFLICT DO NOTHING", "INSERT INTO t (a) VALUES ($1) ON CONFLICT DO NOTHING"},
	}

	for _, tc := range cases {
		got := pg.Rebind(tc.in)
		if got != tc.want {
			t.Errorf("%s: Rebind(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
		// SQLite must be left completely untouched.
		if sqlite.Rebind(tc.in) != tc.in {
			t.Errorf("%s: sqlite Rebind modified the query", tc.name)
		}
	}
}

func TestWithReturningID(t *testing.T) {
	cases := map[string]string{
		"INSERT INTO t (a) VALUES (?)":               "INSERT INTO t (a) VALUES (?) RETURNING id",
		"INSERT INTO t (a) VALUES (?);":              "INSERT INTO t (a) VALUES (?) RETURNING id",
		"INSERT INTO t (a) VALUES (?) RETURNING id":  "INSERT INTO t (a) VALUES (?) RETURNING id",
		"INSERT INTO t (a) VALUES (?) RETURNING id;": "INSERT INTO t (a) VALUES (?) RETURNING id",
	}
	for in, want := range cases {
		if got := withReturningID(in); got != want {
			t.Errorf("withReturningID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRewriteLikeToILike(t *testing.T) {
	cases := map[string]string{
		"a LIKE b":     "a ILIKE b",
		"a NOT LIKE b": "a NOT ILIKE b",
		"a like b":     "a ILIKE b",
		"x = 'LIKE'":   "x = 'LIKE'",
		"a LIKELY b":   "a LIKELY b",
	}
	for in, want := range cases {
		if got := rewriteLikeToILike(in); got != want {
			t.Errorf("rewriteLikeToILike(%q) = %q, want %q", in, got, want)
		}
	}
}
