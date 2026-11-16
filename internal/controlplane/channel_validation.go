package controlplane

import (
	"fmt"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// Channel input validation and partial-update merging, extracted from
// channel_service.go to keep the service orchestration under the size red line.

func validateNewChannel(rec *storage.ChannelRecord) error {
	if rec == nil || rec.Name == "" || rec.BaseURL == "" || rec.Type == "" {
		return &ValidationError{Message: "渠道名称、类型与 base_url 为必填项"}
	}
	// A provided status must be one the data plane understands; see the note in
	// mergeChannelUpdate.
	if status := strings.ToLower(strings.TrimSpace(rec.Status)); status != "" {
		if !validToggleStatus(status) {
			return &ValidationError{Message: "无效的状态，仅支持 active 或 disabled"}
		}
		rec.Status = status
	}
	timeout, err := normalizeChannelTimeout(rec.TimeoutSeconds)
	if err != nil {
		return err
	}
	rec.TimeoutSeconds = timeout
	if err := model.ValidateChannelThinking(rec.Type, rec.ModelCapabilities); err != nil {
		return &ValidationError{Message: err.Error()}
	}
	return validateChannelCapacity(rec)
}

// normalizeChannelTimeout turns the single operator-facing knob into a validated
// per-request upstream budget.
//
// The channel timeout is the only timeout an operator should have to think about.
// It is also the window a channel may stay silent before the first token arrives -
// there is no separate value to keep in sync - and the edge proxy's timeout is
// configured far above the maximum accepted here, so a configured budget is always
// the one that takes effect. Rejecting out-of-range
// values is what makes that guarantee hold: silently accepting a 3000s budget
// behind a 600s proxy produced a number that never decided anything.
func normalizeChannelTimeout(seconds int) (int, error) {
	if seconds == 0 {
		return model.DefaultChannelTimeoutSeconds, nil
	}
	if seconds < model.MinChannelTimeoutSeconds {
		return 0, &ValidationError{Message: fmt.Sprintf("渠道超时必须大于 %d 秒", model.MinChannelTimeoutSeconds)}
	}
	if seconds > model.MaxChannelTimeoutSeconds {
		return 0, &ValidationError{Message: fmt.Sprintf(
			"渠道超时不能超过 %d 秒（约 %d 分钟）。上限由边界代理预算 %d 秒派生（预留 %d 秒余量），"+
				"保证网关自己的超时总是先于代理生效；如需更长预算，需要同时调大代理预算",
			model.MaxChannelTimeoutSeconds, model.MaxChannelTimeoutSeconds/60,
			model.EdgeProxyTimeoutSeconds, model.EdgeProxyTimeoutSeconds-model.MaxChannelTimeoutSeconds)}
	}
	return seconds, nil
}

// normalizeChannelStatus enforces the two-state contract.
//
// It returns "" for an unrecognised value so callers must decide what to do; it
// used to coerce anything that was not exactly "disabled" to "active", which made
// a misspelled disable silently ENABLE the channel (BUG-CP-03).
func normalizeChannelStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		// An omitted status in a bulk request means "no opinion": callers that
		// reach here through bindBatchStatus never do (it rejects an empty value
		// before this point).
		return ""
	}
	if !validToggleStatus(status) {
		return ""
	}
	return status
}

// mergeChannelUpdate copies only the fields the client actually provided onto the
// existing channel record.
//
// For the numeric fields "provided" is decided by the pointer being non-nil,
// never by the value being non-zero: an explicit priority=0 or weight=0 therefore
// reaches storage instead of being silently discarded. An omitted field keeps its
// stored value, so a partial update cannot reset routing weights.
//
// A provided zero is still subject to the same "0 means default" rule that
// creation uses (see storage.CreateChannel / updateChannelTx): priority 0 becomes
// 1 and weight 0 becomes 10 there. The point of this function is that the API no
// longer pretends a field was applied when it was dropped, not that storage's
// default policy changes.
func mergeChannelUpdate(existing *storage.ChannelRecord, patch *channelPatch) error {
	if patch == nil {
		return nil
	}
	if patch.Name != "" {
		existing.Name = patch.Name
	}
	if patch.Type != "" {
		existing.Type = patch.Type
	}
	if patch.BaseURL != "" {
		existing.BaseURL = patch.BaseURL
	}
	if patch.APIKey != "" {
		existing.APIKey = patch.APIKey
	}
	if patch.Models != nil {
		existing.Models = patch.Models
	}
	if patch.ModelMapping != nil {
		existing.ModelMapping = patch.ModelMapping
	}
	if patch.ModelCapabilities != nil {
		existing.ModelCapabilities = patch.ModelCapabilities
	}
	if patch.Protocols != nil {
		existing.Protocols = patch.Protocols
	}
	if patch.Priority != nil {
		existing.Priority = *patch.Priority
	}
	if patch.Weight != nil {
		existing.Weight = *patch.Weight
	}
	if patch.TimeoutSeconds != nil {
		existing.TimeoutSeconds = *patch.TimeoutSeconds
	}
	if patch.MaxConcurrency != nil {
		existing.MaxConcurrency = *patch.MaxConcurrency
	}
	if patch.QueueLimit != nil {
		existing.QueueLimit = *patch.QueueLimit
	}
	if patch.QueueWaitSeconds != nil {
		existing.QueueWaitSeconds = *patch.QueueWaitSeconds
	}
	if patch.Status != "" {
		// An unknown status is rejected rather than stored: the data plane treats
		// every value except "" and "active" as disabled, so a typo ("disable"),
		// or a value from an older client, silently took a channel out of service
		// (or, through the batch endpoint, silently enabled it). See BUG-CP-03.
		status := strings.ToLower(strings.TrimSpace(patch.Status))
		if !validToggleStatus(status) {
			return &ValidationError{Message: "无效的状态，仅支持 active 或 disabled"}
		}
		existing.Status = status
	}
	return nil
}