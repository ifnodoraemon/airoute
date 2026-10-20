package controlplane

import (
	"context"
	"errors"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// --- fakes ---------------------------------------------------------------

type fakeSnapshotSource struct {
	channels  []model.ChannelConfig
	keys      []model.APIKeyConfig
	allKeys   []*storage.APIKeyRecord
	fallbacks map[string]string

	channelsErr  error
	keysErr      error
	allKeysErr   error
	fallbacksErr error
}

func (f *fakeSnapshotSource) ToModelChannels() ([]model.ChannelConfig, error) {
	if f.channelsErr != nil {
		return nil, f.channelsErr
	}
	return f.channels, nil
}

func (f *fakeSnapshotSource) ToModelAPIKeys() ([]model.APIKeyConfig, error) {
	if f.keysErr != nil {
		return nil, f.keysErr
	}
	return f.keys, nil
}

func (f *fakeSnapshotSource) ListAPIKeys() ([]*storage.APIKeyRecord, error) {
	if f.allKeysErr != nil {
		return nil, f.allKeysErr
	}
	return f.allKeys, nil
}

func (f *fakeSnapshotSource) GetModelFallbacks() (map[string]string, error) {
	if f.fallbacksErr != nil {
		return nil, f.fallbacksErr
	}
	return f.fallbacks, nil
}

type fakeSnapshotSink struct {
	updateChannelsCalls int
	setFallbacksCalls   int
	lastChannels        []model.ChannelConfig
	lastFallbacks       map[string]string
}

func (f *fakeSnapshotSink) UpdateChannels(channels []model.ChannelConfig) {
	f.updateChannelsCalls++
	f.lastChannels = channels
}

func (f *fakeSnapshotSink) SetModelFallbacks(fallbacks map[string]string) {
	f.setFallbacksCalls++
	f.lastFallbacks = fallbacks
}

type fakeKeySink struct {
	calls    int
	lastKeys []model.APIKeyConfig
	lastHas  bool
}

func (f *fakeKeySink) UpdateAPIKeys(keys []model.APIKeyConfig, hasConfiguredKeys bool) {
	f.calls++
	f.lastKeys = keys
	f.lastHas = hasConfiguredKeys
}

func newTestSynchronizer(src SnapshotSource, sink SnapshotSink, keys KeyConfigSink, prices PriceReloader) *Synchronizer {
	return &Synchronizer{source: src, sink: sink, keys: keys, prices: prices}
}

func healthySource() *fakeSnapshotSource {
	return &fakeSnapshotSource{
		channels:  []model.ChannelConfig{{ID: 1, Name: "chan-a", Type: "openai"}},
		keys:      []model.APIKeyConfig{{Key: "sk-live"}},
		allKeys:   []*storage.APIKeyRecord{{Key: "sk-live", Status: "active"}},
		fallbacks: map[string]string{"gpt-x": "gpt-y"},
	}
}

// --- the security regression ---------------------------------------------

func TestReloadFromDBDoesNotClearRegisteredKeysOnReadFailure(t *testing.T) {
	// Regression: the "has any key ever been registered" flag is what the auth
	// middleware uses to decide whether to enforce authentication at all
	// (internal/middleware/auth.go). It used to be derived from a ListAPIKeys
	// call whose error was discarded, so a transient failure reported "nothing
	// was ever registered" — and with no active keys at that moment the gateway
	// would serve unauthenticated traffic.
	src := healthySource()
	src.keys = nil // every key currently disabled
	src.allKeysErr = errors.New("connection reset")
	sink := &fakeSnapshotSink{}
	keySink := &fakeKeySink{}
	sync := newTestSynchronizer(src, sink, keySink, nil)

	err := sync.ReloadFromDB()

	if err == nil {
		t.Fatal("a failed key read was reported as a successful reload")
	}
	if keySink.calls != 0 {
		t.Fatalf("the key set was rewritten from a failed read: calls = %d, HasKeys = %v",
			keySink.calls, keySink.lastHas)
	}
	if sink.updateChannelsCalls != 0 {
		t.Errorf("channels were applied despite the failed read: calls = %d", sink.updateChannelsCalls)
	}
	if got := sync.Version(); got != 0 {
		t.Errorf("version = %d, want 0 for a reload that changed nothing", got)
	}
}

func TestReloadFromDBHasKeysFalseOnlyWhenNoKeyEverRegistered(t *testing.T) {
	// The flag legitimately reads false when the gateway has genuinely never had
	// a key; that is the open dev mode. It must not be produced by a failure.
	src := healthySource()
	src.keys = nil
	src.allKeys = nil
	keySink := &fakeKeySink{}
	sync := newTestSynchronizer(src, &fakeSnapshotSink{}, keySink, nil)

	if err := sync.ReloadFromDB(); err != nil {
		t.Fatalf("ReloadFromDB: %v", err)
	}
	if keySink.lastHas {
		t.Error("HasKeys = true with no key ever registered")
	}

	// And with a disabled-but-registered key it must read true.
	src2 := healthySource()
	src2.keys = nil
	src2.allKeys = []*storage.APIKeyRecord{{Key: "sk-disabled", Status: "disabled"}}
	keySink2 := &fakeKeySink{}
	sync2 := newTestSynchronizer(src2, &fakeSnapshotSink{}, keySink2, nil)

	if err := sync2.ReloadFromDB(); err != nil {
		t.Fatalf("ReloadFromDB: %v", err)
	}
	if !keySink2.lastHas {
		t.Error("HasKeys = false although a (disabled) key is registered")
	}
}

// --- two-phase switching -------------------------------------------------

func TestReloadFromDBLeavesTheDataPlaneUntouchedOnAnyReadFailure(t *testing.T) {
	boom := errors.New("db unavailable")

	cases := []struct {
		name   string
		break_ func(*fakeSnapshotSource)
	}{
		{"channels read fails", func(s *fakeSnapshotSource) { s.channelsErr = boom }},
		{"keys read fails", func(s *fakeSnapshotSource) { s.keysErr = boom }},
		{"registered key count fails", func(s *fakeSnapshotSource) { s.allKeysErr = boom }},
		{"fallbacks read fails", func(s *fakeSnapshotSource) { s.fallbacksErr = boom }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := healthySource()
			tc.break_(src)
			sink := &fakeSnapshotSink{}
			keySink := &fakeKeySink{}
			sync := newTestSynchronizer(src, sink, keySink, nil)

			if err := sync.ReloadFromDB(); err == nil {
				t.Fatal("a failed read was reported as a successful reload")
			}

			if sink.updateChannelsCalls != 0 {
				t.Errorf("channels were applied from a partial read: %d", sink.updateChannelsCalls)
			}
			if sink.setFallbacksCalls != 0 {
				t.Errorf("fallbacks were applied from a partial read: %d", sink.setFallbacksCalls)
			}
			if keySink.calls != 0 {
				t.Errorf("keys were applied from a partial read: %d", keySink.calls)
			}
			if got := sync.Version(); got != 0 {
				t.Errorf("version advanced to %d on a failed reload", got)
			}
		})
	}
}

func TestReloadFromDBAppliesEverythingAndBumpsVersion(t *testing.T) {
	src := healthySource()
	sink := &fakeSnapshotSink{}
	keySink := &fakeKeySink{}
	prices := &fakePriceReloader{}
	sync := newTestSynchronizer(src, sink, keySink, prices)

	if err := sync.ReloadFromDB(); err != nil {
		t.Fatalf("ReloadFromDB: %v", err)
	}

	if sink.updateChannelsCalls != 1 {
		t.Errorf("UpdateChannels calls = %d, want 1", sink.updateChannelsCalls)
	}
	if len(sink.lastChannels) != 1 || sink.lastChannels[0].Name != "chan-a" {
		t.Errorf("applied channels = %+v, want the loaded set", sink.lastChannels)
	}
	if keySink.calls != 1 || !keySink.lastHas {
		t.Errorf("key sink calls = %d, HasKeys = %v", keySink.calls, keySink.lastHas)
	}
	if len(keySink.lastKeys) != 1 || keySink.lastKeys[0].Key != "sk-live" {
		t.Errorf("applied keys = %+v", keySink.lastKeys)
	}
	if sink.setFallbacksCalls != 1 || sink.lastFallbacks["gpt-x"] != "gpt-y" {
		t.Errorf("applied fallbacks = %+v", sink.lastFallbacks)
	}
	if prices.calls != 1 {
		t.Errorf("price reload calls = %d, want 1", prices.calls)
	}
	if got := sync.Version(); got != 1 {
		t.Errorf("version = %d, want 1", got)
	}

	// A second successful switch advances the version again.
	if err := sync.ReloadFromDB(); err != nil {
		t.Fatalf("second ReloadFromDB: %v", err)
	}
	if got := sync.Version(); got != 2 {
		t.Errorf("version = %d, want 2", got)
	}
}

func TestReloadFromDBPriceFailureIsAPartialSwitch(t *testing.T) {
	// The in-memory switch cannot be undone, so the configuration is live and the
	// version must say so: it identifies the snapshot in effect, and after a partial
	// apply that snapshot really is in effect. Freezing it would leave the version
	// naming a configuration that is no longer running.
	//
	// The lag is reported instead, as a HotReloadError naming the stage that did not
	// follow, so a caller can tell "fully applied" from "applied, engine lagging".
	src := healthySource()
	sink := &fakeSnapshotSink{}
	keySink := &fakeKeySink{}
	prices := &fakePriceReloader{err: errors.New("engine unavailable")}
	sync := newTestSynchronizer(src, sink, keySink, prices)

	err := sync.ReloadFromDB()

	var hotErr *HotReloadError
	if !errors.As(err, &hotErr) {
		t.Fatalf("err = %v, want *HotReloadError", err)
	}
	if hotErr.Stage != "model prices" {
		t.Errorf("Stage = %q, want it to name the lagging subsystem", hotErr.Stage)
	}
	if sink.updateChannelsCalls != 1 || keySink.calls != 1 {
		t.Error("the in-memory switch should still have been applied")
	}
	if got := sync.Version(); got != 1 {
		t.Errorf("version = %d, want 1: the snapshot is in effect", got)
	}
}

func TestReloadFromDBWithoutSourceFailsCleanly(t *testing.T) {
	sync := newTestSynchronizer(nil, &fakeSnapshotSink{}, &fakeKeySink{}, nil)

	if err := sync.ReloadFromDB(); err == nil {
		t.Fatal("a missing configuration source was reported as success")
	}
	if got := sync.Version(); got != 0 {
		t.Errorf("version = %d, want 0", got)
	}
}

func TestReloadFromDBToleratesMissingSink(t *testing.T) {
	// A synchronizer with no dispatcher must not panic.
	sync := newTestSynchronizer(healthySource(), nil, nil, nil)

	if err := sync.ReloadFromDB(); err != nil {
		t.Fatalf("ReloadFromDB without a sink: %v", err)
	}
}

// --- broadcast -----------------------------------------------------------

func TestReloadAndBroadcastReturnsTheReloadError(t *testing.T) {
	// Without a Redis cluster configured the broadcast is a no-op, so the reload
	// result is the whole answer.
	src := healthySource()
	src.channelsErr = errors.New("db unavailable")
	sync := newTestSynchronizer(src, &fakeSnapshotSink{}, &fakeKeySink{}, nil)

	if err := sync.ReloadAndBroadcast(context.Background(), "test"); err == nil {
		t.Fatal("ReloadAndBroadcast hid the reload failure")
	}
}
