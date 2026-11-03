package router

import (
	"context"
	"io"
	"net/http"
	"sync"
)

// =============================================================================
// Unified Multimodal Forwarding Pipeline (Images, Audio, Video, Generic HTTP)
// =============================================================================

// UpstreamRequest represents a generic HTTP request across any modality.
type UpstreamRequest struct {
	PinnedChannel   string
	PinnedChannelID int64
	PinnedBaseURL   string
	Path            string            // e.g. "/v1/images/generations", "/v1/audio/speech"
	Method          string            // "POST", "GET"
	Headers         map[string]string // custom headers
	Body            []byte            // serialized payload
	ContentType     string            // "application/json", "multipart/form-data"
	Model           string            // requested model for routing
	Protocol        string            // e.g. "images", "audio_speech", "videos"
	GeminiAction    string            // native Gemini action; filters candidates to Gemini upstreams
	GeminiPath      string            // exact native resource path, selected by the gateway
	PinnedIdentity  string
	NoRetry         bool
	NoRewriteBody   bool
	BillableUnits   int // fixed-price units; e.g. requested image count
}

// UpstreamResponse encapsulates a generic HTTP response across any modality.
type UpstreamResponse struct {
	ProviderName string
	ChannelID    int64
	BaseURL      string
	StatusCode   int
	Headers      http.Header
	Body         []byte
	Stream       io.ReadCloser
	// Channel is the routing chain that served this request (e.g. "A" or
	// "A→B" after fallback). Internal-only metadata for audit logging.
	Channel string
}

// cancelOnCloseReadCloser releases the channel's per-request budget exactly when
// the caller is done reading the upstream body. Modality responses (image JSON,
// audio bytes, video payloads) are streamed to the client after DispatchHTTP
// returns, so cancelling the timeout context on return would abort the body
// mid-copy.
type cancelOnCloseReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
	once   sync.Once
}

func (c *cancelOnCloseReadCloser) Close() error {
	err := c.ReadCloser.Close()
	c.once.Do(c.cancel)
	return err
}
