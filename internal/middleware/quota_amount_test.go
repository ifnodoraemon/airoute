package middleware

import "testing"

func TestParseInflightAdmissionSettings(t *testing.T) {
	if got := parseInflightReserveCNY(""); got != DefaultInflightReserveCNY {
		t.Fatalf("empty reserve = %v, want %v", got, DefaultInflightReserveCNY)
	}
	if got := parseInflightReserveCNY("0.2"); got != 0.2 {
		t.Fatalf("reserve = %v, want 0.2", got)
	}
	for _, invalid := range []string{"abc", "0", "-1", "NaN", "+Inf"} {
		if got := parseInflightReserveCNY(invalid); got != DefaultInflightReserveCNY {
			t.Fatalf("invalid reserve %q = %v, want default %v", invalid, got, DefaultInflightReserveCNY)
		}
	}

	if got := parseMaxInflightPerUser(""); got != DefaultMaxInflightPerUser {
		t.Fatalf("empty cap = %v, want %v", got, DefaultMaxInflightPerUser)
	}
	if got := parseMaxInflightPerUser("8"); got != 8 {
		t.Fatalf("cap = %v, want 8", got)
	}
	if got := parseMaxInflightPerUser("0"); got != 0 {
		t.Fatalf("cap 0 = %v, want 0 (uncapped)", got)
	}
	for _, invalid := range []string{"abc", "-3", "1.5"} {
		if got := parseMaxInflightPerUser(invalid); got != DefaultMaxInflightPerUser {
			t.Fatalf("invalid cap %q = %v, want default %v", invalid, got, DefaultMaxInflightPerUser)
		}
	}
}
