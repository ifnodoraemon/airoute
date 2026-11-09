package middleware

import "testing"

func TestShouldFailClosedOnUnknownAccount(t *testing.T) {
	cases := []struct {
		name       string
		freeRead   bool
		userStatus string
		userID     int64
		want       bool
	}{
		{"known active user is admitted", false, "active", 7, false},
		{"known locked user is decided elsewhere", false, "locked", 7, false},
		{"unknown status with an owner fails closed", false, "", 7, true},
		{"key without an owner keeps open/dev-mode behaviour", false, "", 0, false},
		{"free read is never blocked by unknown state", true, "", 7, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldFailClosedOnUnknownAccount(tc.freeRead, tc.userStatus, tc.userID); got != tc.want {
				t.Fatalf("shouldFailClosedOnUnknownAccount(%v, %q, %d) = %v, want %v",
					tc.freeRead, tc.userStatus, tc.userID, got, tc.want)
			}
		})
	}
}

// TestBalanceUnknownDenialIsRetryableDocumentedContract pins the response shape:
// a 503 with Retry-After, because a database stall is transient by definition.
func TestBalanceUnknownDenial(t *testing.T) {
	denial := BalanceUnknownDenial()
	if denial == nil {
		t.Fatal("nil denial")
	}
	if denial.StatusCode != 503 {
		t.Fatalf("status = %d, want 503", denial.StatusCode)
	}
	if denial.Code != "account_state_unavailable" {
		t.Fatalf("code = %q", denial.Code)
	}
	if denial.Headers["Retry-After"] == "" {
		t.Fatal("missing Retry-After header: a fail-closed denial must tell the client to retry")
	}
}
