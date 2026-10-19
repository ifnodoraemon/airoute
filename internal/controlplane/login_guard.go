package controlplane

import (
	"sync"
	"time"
)

// loginAttempt tracks failed password attempts for one account/IP pair.
type loginAttempt struct {
	mu          sync.Mutex
	failures    int
	lockedUntil time.Time
	lastFailure time.Time
}

const (
	maxLoginFailures   = 5
	loginLockoutWindow = 15 * time.Minute
)

// loginGuard throttles password guessing per username and per client IP.
type loginGuard struct {
	mu      sync.Mutex
	byUser  map[string]*loginAttempt
	byIP    map[string]*loginAttempt
	cleanup time.Time
}

var globalLoginGuard = &loginGuard{
	byUser: make(map[string]*loginAttempt),
	byIP:   make(map[string]*loginAttempt),
}

func (g *loginGuard) get(m map[string]*loginAttempt, key string) *loginAttempt {
	a, ok := m[key]
	if !ok {
		a = &loginAttempt{}
		m[key] = a
	}
	return a
}

// check reports whether the login attempt may proceed and how long the caller
// must wait when it is throttled.
func (g *loginGuard) check(username, ip string) (bool, time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.maybeCleanupLocked()

	now := time.Now()
	var retry time.Duration
	blocked := false
	for _, pair := range []struct {
		m   map[string]*loginAttempt
		key string
	}{
		{g.byUser, username},
		{g.byIP, ip},
	} {
		if a, ok := pair.m[pair.key]; ok {
			a.mu.Lock()
			if now.Before(a.lockedUntil) {
				if d := a.lockedUntil.Sub(now); d > retry {
					retry = d
				}
				blocked = true
			}
			a.mu.Unlock()
		}
	}
	return !blocked, retry
}

// failure records a failed attempt and locks the pair once the budget is spent.
func (g *loginGuard) failure(username, ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	record := func(m map[string]*loginAttempt, key string) {
		a := g.get(m, key)
		a.mu.Lock()
		defer a.mu.Unlock()
		// Reset the counter if the previous failure is stale.
		if !a.lastFailure.IsZero() && now.Sub(a.lastFailure) > loginLockoutWindow {
			a.failures = 0
		}
		a.failures++
		a.lastFailure = now
		if a.failures >= maxLoginFailures {
			a.lockedUntil = now.Add(loginLockoutWindow)
			a.failures = 0
		}
	}
	record(g.byUser, username)
	record(g.byIP, ip)
}

// success clears the failure counters after a valid login.
func (g *loginGuard) success(username, ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.byUser, username)
	delete(g.byIP, ip)
}

// maybeCleanupLocked drops entries that have been idle for a while.
func (g *loginGuard) maybeCleanupLocked() {
	now := time.Now()
	if now.Sub(g.cleanup) < 5*time.Minute {
		return
	}
	g.cleanup = now
	for _, m := range []map[string]*loginAttempt{g.byUser, g.byIP} {
		for k, a := range m {
			a.mu.Lock()
			idle := now.After(a.lockedUntil) && now.Sub(a.lastFailure) > loginLockoutWindow
			a.mu.Unlock()
			if idle {
				delete(m, k)
			}
		}
	}
}
