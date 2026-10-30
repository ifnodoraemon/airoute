package controlplane

import (
	"context"
	"errors"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// errNoRows mirrors what the storage layer returns for a missing row: an error,
// not (nil, nil).
var errNoRows = errors.New("sql: no rows in result set")

// --- fakes ---------------------------------------------------------------

type fakeChannelStore struct {
	channels map[int64]*storage.ChannelRecord
	nextID   int64

	listErr        error
	getErr         error
	createErr      error
	updateErr      error
	deleteErr      error
	batchDeleteErr error
	batchStatusErr error

	createCalls      int
	updateCalls      int
	deleteCalls      int
	batchDeleteCalls int
	batchStatusCalls int
	lastBatchIDs     []int64
	lastBatchStatus  string

	// afterGet runs after GetChannel has produced its answer. It is how a
	// concurrent writer is simulated landing between a caller's read and its
	// write-back.
	afterGet func()
}

func newFakeChannelStore() *fakeChannelStore {
	return &fakeChannelStore{channels: map[int64]*storage.ChannelRecord{}, nextID: 1}
}

func (f *fakeChannelStore) seed(rec *storage.ChannelRecord) *storage.ChannelRecord {
	if rec.ID == 0 {
		rec.ID = f.nextID
		f.nextID++
	}
	cp := *rec
	f.channels[rec.ID] = &cp
	return &cp
}

func (f *fakeChannelStore) ListChannels() ([]*storage.ChannelRecord, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]*storage.ChannelRecord, 0, len(f.channels))
	for _, c := range f.channels {
		out = append(out, c)
	}
	return out, nil
}

// GetChannel hands out a detached copy, like a real store: the caller's later
// mutation of the returned record must not be visible in storage until it is
// written back.
func (f *fakeChannelStore) GetChannel(id int64) (*storage.ChannelRecord, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	c, ok := f.channels[id]
	if !ok {
		return nil, errNoRows
	}
	cp := *c
	if f.afterGet != nil {
		f.afterGet()
	}
	return &cp, nil
}

// PatchChannel mirrors the storage layer: the mutation runs against the row as
// it is at patch time, in one step, so a concurrent write cannot be lost.
func (f *fakeChannelStore) PatchChannel(id int64, fn func(*storage.ChannelRecord) error) (*storage.ChannelRecord, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	c, ok := f.channels[id]
	if !ok {
		return nil, storage.ErrChannelNotFound
	}
	if f.afterGet != nil {
		f.afterGet()
	}
	cp := *c
	if err := fn(&cp); err != nil {
		return nil, err
	}
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	f.updateCalls++
	f.channels[id] = &cp
	return &cp, nil
}

func (f *fakeChannelStore) CreateChannel(rec *storage.ChannelRecord) error {
	f.createCalls++
	if f.createErr != nil {
		return f.createErr
	}
	rec.ID = f.nextID
	f.nextID++
	cp := *rec
	f.channels[rec.ID] = &cp
	return nil
}

func (f *fakeChannelStore) UpdateChannel(rec *storage.ChannelRecord) error {
	f.updateCalls++
	if f.updateErr != nil {
		return f.updateErr
	}
	cp := *rec
	f.channels[rec.ID] = &cp
	return nil
}

func (f *fakeChannelStore) DeleteChannel(id int64) error {
	f.deleteCalls++
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.channels, id)
	return nil
}

func (f *fakeChannelStore) BatchDeleteChannels(ids []int64) (int64, error) {
	f.batchDeleteCalls++
	f.lastBatchIDs = ids
	if f.batchDeleteErr != nil {
		return 0, f.batchDeleteErr
	}
	var n int64
	for _, id := range ids {
		if _, ok := f.channels[id]; ok {
			delete(f.channels, id)
			n++
		}
	}
	return n, nil
}

func (f *fakeChannelStore) BatchUpdateChannelStatus(ids []int64, status string) (int64, error) {
	f.batchStatusCalls++
	f.lastBatchIDs = ids
	f.lastBatchStatus = status
	if f.batchStatusErr != nil {
		return 0, f.batchStatusErr
	}
	var n int64
	for _, id := range ids {
		if c, ok := f.channels[id]; ok {
			c.Status = status
			n++
		}
	}
	return n, nil
}

type fakeReloader struct {
	calls   int
	reasons []string
	err     error
}

func (f *fakeReloader) ReloadAndBroadcast(_ context.Context, reason string) error {
	f.calls++
	f.reasons = append(f.reasons, reason)
	return f.err
}

type fakeBreakers struct {
	statuses map[string]string
	calls    int
}

func (f *fakeBreakers) GetBreakerStatus(name string) string {
	f.calls++
	if s, ok := f.statuses[name]; ok {
		return s
	}
	return "CLOSED"
}

func validChannel() *storage.ChannelRecord {
	return &storage.ChannelRecord{
		Name:     "chan-a",
		Type:     "openai",
		BaseURL:  "http://a.invalid",
		APIKey:   "secret",
		Models:   []string{"m1"},
		Priority: 3,
		Weight:   7,
		Status:   "active",
	}
}

// --- create --------------------------------------------------------------

func TestChannelServiceCreateRejectsMissingRequiredFields(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*storage.ChannelRecord)
	}{
		{"missing name", func(c *storage.ChannelRecord) { c.Name = "" }},
		{"missing type", func(c *storage.ChannelRecord) { c.Type = "" }},
		{"missing base url", func(c *storage.ChannelRecord) { c.BaseURL = "" }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeChannelStore()
			reloader := &fakeReloader{}
			svc := NewChannelService(store, reloader, nil)

			rec := validChannel()
			tc.mutate(rec)

			_, err := svc.Create(context.Background(), rec)

			var verr *ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("err = %v, want *ValidationError", err)
			}
			if store.createCalls != 0 {
				t.Errorf("invalid input reached the store: createCalls = %d", store.createCalls)
			}
			if reloader.calls != 0 {
				t.Errorf("invalid input triggered a data plane reload: calls = %d", reloader.calls)
			}
		})
	}
}

func TestChannelServiceCreatePersistsAndReloads(t *testing.T) {
	store := newFakeChannelStore()
	reloader := &fakeReloader{}
	svc := NewChannelService(store, reloader, nil)

	rec, err := svc.Create(context.Background(), validChannel())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rec.ID == 0 {
		t.Error("Create did not surface the assigned id")
	}
	if store.createCalls != 1 {
		t.Errorf("createCalls = %d, want 1", store.createCalls)
	}
	if reloader.calls != 1 || reloader.reasons[0] != "channel_created" {
		t.Errorf("reload calls = %v, want one \"channel_created\"", reloader.reasons)
	}
}

func TestChannelServiceCreatePropagatesStoreErrorWithoutReloading(t *testing.T) {
	store := newFakeChannelStore()
	store.createErr = errors.New("disk on fire")
	reloader := &fakeReloader{}
	svc := NewChannelService(store, reloader, nil)

	_, err := svc.Create(context.Background(), validChannel())

	if err == nil || err.Error() != "disk on fire" {
		t.Fatalf("err = %v, want the store error", err)
	}
	if reloader.calls != 0 {
		t.Errorf("a failed write still reloaded the data plane: calls = %d", reloader.calls)
	}
}

// --- list ----------------------------------------------------------------

func TestChannelServiceListDecoratesBreakerStatus(t *testing.T) {
	store := newFakeChannelStore()
	store.seed(&storage.ChannelRecord{Name: "chan-a"})
	store.seed(&storage.ChannelRecord{Name: "chan-b"})
	breakers := &fakeBreakers{statuses: map[string]string{"chan-a": "OPEN", "chan-b": "CLOSED"}}
	svc := NewChannelService(store, nil, breakers)

	list, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
	for _, ch := range list {
		if ch.BreakerStatus != breakers.statuses[ch.Name] {
			t.Errorf("%s breaker = %q, want %q", ch.Name, ch.BreakerStatus, breakers.statuses[ch.Name])
		}
	}
}

func TestChannelServiceListPropagatesStoreError(t *testing.T) {
	store := newFakeChannelStore()
	store.listErr = errors.New("db down")
	svc := NewChannelService(store, nil, nil)

	if _, err := svc.List(); err == nil {
		t.Fatal("List swallowed the store error")
	}
}

// --- update --------------------------------------------------------------

func TestChannelServiceUpdateMergesOnlyProvidedFields(t *testing.T) {
	store := newFakeChannelStore()
	existing := store.seed(&storage.ChannelRecord{
		Name: "chan-a", Type: "openai", BaseURL: "http://a.invalid",
		APIKey: "secret", Models: []string{"m1"}, Priority: 3, Weight: 7,
		TimeoutSeconds: 60, Status: "active",
	})
	reloader := &fakeReloader{}
	svc := NewChannelService(store, reloader, nil)

	// A partial update that only changes the display name.
	got, err := svc.Update(context.Background(), existing.ID, &channelPatch{Name: "renamed"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if got.Name != "renamed" {
		t.Errorf("Name = %q, want renamed", got.Name)
	}
	// Everything the caller omitted must survive.
	if got.APIKey != "secret" {
		t.Errorf("omitted api_key was wiped: %q", got.APIKey)
	}
	if got.BaseURL != "http://a.invalid" {
		t.Errorf("omitted base_url was wiped: %q", got.BaseURL)
	}
	if got.Priority != 3 || got.Weight != 7 || got.TimeoutSeconds != 60 {
		t.Errorf("omitted routing fields were reset: priority=%d weight=%d timeout=%d",
			got.Priority, got.Weight, got.TimeoutSeconds)
	}
	if len(got.Models) != 1 || got.Models[0] != "m1" {
		t.Errorf("omitted models were wiped: %v", got.Models)
	}
	if reloader.calls != 1 || reloader.reasons[0] != "channel_updated" {
		t.Errorf("reload reasons = %v, want one \"channel_updated\"", reloader.reasons)
	}
}

func TestChannelServiceUpdateAppliesExplicitZeroRoutingFields(t *testing.T) {
	// Pins the provided-vs-omitted contract: an explicit 0 is a value the caller
	// asked for, so it must reach storage instead of being read as "not provided".
	// The old merge tested `!= 0`, so a request that set priority/weight to 0
	// returned 200 while changing nothing.
	store := newFakeChannelStore()
	existing := store.seed(&storage.ChannelRecord{
		Name: "chan-a", Type: "openai", BaseURL: "http://a.invalid",
		Priority: 3, Weight: 7, TimeoutSeconds: 60, Status: "active",
	})
	svc := NewChannelService(store, nil, nil)

	zero, zeroTimeout := 0, 0
	got, err := svc.Update(context.Background(), existing.ID, &channelPatch{
		Priority: &zero, Weight: &zero, TimeoutSeconds: &zeroTimeout,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	// TimeoutSeconds is normalized by the service (0 = use the default budget), so
	// only the two routing fields keep the explicit zero here.
	if got.Priority != 0 || got.Weight != 0 {
		t.Errorf("explicit zeros were dropped: priority=%d weight=%d, want 0/0",
			got.Priority, got.Weight)
	}
	if got.TimeoutSeconds == 0 {
		t.Errorf("timeout_seconds = 0 after normalization, want the default budget")
	}
}

func TestChannelServiceUpdateKeepsOmittedRoutingFields(t *testing.T) {
	store := newFakeChannelStore()
	existing := store.seed(&storage.ChannelRecord{
		Name: "chan-a", Type: "openai", BaseURL: "http://a.invalid",
		Priority: 3, Weight: 7, TimeoutSeconds: 60, Status: "active",
	})
	svc := NewChannelService(store, nil, nil)

	got, err := svc.Update(context.Background(), existing.ID, &channelPatch{Name: "renamed"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Priority != 3 || got.Weight != 7 || got.TimeoutSeconds != 60 {
		t.Errorf("omitted routing fields were reset: priority=%d weight=%d timeout=%d",
			got.Priority, got.Weight, got.TimeoutSeconds)
	}
}

func TestChannelServiceUpdateRejectsUnknownChannel(t *testing.T) {
	store := newFakeChannelStore()
	svc := NewChannelService(store, nil, nil)

	_, err := svc.Update(context.Background(), 4242, &channelPatch{Name: "x"})

	if !errors.Is(err, ErrChannelNotFound) {
		t.Fatalf("err = %v, want ErrChannelNotFound", err)
	}
	if store.updateCalls != 0 {
		t.Errorf("update ran against a missing channel: calls = %d", store.updateCalls)
	}
}

func TestChannelServiceUpdatePropagatesReadFailure(t *testing.T) {
	// A read failure is not "not found". Reporting it as 404 made a database
	// outage look like a deleted channel to the operator.
	store := newFakeChannelStore()
	store.getErr = errors.New("connection reset")
	svc := NewChannelService(store, nil, nil)

	_, err := svc.Update(context.Background(), 1, &channelPatch{Name: "x"})

	if errors.Is(err, ErrChannelNotFound) {
		t.Fatal("a read failure was reported as a missing channel")
	}
	if err == nil || err.Error() != "connection reset" {
		t.Fatalf("err = %v, want the read error", err)
	}
}

func TestChannelServiceUpdatePropagatesStoreErrorWithoutReloading(t *testing.T) {
	store := newFakeChannelStore()
	existing := store.seed(&storage.ChannelRecord{Name: "chan-a", Type: "openai", BaseURL: "http://a.invalid"})
	store.updateErr = errors.New("write failed")
	reloader := &fakeReloader{}
	svc := NewChannelService(store, reloader, nil)

	_, err := svc.Update(context.Background(), existing.ID, &channelPatch{Name: "renamed"})

	if err == nil || err.Error() != "write failed" {
		t.Fatalf("err = %v, want the store error", err)
	}
	if reloader.calls != 0 {
		t.Errorf("a failed write still reloaded: calls = %d", reloader.calls)
	}
}

func TestChannelServiceUpdateDoesNotLoseAConcurrentEdit(t *testing.T) {
	// Reproduces the read-modify-write hazard: the caller reads the channel, a
	// second administrator changes a field the caller never touched, and the
	// caller's write-back must not resurrect the stale value.
	//
	// This is what "whole-row overwrite from a transaction-external read" means in
	// practice: without re-reading inside the write, the second administrator's
	// credential change is silently lost.
	store := newFakeChannelStore()
	existing := store.seed(&storage.ChannelRecord{
		Name: "chan-a", Type: "openai", BaseURL: "http://a.invalid",
		APIKey: "credential-v1", Models: []string{"m1"}, Priority: 3, Weight: 7,
		Status: "active",
	})

	// The concurrent edit lands after our read has produced its answer.
	store.afterGet = func() {
		store.channels[existing.ID].APIKey = "credential-v2"
	}

	svc := NewChannelService(store, nil, nil)
	if _, err := svc.Update(context.Background(), existing.ID, &channelPatch{Name: "renamed"}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	stored := store.channels[existing.ID]
	if stored.APIKey != "credential-v2" {
		t.Errorf("the concurrent credential change was lost: api_key = %q, want credential-v2", stored.APIKey)
	}
	if stored.Name != "renamed" {
		t.Errorf("the caller's own change was not applied: name = %q", stored.Name)
	}
}

// --- delete --------------------------------------------------------------

func TestChannelServiceDeleteReloads(t *testing.T) {
	store := newFakeChannelStore()
	existing := store.seed(&storage.ChannelRecord{Name: "chan-a"})
	reloader := &fakeReloader{}
	svc := NewChannelService(store, reloader, nil)

	if err := svc.Delete(context.Background(), existing.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if store.deleteCalls != 1 {
		t.Errorf("deleteCalls = %d, want 1", store.deleteCalls)
	}
	if reloader.calls != 1 || reloader.reasons[0] != "channel_deleted" {
		t.Errorf("reload reasons = %v, want one \"channel_deleted\"", reloader.reasons)
	}
}

func TestChannelServiceDeletePropagatesErrorWithoutReloading(t *testing.T) {
	store := newFakeChannelStore()
	store.deleteErr = errors.New("in use")
	reloader := &fakeReloader{}
	svc := NewChannelService(store, reloader, nil)

	if err := svc.Delete(context.Background(), 1); err == nil {
		t.Fatal("Delete swallowed the store error")
	}
	if reloader.calls != 0 {
		t.Errorf("a failed delete still reloaded: calls = %d", reloader.calls)
	}
}

// --- batch ---------------------------------------------------------------

func TestChannelServiceBatchDeleteReloadsOnce(t *testing.T) {
	store := newFakeChannelStore()
	a := store.seed(&storage.ChannelRecord{Name: "a"})
	b := store.seed(&storage.ChannelRecord{Name: "b"})
	reloader := &fakeReloader{}
	svc := NewChannelService(store, reloader, nil)

	n, err := svc.BatchDelete(context.Background(), []int64{a.ID, b.ID})
	if err != nil {
		t.Fatalf("BatchDelete: %v", err)
	}
	if n != 2 {
		t.Errorf("deleted = %d, want 2", n)
	}
	if reloader.calls != 1 || reloader.reasons[0] != "channels_batch_deleted" {
		t.Errorf("reload reasons = %v, want exactly one broadcast", reloader.reasons)
	}
}

// TestChannelServiceBatchStatusRefusesUnknownStatus is the regression for
// BUG-CP-03: an unrecognised status used to be coerced to "active", so a
// misspelled bulk disable enabled the channel. It must now be refused, and nothing
// may be written or reloaded.
func TestChannelServiceBatchStatusRefusesUnknownStatus(t *testing.T) {
	// Case and surrounding whitespace are normalised (that preserves the operator's
	// intent); anything that is not a known state is refused.
	for _, in := range []string{"bogus", "disable", "off", ""} {
		t.Run("status="+in, func(t *testing.T) {
			store := newFakeChannelStore()
			rec := store.seed(&storage.ChannelRecord{Name: "a", Status: "disabled"})
			reloader := &fakeReloader{}
			svc := NewChannelService(store, reloader, nil)

			if _, err := svc.BatchStatus(context.Background(), []int64{rec.ID}, in); err == nil {
				t.Fatalf("BatchStatus(%q) accepted an unknown status", in)
			}
			if store.lastBatchStatus != "" {
				t.Errorf("store received status %q for an invalid request", store.lastBatchStatus)
			}
			if len(reloader.reasons) != 0 {
				t.Errorf("reload happened for an invalid request: %v", reloader.reasons)
			}
		})
	}
}

// TestChannelServiceBatchStatusAppliesKnownStatus keeps the happy path pinned.
func TestChannelServiceBatchStatusAppliesKnownStatus(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"disabled", "disabled"},
		{"active", "active"},
		{" Disabled ", "disabled"},
		{"ACTIVE", "active"},
	} {
		t.Run("status="+tc.in, func(t *testing.T) {
			store := newFakeChannelStore()
			rec := store.seed(&storage.ChannelRecord{Name: "a", Status: "disabled"})
			reloader := &fakeReloader{}
			svc := NewChannelService(store, reloader, nil)

			n, err := svc.BatchStatus(context.Background(), []int64{rec.ID}, tc.in)
			if err != nil {
				t.Fatalf("BatchStatus: %v", err)
			}
			if n != 1 {
				t.Errorf("updated = %d, want 1", n)
			}
			if store.lastBatchStatus != tc.want {
				t.Errorf("store received status %q, want %q", store.lastBatchStatus, tc.want)
			}
			if reloader.calls != 1 || reloader.reasons[0] != "channels_batch_status_updated" {
				t.Errorf("reload reasons = %v, want one broadcast", reloader.reasons)
			}
		})
	}
}

// --- degradation ---------------------------------------------------------

func TestChannelServiceWorksWithoutOptionalDependencies(t *testing.T) {
	// The caller may legitimately have no cluster and no dispatcher.
	store := newFakeChannelStore()
	svc := NewChannelService(store, nil, nil)

	rec, err := svc.Create(context.Background(), validChannel())
	if err != nil {
		t.Fatalf("Create without reloader: %v", err)
	}
	if _, err := svc.List(); err != nil {
		t.Fatalf("List without breakers: %v", err)
	}
	if err := svc.Delete(context.Background(), rec.ID); err != nil {
		t.Fatalf("Delete without reloader: %v", err)
	}
	if _, err := svc.BatchDelete(context.Background(), []int64{rec.ID}); err != nil {
		t.Fatalf("BatchDelete without reloader: %v", err)
	}
	if _, err := svc.BatchStatus(context.Background(), []int64{rec.ID}, "active"); err != nil {
		t.Fatalf("BatchStatus without reloader: %v", err)
	}
}

func TestChannelServiceReportsSavedButNotApplied(t *testing.T) {
	// A failed reload is distinguished from a rejected write.
	store := newFakeChannelStore()
	reloader := &fakeReloader{err: errors.New("redis unreachable")}
	svc := NewChannelService(store, reloader, nil)

	rec, err := svc.Create(context.Background(), validChannel())
	var hot *HotReloadError
	if !errors.As(err, &hot) || rec == nil || store.channels[rec.ID] == nil {
		t.Fatalf("want committed channel plus hot-reload warning, got %v, %v", rec, err)
	}
	if reloader.calls != 1 {
		t.Errorf("reload calls = %d, want 1", reloader.calls)
	}
}
