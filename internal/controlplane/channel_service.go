package controlplane

import (
	"context"
	"errors"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// ChannelStore is the persistence surface the channel rules need.
//
// It is narrow on purpose. It documents exactly which storage calls the channel
// domain is allowed to make, and it lets the rules be tested with a fake instead
// of a live database — which is what makes the validation and merge rules below
// testable at all.
type ChannelStore interface {
	ListChannels() ([]*storage.ChannelRecord, error)
	// PatchChannel applies a mutation to the stored channel inside a single
	// transaction. Update deliberately depends on this rather than on
	// GetChannel + UpdateChannel: a read-modify-write across two calls loses a
	// concurrent edit made in between.
	PatchChannel(id int64, fn func(*storage.ChannelRecord) error) (*storage.ChannelRecord, error)
	CreateChannel(rec *storage.ChannelRecord) error
	UpdateChannel(rec *storage.ChannelRecord) error
	DeleteChannel(id int64) error
	BatchDeleteChannels(ids []int64) (int64, error)
	BatchUpdateChannelStatus(ids []int64, status string) (int64, error)
}

// ChannelReloader pushes a configuration change into the data plane and, when a
// cluster is configured, to its peers. Implemented by *Synchronizer.
type ChannelReloader interface {
	ReloadAndBroadcast(ctx context.Context, reason string) error
}

// BreakerStatusReader reports the live circuit-breaker state of a channel.
// Implemented by *router.Dispatcher.
type BreakerStatusReader interface {
	GetBreakerStatus(name string) string
}

// ErrChannelNotFound reports an operation against a channel that does not exist.
// Handlers map it to 404.
var ErrChannelNotFound = errors.New("channel not found")

// ValidationError marks input the caller has to correct. Handlers map it to 400.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

// channelPatch is a partial channel update.
//
// The numeric fields are pointers so the merge can tell "the client omitted it"
// from "the client sent 0". The distinction is load-bearing for priority and
// weight: they are ordered/weighted router inputs where 0 is a value an operator
// may want, and the old merge (`if patch.Priority != 0`) silently treated it as
// "leave unchanged", so a request to set 0 returned 200 while changing nothing.
//
// Text fields stay plain strings where empty means "not provided". That is not an
// oversight: name, type and base_url are required, so an empty value can never be
// a meaningful update, and api_key empty is the documented "keep the stored
// credential". The zero-value ambiguity only exists for the numbers.
//
// It is a controlplane type rather than a storage one because
// storage.ChannelRecord is the full row: its zero values are indistinguishable
// from an omitted field.
type channelPatch struct {
	Name              string
	Type              model.ProviderType
	BaseURL           string
	APIKey            string
	Models            []string
	ModelMapping      map[string]string
	ModelCapabilities map[string]model.ModelCapabilityConfig
	Protocols         []string
	Priority          *int
	Weight            *int
	TimeoutSeconds    *int
	MaxConcurrency    *int
	QueueLimit        *int
	QueueWaitSeconds  *int
	Status            string
}

// ChannelService owns the channel business rules: which fields a channel cannot
// be created without, how a partial update merges onto the stored record, which
// status values are legal, and when the data plane has to be reloaded.
//
// The point of the type is that adding or changing a channel rule no longer
// means editing an HTTP handler, and that the rules can be exercised without a
// router, a database or an HTTP request.
type ChannelService struct {
	store    ChannelStore
	reloader ChannelReloader
	breakers BreakerStatusReader
}

// NewChannelService wires the channel rules to their dependencies. reloader and
// breakers may be nil; every use is guarded, because the caller can legitimately
// run without a cluster or a dispatcher (tests, CLI tooling).
func NewChannelService(store ChannelStore, reloader ChannelReloader, breakers BreakerStatusReader) *ChannelService {
	return &ChannelService{store: store, reloader: reloader, breakers: breakers}
}

// List returns every channel, decorated with its live circuit-breaker state.
func (s *ChannelService) List() ([]*storage.ChannelRecord, error) {
	channels, err := s.store.ListChannels()
	if err != nil {
		return nil, err
	}
	if s.breakers != nil {
		for _, ch := range channels {
			ch.BreakerStatus = s.breakers.GetBreakerStatus(ch.Name)
		}
	}
	return channels, nil
}

// Create validates and persists a new channel, then reloads the data plane.
func (s *ChannelService) Create(ctx context.Context, rec *storage.ChannelRecord) (*storage.ChannelRecord, error) {
	if err := validateNewChannel(rec); err != nil {
		return nil, err
	}
	if err := s.store.CreateChannel(rec); err != nil {
		return nil, err
	}
	return rec, s.reload(ctx, "channel_created")
}

// Update merges a partial update onto the stored channel and persists it.
//
// The merge runs inside the store's transaction, against the row as it is at
// write time. That is the whole point: reading the channel here, merging in Go
// and writing the whole row back would revert any field a second administrator
// changed in the meantime — a lost update that no error would ever reveal.
//
// Fields the caller omitted keep their stored values, so a partial update cannot
// wipe a credential or a routing weight.
func (s *ChannelService) Update(ctx context.Context, id int64, patch *channelPatch) (*storage.ChannelRecord, error) {
	updated, err := s.store.PatchChannel(id, func(current *storage.ChannelRecord) error {
		if err := mergeChannelUpdate(current, patch); err != nil {
			return err
		}
		timeout, err := normalizeChannelTimeout(current.TimeoutSeconds)
		if err != nil {
			return err
		}
		current.TimeoutSeconds = timeout
		if err := model.ValidateChannelThinking(current.Type, current.ModelCapabilities); err != nil {
			return &ValidationError{Message: err.Error()}
		}
		return validateChannelCapacity(current)
	})
	if err != nil {
		// Only a genuinely missing row becomes a 404. A read or write failure
		// propagates: reporting a database outage as "channel not found" made the
		// console tell operators the channel had been deleted.
		if errors.Is(err, storage.ErrChannelNotFound) {
			return nil, ErrChannelNotFound
		}
		return nil, err
	}

	return updated, s.reload(ctx, "channel_updated")
}

// Delete removes a channel and reloads the data plane.
func (s *ChannelService) Delete(ctx context.Context, id int64) error {
	if err := s.store.DeleteChannel(id); err != nil {
		return err
	}
	return s.reload(ctx, "channel_deleted")
}

// BatchDelete removes several channels and reloads once.
func (s *ChannelService) BatchDelete(ctx context.Context, ids []int64) (int64, error) {
	n, err := s.store.BatchDeleteChannels(ids)
	if err != nil {
		return 0, err
	}
	return n, s.reload(ctx, "channels_batch_deleted")
}

// BatchStatus toggles several channels and reloads once.
func (s *ChannelService) BatchStatus(ctx context.Context, ids []int64, status string) (int64, error) {
	normalized := normalizeChannelStatus(status)
	if normalized == "" {
		return 0, &ValidationError{Message: "无效的状态，仅支持 active 或 disabled"}
	}
	n, err := s.store.BatchUpdateChannelStatus(ids, normalized)
	if err != nil {
		return 0, err
	}
	return n, s.reload(ctx, "channels_batch_status_updated")
}

// reload pushes the change into the data plane.
//
// The write has committed: report delayed application separately from rejection.
func (s *ChannelService) reload(ctx context.Context, reason string) error {
	if s.reloader == nil {
		return nil
	}
	if err := s.reloader.ReloadAndBroadcast(ctx, reason); err != nil {
		return &HotReloadError{Stage: "渠道配置", Err: err}
	}
	return nil
}
