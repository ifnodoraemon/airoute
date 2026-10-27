package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func newRetentionRepo(t *testing.T) *Repository {
	t.Helper()
	db, err := OpenDB(filepath.Join(t.TempDir(), "retention.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewRepository(db)
}

func indexExists(t *testing.T, repo *Repository, name string) bool {
	t.Helper()
	var count int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&count); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	return count == 1
}

// Regression for the operational indexes (ST-09 verification_codes, ST-15
// request_traces, ST-16 video_tasks): migration 28 must create them.
func TestOperationalIndexesExist(t *testing.T) {
	repo := newRetentionRepo(t)
	for _, name := range []string{
		"idx_verify_lookup",
		"idx_verify_expires",
		"idx_trace_api_key",
		"idx_video_tasks_owner",
	} {
		if !indexExists(t, repo, name) {
			t.Errorf("index %q was not created by migration 28", name)
		}
	}
}

// Regression: verification codes were never deleted, so the table grew by one row
// per verification mail forever.
func TestVerificationCodeRetentionPrunesExpiredRows(t *testing.T) {
	repo := newRetentionRepo(t)
	now := time.Now().UTC()

	// An expired code well past the retention window, and one that is still valid.
	if _, err := repo.db.Exec(`INSERT INTO verification_codes (email, code, purpose, expires_at, used) VALUES (?, ?, ?, ?, 0)`,
		"old@example.com", "111111", "register", now.Add(-30*24*time.Hour).Format("2006-01-02 15:04:05")); err != nil {
		t.Fatalf("insert old code: %v", err)
	}
	if err := repo.SaveVerificationCode("fresh@example.com", "222222", "register", 10*time.Minute); err != nil {
		t.Fatalf("save fresh code: %v", err)
	}

	// SaveVerificationCode prunes opportunistically (first call per repository).
	var rows int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM verification_codes WHERE email = 'old@example.com'`).Scan(&rows); err != nil {
		t.Fatalf("count old codes: %v", err)
	}
	if rows != 0 {
		t.Fatalf("expired verification code was not pruned, %d row(s) left", rows)
	}

	// The still-valid code must survive and still verify.
	if ok := repo.VerifyCode("fresh@example.com", "222222", "register"); !ok {
		t.Fatal("retention pruning removed a still-valid code")
	}

	// The explicit purge honours its cutoff too (expired 30d ago vs cutoff 8d ago).
	if _, err := repo.db.Exec(`INSERT INTO verification_codes (email, code, purpose, expires_at, used) VALUES (?, ?, ?, ?, 0)`,
		"boundary@example.com", "333333", "register", now.Add(-10*24*time.Hour).Format("2006-01-02 15:04:05")); err != nil {
		t.Fatalf("insert boundary code: %v", err)
	}
	deleted, err := repo.PurgeVerificationCodesBefore(now.Add(-8 * 24 * time.Hour))
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("purge deleted %d rows, want 1", deleted)
	}
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM verification_codes WHERE email = 'boundary@example.com'`).Scan(&rows); err != nil {
		t.Fatalf("count boundary codes: %v", err)
	}
	if rows != 0 {
		t.Fatal("purge kept a code older than the cutoff")
	}
}

// Regression: video task rows were never deleted.
func TestVideoTaskRetentionPurgesOldRows(t *testing.T) {
	repo := newRetentionRepo(t)

	// A row older than the retention window, inserted directly because the write
	// path stamps created_at with the current time.
	if _, err := repo.db.Exec(`INSERT INTO video_tasks (id, upstream_id, owner, model, channel_id, channel_name, base_url, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"old-task", "up-old", "alice", "m", 1, "ch", "http://upstream.invalid", time.Now().UTC().AddDate(0, 0, -90).Format("2006-01-02 15:04:05")); err != nil {
		t.Fatalf("insert old task: %v", err)
	}

	deleted, err := repo.PurgeVideoTasksBefore(time.Now().UTC().AddDate(0, 0, -30))
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("purge deleted %d rows, want 1", deleted)
	}
	if _, err := repo.GetVideoTask(t.Context(), "old-task", "alice"); err == nil {
		t.Fatal("old video task survived the retention purge")
	}

	// A fresh task must survive.
	task := &VideoTask{ID: "fresh-task", UpstreamID: "up-new", Owner: "alice", Model: "m", ChannelID: 1, ChannelName: "ch", BaseURL: "http://upstream.invalid"}
	if err := repo.SaveVideoTask(t.Context(), task); err != nil {
		t.Fatalf("save fresh task: %v", err)
	}
	if _, err := repo.PurgeVideoTasksBefore(time.Now().UTC().AddDate(0, 0, -30)); err != nil {
		t.Fatalf("second purge: %v", err)
	}
	if _, err := repo.GetVideoTask(t.Context(), "fresh-task", "alice"); err != nil {
		t.Fatalf("fresh video task was pruned: %v", err)
	}
}

// Regression (ST-16): the persisted upstream base URL must not carry inline
// credentials, while routing-relevant parts survive.
func TestVideoTaskBaseURLCredentialsAreNotPersisted(t *testing.T) {
	repo := newRetentionRepo(t)

	cases := []struct {
		name       string
		in         string
		wantInURL  []string
		wantNotURL []string
	}{
		{
			name:       "userinfo and credential query parameters",
			in:         "https://user:s3cret@video.example.com/v1?api_key=sk-live-123&region=cn&token=abc",
			wantInURL:  []string{"video.example.com", "/v1", "region=cn"},
			wantNotURL: []string{"s3cret", "sk-live-123", "token=abc", "user:"},
		},
		{
			name:       "bearer token in query",
			in:         "http://10.0.0.5:9000/api?access_token=xyz&model=video-1",
			wantInURL:  []string{"10.0.0.5:9000", "model=video-1"},
			wantNotURL: []string{"xyz"},
		},
		{
			name:       "no credentials is a no-op",
			in:         "http://upstream.invalid/v1",
			wantInURL:  []string{"http://upstream.invalid/v1"},
			wantNotURL: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			task := &VideoTask{ID: "task-" + tc.name, UpstreamID: "up", Owner: "bob", Model: "m", ChannelID: 1, ChannelName: "ch", BaseURL: tc.in}
			if err := repo.SaveVideoTask(t.Context(), task); err != nil {
				t.Fatalf("save: %v", err)
			}
			got, err := repo.GetVideoTask(t.Context(), task.ID, "bob")
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			for _, want := range tc.wantInURL {
				if !contains(got.BaseURL, want) {
					t.Fatalf("stored URL %q lost %q", got.BaseURL, want)
				}
			}
			for _, unwanted := range tc.wantNotURL {
				if contains(got.BaseURL, unwanted) {
					t.Fatalf("stored URL %q still contains %q", got.BaseURL, unwanted)
				}
			}
		})
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// Regression (ST-16): saving a task with a clean URL must not rewrite it.
func TestSanitizeVideoTaskBaseURLKeepsCleanURLs(t *testing.T) {
	for _, raw := range []string{
		"http://upstream.invalid/v1",
		"https://video.example.com:8443/api/v1?region=cn",
	} {
		got, changed := sanitizeVideoTaskBaseURL(raw)
		if changed {
			t.Errorf("clean URL %q reported as modified", raw)
		}
		if got != raw {
			t.Errorf("clean URL %q rewritten to %q", raw, got)
		}
	}
}
