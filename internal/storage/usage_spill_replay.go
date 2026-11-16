package storage

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// Replay/claim logic for the usage spill file, extracted from usage_spill.go to
// keep the append-path file under the size red line.

func (s *usageSpill) replay(repo *Repository) (int, error) {
	if !s.enabled() || repo == nil {
		return 0, nil
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0, err
	}
	now := time.Now()
	replayed := 0
	deferred := 0
	var firstErr error
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), usageSpillPrefix) || !strings.HasSuffix(entry.Name(), usageSpillSuffix) {
			continue
		}
		path := filepath.Join(s.dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if !s.claimable(info, path, now) {
			// Another replica is still writing this file. Consuming it would delete
			// records it is about to add, so leave it for a later pass (it becomes
			// claimable once it goes quiet).
			deferred++
			continue
		}
		claimed, err := s.claimSpillFile(path)
		if err != nil {
			// Somebody else claimed it first (rename is atomic): skip it.
			if firstErr == nil && !os.IsNotExist(err) {
				firstErr = fmt.Errorf("claim %s: %w", entry.Name(), err)
			}
			continue
		}
		count, err := s.replayFile(repo, claimed)
		replayed += count
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("replay %s: %w", entry.Name(), err)
			}
			// Put the records back under their original (scanned) name so a later
			// startup retries them instead of stranding a claimed file nobody looks
			// at. The claim itself is a spill-shaped name, so a crash here is still
			// picked up by the next pass.
			if restoreErr := os.Rename(claimed, path); restoreErr != nil {
				telemetry.ScopedLogger("system").Warn("failed to restore an unreplayed usage spill file",
					"file", filepath.Base(claimed), "error", restoreErr.Error())
			}
			continue
		}
		if err := os.Remove(claimed); err != nil && !os.IsNotExist(err) {
			telemetry.ScopedLogger("system").Warn("failed to remove replayed usage spill file",
				"file", filepath.Base(claimed), "error", err.Error())
		}
	}
	if deferred > 0 {
		// The records are safe but not yet persisted; say so instead of finishing
		// the replay silently with work left behind.
		telemetry.WarnLogger(telemetry.ScopedLogger("system"), "usage-spill-replay-deferred", 30*time.Second,
			"usage spill files are still being written by a live replica; replay deferred until they go quiet",
			"deferred_files", deferred,
			"grace_sec", int(usageSpillClaimGrace().Seconds()))
	}
	return replayed, firstErr
}

// claimable reports whether this process may take ownership of a spill file.
//
// A spill file is per process (its name carries the hostname and pid) and the
// volume is shared by every replica, so a starting replica used to replay and
// delete its peers' files: records a peer appended after the read were lost and
// its later appends went to a recreated empty file. Only a file that is quiet
// (no write for the grace window) or that is our own is claimed here.
func (s *usageSpill) claimable(info os.FileInfo, path string, now time.Time) bool {
	if path == s.path {
		return true
	}
	if now.Sub(info.ModTime()) >= usageSpillClaimGrace() {
		return true
	}
	// Own hostname: another process of ours is alive and writing (a restart in
	// place), so wait for it to go quiet instead of stealing the file.
	return false
}

// claimSpillFile atomically takes a spill file out of the shared directory.
//
// The rename is what makes the claim safe: the owner opens its file by path on
// every append, so after the rename its next append creates a fresh file while
// this process replays a private copy. Reading and then removing the shared path
// would instead lose every record appended in between.
func (s *usageSpill) claimSpillFile(path string) (string, error) {
	// The claimed name keeps the spill prefix/suffix on purpose: if this process
	// dies mid-replay, the next startup still scans it instead of leaving an
	// invisible orphan file behind.
	claimed := filepath.Join(s.dir, fmt.Sprintf("%sclaimed-%d-%d%s",
		usageSpillPrefix, os.Getpid(), time.Now().UnixNano(), usageSpillSuffix))
	if err := os.Rename(path, claimed); err != nil {
		return "", err
	}
	return claimed, nil
}

// usageSpillClaimGrace is how long a spill file must be untouched before another
// replica may claim it. GATEWAY_USAGE_SPILL_CLAIM_GRACE_SEC overrides it.
func usageSpillClaimGrace() time.Duration {
	const defaultGrace = 10 * time.Minute
	raw := strings.TrimSpace(os.Getenv("GATEWAY_USAGE_SPILL_CLAIM_GRACE_SEC"))
	if raw == "" {
		return defaultGrace
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 0 {
		return defaultGrace
	}
	return time.Duration(seconds) * time.Second
}

func (s *usageSpill) replayFile(repo *Repository, path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	var batch []*UsageLogRecord
	replayed := 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), usageSpillReplayTimeout)
		err := repo.BatchRecordUsageLogsContext(ctx, batch)
		cancel()
		if err != nil {
			return err
		}
		replayed += len(batch)
		batch = batch[:0]
		return nil
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record UsageLogRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			// A corrupt line must not block the rest of the file; the charge it
			// described is reported through the counter instead.
			telemetry.GlobalMetrics.UsageLogSpillReplayFailures.Add(1)
			telemetry.ScopedLogger("system").Error("unreadable usage spill record",
				"file", filepath.Base(path), "error", err.Error())
			continue
		}
		if record.BillingID == "" {
			// Line written by a binary that predates billing_id, or a file that is
			// re-read after a partial replay. The position is stable across
			// attempts (the file is only removed after a complete replay), so the
			// already-persisted prefix is not charged twice.
			record.BillingID = BillingIDFromSpillLine(filepath.Base(path), lineNumber)
		}
		batch = append(batch, &record)
		if len(batch) >= usageSpillReplayBatchSize {
			if err := flush(); err != nil {
				return replayed, err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return replayed, err
	}
	if err := flush(); err != nil {
		return replayed, err
	}
	return replayed, nil
}