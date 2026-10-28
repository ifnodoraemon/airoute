package controlplane

import (
	"context"
	"fmt"
	"testing"
)

// B-02 regression suite: what a successful login may and may not unblock.
//
// The login guard throttles two dimensions: the account (5 failures) and, when the
// peer is a trusted proxy, the client address (20 failures, a cross-account
// spraying defence). A successful login proves the password of one account only,
// so it must clear that account's counter and nothing else.

// loginAttemptOnce models what the handler does for one attempt: throttle check
// first, then - for a wrong password - record a failure. It reports whether the
// attempt was admitted.
func loginAttemptOnce(g *loginGuard, username, ip string) bool {
	client := loginClient{Username: username, IP: ip, IPIsForwarded: true}
	ok, _ := g.check(context.Background(), client)
	if ok {
		g.failure(context.Background(), client)
	}
	return ok
}

// TestSuccessfulLoginDoesNotResetPerIPSprayBudget is the regression for the
// original hole: an attacker holding one valid account sprayed 19 passwords, logged
// into their own account to reset the address budget, and repeated forever.
func TestSuccessfulLoginDoesNotResetPerIPSprayBudget(t *testing.T) {
	ctx := context.Background()
	g := &loginGuard{local: newLocalLoginStore()}

	const attackerIP = "203.0.113.9"
	attacker := loginClient{Username: "attacker", IP: attackerIP, IPIsForwarded: true}

	// 19 sprayed failures against other accounts from one address.
	for i := 0; i < maxLoginFailuresPerIP-1; i++ {
		if !loginAttemptOnce(g, fmt.Sprintf("victim-%d", i), attackerIP) {
			t.Fatalf("attempt %d/%d should still be within the address budget", i+1, maxLoginFailuresPerIP)
		}
	}

	// The attacker logs into their own account. That resets the attacker's account
	// bucket (a valid credential was used), but must NOT touch the address budget.
	g.success(ctx, attacker)

	// The next attempt is the 20th failure for this address and must trip the
	// per-IP lockout.
	if ok, _ := g.check(ctx, loginClient{Username: "final-victim", IP: attackerIP, IPIsForwarded: true}); !ok {
		t.Fatalf("per-IP budget was already exhausted after %d failures (expected the 20th)", maxLoginFailuresPerIP-1)
	}
	loginAttemptOnce(g, "final-victim", attackerIP)

	// Now the address is locked: a fresh account from the same address is refused
	// even though the attacker's own login succeeded in between.
	if ok, retry := g.check(ctx, loginClient{Username: "another-victim", IP: attackerIP, IPIsForwarded: true}); ok {
		t.Fatalf("address budget was reset by a successful login of another account (retry=%s); "+
			"that is the B-02 hole: %d attempts from %s did not accumulate", retry, maxLoginFailuresPerIP, attackerIP)
	}
}

// TestSuccessfulLoginStillClearsItsOwnAccountBucket keeps the intended half of the
// old behaviour: a user who mistypes their password a few times and then succeeds
// must not stay one failure away from a lockout.
func TestSuccessfulLoginStillClearsItsOwnAccountBucket(t *testing.T) {
	ctx := context.Background()
	g := &loginGuard{local: newLocalLoginStore()}

	const ip = "198.51.100.7"
	for i := 0; i < maxLoginFailuresPerAccount-1; i++ {
		g.failure(ctx, loginClient{Username: "alice", IP: ip, IPIsForwarded: true})
	}
	if ok, _ := g.check(ctx, loginClient{Username: "alice", IP: ip, IPIsForwarded: true}); !ok {
		t.Fatalf("account bucket locked too early after %d failures", maxLoginFailuresPerAccount-1)
	}

	g.success(ctx, loginClient{Username: "alice", IP: ip, IPIsForwarded: true})

	// The account counter is back to zero: a full budget of failures is available
	// again for that account.
	for i := 0; i < maxLoginFailuresPerAccount-1; i++ {
		if ok, _ := g.check(ctx, loginClient{Username: "alice", IP: ip, IPIsForwarded: true}); !ok {
			t.Fatalf("account bucket was not cleared by a successful login (failure %d)", i+1)
		}
		g.failure(ctx, loginClient{Username: "alice", IP: ip, IPIsForwarded: true})
	}
}

// TestAccountLockoutStillApplies pins the account dimension independently: after
// maxLoginFailuresPerAccount failures for one username, that username is refused
// even from a different address (each address has its own budget).
func TestAccountLockoutStillApplies(t *testing.T) {
	ctx := context.Background()
	g := &loginGuard{local: newLocalLoginStore()}

	for i := 0; i < maxLoginFailuresPerAccount; i++ {
		g.failure(ctx, loginClient{Username: "bob", IP: "198.51.100.1", IPIsForwarded: true})
	}
	ok, retry := g.check(ctx, loginClient{Username: "bob", IP: "198.51.100.99", IPIsForwarded: true})
	if ok {
		t.Fatal("account lockout did not trigger after the failure budget was spent")
	}
	if retry <= 0 {
		t.Fatalf("locked account reported retry=%s, want a positive wait", retry)
	}
}
