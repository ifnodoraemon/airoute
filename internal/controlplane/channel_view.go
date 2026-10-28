package controlplane

import (
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// channelView is the wire representation of a channel.
//
// storage.ChannelRecord carries the upstream provider's credential in cleartext
// (channels.api_key holds the real key, unlike api_keys.key which holds a
// digest). Serializing the row directly therefore shipped a long-lived provider
// secret to the browser on every list/create/update call, while the console only
// ever needed to know whether one is configured: it recomputed that flag from the
// cleartext itself.
//
// The view names its fields explicitly and exposes only `api_key_configured` plus
// a display mask. The cleartext is still written by the create/update path (the
// request carries it) and read by the data plane; it simply never leaves the
// process again. This mirrors apiKeyView for client credentials.
type channelView struct {
	ID             int64              `json:"id"`
	Name           string             `json:"name"`
	Type           model.ProviderType `json:"type"`
	BaseURL        string             `json:"base_url"`
	Models         []string           `json:"models"`
	ModelMapping   map[string]string  `json:"model_mapping"`
	Protocols      []string           `json:"protocols,omitempty"`
	Priority       int                `json:"priority"`
	Weight         int                `json:"weight"`
	TimeoutSeconds int                `json:"timeout_seconds"`
	Status         string             `json:"status"`
	BreakerStatus  string             `json:"breaker_status,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
	// APIKeyConfigured replaces the old "the response contains the credential"
	// contract: the console says "already configured, leave blank to keep it".
	APIKeyConfigured bool `json:"api_key_configured"`
	// APIKeyMasked is for operator recognition only (which upstream key is this),
	// never for authentication.
	APIKeyMasked string `json:"api_key_masked,omitempty"`
}

// channelViewFrom projects a stored row onto the wire representation.
func channelViewFrom(rec *storage.ChannelRecord) channelView {
	if rec == nil {
		return channelView{}
	}
	return channelView{
		ID:               rec.ID,
		Name:             rec.Name,
		Type:             rec.Type,
		BaseURL:          rec.BaseURL,
		Models:           rec.Models,
		ModelMapping:     rec.ModelMapping,
		Protocols:        rec.Protocols,
		Priority:         rec.Priority,
		Weight:           rec.Weight,
		TimeoutSeconds:   rec.TimeoutSeconds,
		Status:           rec.Status,
		BreakerStatus:    rec.BreakerStatus,
		CreatedAt:        rec.CreatedAt,
		UpdatedAt:        rec.UpdatedAt,
		APIKeyConfigured: rec.APIKey != "",
		APIKeyMasked:     storage.MaskAPIKey(rec.APIKey),
	}
}

// channelViews projects a list, preserving order and returning an empty
// (non-null) slice so a caller with no channels receives [] rather than null.
func channelViews(records []*storage.ChannelRecord) []channelView {
	views := make([]channelView, 0, len(records))
	for _, rec := range records {
		views = append(views, channelViewFrom(rec))
	}
	return views
}
