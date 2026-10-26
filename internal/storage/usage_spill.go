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
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// The usage-log pipeline is the billing ledger: a record that never reaches the
// database is a charge the gateway paid upstream and never collected. Every
// in-memory and Redis path can fail (queue full, flush timeout, Redis outage),
// so the last resort is a local append-only spill file that is replayed on the
// next start.
//
// Records carry their billing_id, so replaying a file that another replica has
// already persisted (both replicas share the gateway-data volume) is a no-op
// rather than a double charge.
const (
	usageSpillPrefix = "usage_spill-"
	usageSpillSuffix = ".jsonl"

	defaultUsageSpillDir   = "data"
	defaultUsageSpillMaxMB = 256

	// Spill files are replayed in bounded batches with a deadline so a wedged
	// database cannot block startup forever.
	usageSpillReplayBatchSize = 100
	usageSpillReplayTimeout   = 10 * time.Second
)

// usageSpill writes billing records that could not be queued anywhere else.
type usageSpill struct {
	dir      string
	maxBytes int64
	path     string // this process's spill file
	mu       sync.Mutex
}

// newUsageSpill resolves the spill location from the environment.
// GATEWAY_USAGE_SPILL_DIR="-" disables disk spilling.
func newUsageSpill() *usageSpill {
	dir := strings.TrimSpace(os.Getenv("GATEWAY_USAGE_SPILL_DIR"))
	if dir == "" {
		dir = defaultUsageSpillDir
	}
	if dir == "-" || dir == "off" {
		return &usageSpill{}
	}
	maxMB := defaultUsageSpillMaxMB
	if raw := strings.TrimSpace(os.Getenv("GATEWAY_USAGE_SPILL_MAX_MB")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			maxMB = value
		}
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		telemetry.ScopedLogger("system").Error("usage log spill disabled: cannot create directory",
			"dir", dir, "error", err.Error())
		return &usageSpill{}
	}
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "gateway"
	}
	return &usageSpill{
		dir:      dir,
		maxBytes: int64(maxMB) * 1024 * 1024,
		path:     filepath.Join(dir, fmt.Sprintf("%s%s-%d-%d%s", usageSpillPrefix, hostname, os.Getpid(), time.Now().UnixNano(), usageSpillSuffix)),
	}
}

func (s *usageSpill) enabled() bool {
	return s != nil && s.dir != ""
}

// append persists records to disk. It fsyncs before returning: a record is either
// fully on disk or reported as a failure, so the caller can raise a critical
// alert instead of silently losing a charge.
func (s *usageSpill) append(records []*UsageLogRecord) (int, error) {
	if !s.enabled() || len(records) == 0 {
		return 0, fmt.Errorf("usage log spill is disabled")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if size, err := s.currentSizeLocked(); err == nil && size >= s.maxBytes {
		return 0, fmt.Errorf("usage log spill is full (%d bytes >= %d)", size, s.maxBytes)
	}

	file, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	written := 0
	writer := bufio.NewWriter(file)
	for _, record := range records {
		if record == nil {
			continue
		}
		encoded, err := json.Marshal(record)
		if err != nil {
			continue
		}
		if _, err := writer.Write(append(encoded, '\n')); err != nil {
			file.Close()
			return written, err
		}
		written++
	}
	if err := writer.Flush(); err != nil {
		file.Close()
		return written, err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return written, err
	}
	return written, file.Close()
}

// currentSizeLocked sums all spill files so one replica cannot fill the shared
// volume on its own.
func (s *usageSpill) currentSizeLocked() (int64, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), usageSpillPrefix) || !strings.HasSuffix(entry.Name(), usageSpillSuffix) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		total += info.Size()
	}
	return total, nil
}

func (s *usageSpill) replay(repo *Repository) (int, error) {
	if !s.enabled() || repo == nil {
		return 0, nil
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0, err
	}
	replayed := 0
	var firstErr error
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), usageSpillPrefix) || !strings.HasSuffix(entry.Name(), usageSpillSuffix) {
			continue
		}
		path := filepath.Join(s.dir, entry.Name())
		count, err := s.replayFile(repo, path)
		replayed += count
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("replay %s: %w", entry.Name(), err)
			}
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			telemetry.ScopedLogger("system").Warn("failed to remove replayed usage spill file",
				"file", entry.Name(), "error", err.Error())
		}
	}
	return replayed, firstErr
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
	for scanner.Scan() {
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
