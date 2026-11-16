package controlplane

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// SnapshotSource reads the complete configuration from durable storage. Every
// method must either succeed or report an error: a partial read is never
// applied to the data plane.
type SnapshotSource interface {
	ToModelChannels() ([]model.ChannelConfig, error)
	ToModelAPIKeys() ([]model.APIKeyConfig, error)
	HasRegisteredAPIKeys() (bool, error)
	GetModelFallbacks() (map[string]string, error)
}

// SnapshotSink installs a complete configuration into the data plane.
type SnapshotSink interface {
	UpdateChannels(channels []model.ChannelConfig)
	SetModelFallbacks(fallbacks map[string]string)
}

// KeyConfigSink installs the API key set into the runtime configuration.
type KeyConfigSink interface {
	UpdateAPIKeys(keys []model.APIKeyConfig, hasConfiguredKeys bool)
}

// ConfigSnapshot is a complete, fully read view of the configuration that can be
// installed in one step.
type ConfigSnapshot struct {
	Channels []model.ChannelConfig
	APIKeys  []model.APIKeyConfig
	// HasKeys records whether any key has ever been registered, including
	// disabled and deleted ones. The auth middleware uses it to decide whether to enforce
	// authentication at all, so it must never be derived from a failed read.
	HasKeys   bool
	Fallbacks map[string]string
}

// HotReloadError reports that a change is persisted and in effect in the database,
// but the in-memory state that serves traffic did not pick it up.
//
// It is deliberately distinct from a plain failure, and it is the single way this
// package expresses "the write succeeded but is not fully live":
//
//   - The write committed, so the operator's change survives a restart and must not
//     be reported as rejected.
//   - Requests are still served by the previous configuration until a reload
//     succeeds, so reporting plain success would be a lie.
//
// Stage names the part that lagged (for example "model prices"), which is what lets
// a caller tell a fully applied change from a partially applied one.
type HotReloadError struct {
	Stage string
	Err   error
}

func (e *HotReloadError) Error() string {
	if e.Stage == "" {
		return e.Err.Error()
	}
	return e.Stage + ": " + e.Err.Error()
}

func (e *HotReloadError) Unwrap() error { return e.Err }

// Synchronizer synchronizes persistent database state to the in-memory Data Plane.
type Synchronizer struct {
	mu         sync.Mutex
	repo       *storage.Repository
	dispatcher *router.Dispatcher

	source SnapshotSource
	sink   SnapshotSink
	keys   KeyConfigSink
	prices PriceReloader

	// version counts fully applied configuration switches. It only advances when
	// every part of the configuration was read and installed, which is what makes
	// "which configuration is live" answerable.
	version atomic.Uint64

	// lastFingerprint is the digest of the snapshot currently installed. The
	// periodic 10s reload runs unconditionally, so without this the log carried
	// an identical "hot reloaded data plane memory state" INFO line every 10s
	// per replica — about 20% of all log volume — and buried the reloads that
	// actually changed something.
	lastFingerprint string

	// onReady fires exactly once, after the first complete snapshot is installed.
	// The server uses it to leave the fail-closed startup state.
	onReady func()
	// ready records that a snapshot was installed at least once.
	ready bool
}

// NewSynchronizer creates a new Synchronizer.
func NewSynchronizer(repo *storage.Repository, dispatcher *router.Dispatcher) *Synchronizer {
	// The conversions are guarded because a nil concrete pointer stored in an
	// interface is a non-nil interface: the nil checks in ReloadFromDB would pass
	// and the first call would panic.
	var source SnapshotSource
	if repo != nil {
		source = repo
	}
	var sink SnapshotSink
	if dispatcher != nil {
		sink = dispatcher
	}

	return &Synchronizer{
		repo:       repo,
		dispatcher: dispatcher,
		source:     source,
		sink:       sink,
		keys:       globalKeyConfigSink{},
		prices:     globalPriceReloader{},
	}
}

// Version returns the number of fully applied configuration switches.
func (s *Synchronizer) Version() uint64 {
	return s.version.Load()
}

// ReloadFromDB rebuilds the data plane from the database.
//
// It is deliberately two-phase: the whole configuration is read first, and the
// running state is replaced only once every read has succeeded. Applying each
// concern as it was read meant a failure part way through left the data plane
// holding a mix of old and new configuration — new channels with stale API keys,
// for instance.
//
// Two of those reads used to be optional, and both mattered:
//
//   - Registered-key lookup errors were discarded, so a transient failure reported "no key
//     has ever been registered". That is the flag the auth middleware uses to
//     decide whether to enforce authentication at all, so the failure could open
//     the gateway to unauthenticated traffic.
//   - GetModelFallbacks errors were discarded, silently keeping stale fallbacks
//     while the reload reported success.
//
// Prices are the one part that cannot join the switch: the billing engine reloads
// from the database itself. When that fails the snapshot is already live, so the
// version advances and the lag is reported as a HotReloadError. Freezing the
// version instead would leave it identifying a configuration that is no longer
// running — the version's job is to name the snapshot in effect, and after a
// partial apply that snapshot really is in effect.
func (s *Synchronizer) ReloadFromDB() error {
	s.mu.Lock()
	applied, err := s.reloadLocked()
	firstReady := applied && !s.ready
	if applied {
		s.ready = true
	}
	onReady := s.onReady
	s.mu.Unlock()

	// Fire outside the lock: the callback flips the data-plane readiness flag the
	// auth middleware reads, and running it here keeps "snapshot installed" and
	// "traffic allowed" ordered (the first load must complete before ListenAndServe).
	if firstReady && onReady != nil {
		onReady()
	}
	return err
}

// SetOnReady registers the callback invoked once, after the first snapshot is
// installed. Subsequent snapshot applications do not fire it again.
func (s *Synchronizer) SetOnReady(fn func()) {
	s.mu.Lock()
	s.onReady = fn
	ready := s.ready
	s.mu.Unlock()
	if ready && fn != nil {
		// A snapshot was already installed before the callback was registered.
		fn()
	}
}

// Ready reports whether at least one complete snapshot has been installed.
func (s *Synchronizer) Ready() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ready
}

// reloadLocked performs one load-and-apply cycle. It reports whether a complete
// snapshot was installed: a partial apply (model prices failing after the switch)
// still installed the configuration, and the readiness flag must reflect that.
func (s *Synchronizer) reloadLocked() (bool, error) {
	snap, err := s.loadSnapshot()
	if err != nil {
		// Nothing was mutated: the data plane still runs the previous snapshot, and
		// the version still identifies it.
		return false, err
	}

	s.applySnapshot(snap)

	// The snapshot is in effect from here on, so the version moves with it even if a
	// dependent subsystem lags.
	version := s.version.Add(1)
	fingerprint := snapshotFingerprint(snap)
	changed := fingerprint != s.lastFingerprint
	s.lastFingerprint = fingerprint
	logger := telemetry.ScopedLogger("sync")
	if changed {
		logger.Info("hot reloaded data plane memory state from database",
			"version", version,
			"active_channels", len(snap.Channels),
			"active_keys", len(snap.APIKeys),
		)
	} else {
		// The periodic reload found byte-identical configuration. Keep it visible
		// at debug level rather than repeating the same INFO line every 10s.
		logger.Debug("data plane snapshot unchanged; periodic reload was a no-op",
			"version", version,
			"active_channels", len(snap.Channels),
			"active_keys", len(snap.APIKeys),
		)
	}

	if s.prices != nil {
		if err := s.prices.ReloadPrices(); err != nil {
			return true, &HotReloadError{
				Stage: "model prices",
				Err:   fmt.Errorf("reload model prices error: %w", err),
			}
		}
	}

	return true, nil
}
