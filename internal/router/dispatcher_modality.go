package router

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
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
	BillableUnits   int               // fixed-price units; e.g. requested image count
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

// dispatchHTTPOnce routes one modality HTTP request across matching candidate
// channels. The exported DispatchHTTP wrapper adds the total request budget, the
// cross-model fallback and the budget release tied to the response body lifetime.
func (d *Dispatcher) dispatchHTTPOnce(ctx context.Context, req *UpstreamRequest) (*UpstreamResponse, error) {
	var channels []*model.ChannelConfig
	if req.PinnedChannel != "" {
		for _, ch := range d.ListChannelsForModel(req.Model, req.Protocol) {
			if ((req.PinnedChannelID > 0 && ch.ID == req.PinnedChannelID) || (req.PinnedChannelID == 0 && ch.Name == req.PinnedChannel)) && ch.BaseURL == req.PinnedBaseURL {
				channels = append(channels, ch)
				break
			}
		}
	} else {
		channels = d.ResolveCandidateChannels(ctx, req.Model, req.Protocol)
	}
	if len(channels) == 0 {
		return nil, newNoRouteError(ctx, req.Model, fmt.Errorf("no upstream provider available for model '%s' and protocol '%s'", req.Model, req.Protocol))
	}

	var lastErr error
	var attempted []string // channels actually called and failed
	var skipped []string   // channels bypassed by the circuit breaker, never called
	for i, ch := range channels {
		// The budget may have expired while waiting on the previous attempt; stop
		// before paying the next channel timeout too (B-01).
		if ctxErr := ctx.Err(); ctxErr != nil {
			if requestBudgetExceeded(ctx, ctxErr) {
				return nil, newRequestBudgetError(req.Model, attempted, lastErr)
			}
			return nil, newCanceledError(ctx, attempted, "", 0)
		}

		// Circuit Breaker: fail fast if upstream is down
		if !d.circuitBreaker.CanExecute(ch.Name) {
			telemetry.WarnLogger(routerLog(ctx), "breaker-open:"+ch.Name, 30*time.Second,
				"circuit breaker is OPEN, bypassing dead upstream",
				"channel", ch.Name, "attempt", i+1)
			skipped = append(skipped, ch.Name)
			continue
		}

		targetModel := ch.GetUpstreamModel(req.Model)
		targetBody := req.Body
		contentType := req.ContentType
		if len(targetBody) > 0 {
			switch {
			case strings.Contains(contentType, "application/json"):
				targetBody = RewriteJSONModel(targetBody, targetModel)
			case strings.Contains(strings.ToLower(contentType), "multipart/form-data"):
				// Audio transcription/translation requests carry the model in a
				// multipart form field, not in JSON. Without this rewrite the
				// channel's model alias (e.g. speech/asr -> whisper-1) was only
				// applied to JSON bodies, so STT requests reached the upstream
				// with the downstream alias and were rejected.
				if rewritten, newContentType, err := RewriteMultipartModel(targetBody, contentType, targetModel); err != nil {
					routerLog(ctx).Warn("failed to rewrite model alias in multipart body, forwarding original",
						"channel", ch.Name, "error", err)
				} else {
					targetBody = rewritten
					contentType = newContentType
				}
			}
		}

		targetURL := provider.APIURL(ch.BaseURL, req.Path)
		// The channel timeout bounds the upstream exchange like every other
		// dispatch path; before this, images/audio/video had no upstream budget
		// at all and a hung provider held the request until the client left.
		callCtx, cancelCall := channelCallContext(ctx, ch)
		httpReq, err := http.NewRequestWithContext(callCtx, req.Method, targetURL, bytes.NewReader(targetBody))
		if err != nil {
			cancelCall()
			lastErr = err
			d.circuitBreaker.AbortCanary(ch.Name)
			continue
		}

		if contentType != "" {
			httpReq.Header.Set("Content-Type", contentType)
		}
		if ch.APIKey != "" && ch.APIKey != "none" {
			httpReq.Header.Set("Authorization", "Bearer "+ch.APIKey)
			httpReq.Header.Set("x-api-key", ch.APIKey)
		}
		for k, v := range req.Headers {
			httpReq.Header.Set(k, v)
		}
		if tid := middleware.GetTraceIDFromContext(ctx); tid != "" {
			httpReq.Header.Set(middleware.HeaderAirouteTraceID, tid)
			httpReq.Header.Set(middleware.HeaderRequestID, tid)
			httpReq.Header.Set(middleware.HeaderTraceID, tid)
		}

		start := time.Now()
		httpResp, err := provider.SharedDefaultHTTPClient.Do(httpReq)
		if err != nil {
			cancelCall()
			// A budget expiry surfaces as DeadlineExceeded on the attempt
			// context; classify it as ours rather than charging the channel.
			if ctxErr := ctx.Err(); ctxErr != nil {
				d.circuitBreaker.AbortCanary(ch.Name)
				if requestBudgetExceeded(ctx, ctxErr) {
					return nil, newRequestBudgetError(req.Model, append(attempted, ch.Name), err)
				}
				return nil, newCanceledError(ctx, attempted, ch.Name, model.EstimateTokensFromChars(len(req.Body)))
			}
			if errors.Is(err, context.Canceled) {
				d.circuitBreaker.AbortCanary(ch.Name)
				return nil, newCanceledError(ctx, attempted, ch.Name, model.EstimateTokensFromChars(len(req.Body)))
			}
			lastErr = err
			// 404 is node-specific: another candidate may host the model (B-03).
			if provider.IsModelNotFound(err) {
				d.circuitBreaker.AbortCanary(ch.Name)
				attempted = append(attempted, ch.Name)
				continue
			}
			if provider.IsFatalPayloadError(err) {
				d.circuitBreaker.AbortCanary(ch.Name)
				return nil, err
			}
			if !provider.IsChannelFailure(err) {
				d.circuitBreaker.AbortCanary(ch.Name)
				attempted = append(attempted, ch.Name)
				continue
			}
			attempted = append(attempted, d.recordChannelFailure(ctx, ch.Name, err, i+1))
			continue
		}

		// Upstream status classes follow the same contract as the chat path:
		// 401/403/408/5xx mean the channel is unhealthy, while 429 is backpressure
		// from a healthy provider. The previous ad-hoc list charged 429 to the
		// breaker, so three rate-limit replies opened the circuit and turned
		// backpressure into a 502 blackout for every image/audio/video request.
		if status := httpResp.StatusCode; status == http.StatusTooManyRequests || status == http.StatusUnauthorized ||
			status == http.StatusForbidden || status == http.StatusRequestTimeout || status >= http.StatusInternalServerError {
			bodyBytes, _ := io.ReadAll(io.LimitReader(httpResp.Body, 64<<10))
			httpResp.Body.Close()
			cancelCall()
			lastErr = provider.NewUpstreamError(ch.Name, status, bodyBytes)
			if provider.IsChannelFailure(lastErr) {
				attempted = append(attempted, d.recordChannelFailure(ctx, ch.Name, lastErr, i+1))
			} else {
				// Release the half-open canary slot and fail over without charging
				// the breaker for the provider's rate limit.
				d.circuitBreaker.AbortCanary(ch.Name)
				attempted = append(attempted, ch.Name)
				routerLog(ctx).Warn("upstream rate limited the modality request, trying next channel without tripping breaker",
					"channel", ch.Name, "status", status, "attempt", i+1)
			}
			continue
		}

		// A 404 means this node does not host the model, so the next candidate
		// channel may still serve it; fanning out is exactly what the chat path
		// does. A deterministic payload error (400/413/422) would fail identically
		// everywhere: return the upstream error body so the caller keeps the
		// diagnostic detail. Other 4xx statuses keep the historic pass-through.
		if httpResp.StatusCode >= 400 && httpResp.StatusCode < 500 {
			bodyBytes, _ := io.ReadAll(io.LimitReader(httpResp.Body, 1<<20))
			httpResp.Body.Close()
			cancelCall()
			d.circuitBreaker.AbortCanary(ch.Name)

			if httpResp.StatusCode == http.StatusNotFound {
				lastErr = provider.NewUpstreamError(ch.Name, httpResp.StatusCode, bodyBytes)
				attempted = append(attempted, ch.Name)
				continue
			}

			// The buffered body must be replayed with a matching Content-Length;
			// the original header would be stale once the body was truncated.
			headers := httpResp.Header.Clone()
			headers.Del("Content-Length")
			headers.Del("Transfer-Encoding")
			headers.Del("Content-Encoding")
			headers.Set("Content-Length", strconv.Itoa(len(bodyBytes)))
			return &UpstreamResponse{
				StatusCode:   httpResp.StatusCode,
				ProviderName: ch.Name, ChannelID: ch.ID, BaseURL: ch.BaseURL,
				Headers: headers,
				Stream:  io.NopCloser(bytes.NewReader(bodyBytes)),
				Channel: formatChannelTrail(ctx, attempted, ch.Name),
			}, nil
		}

		// Success! Mark circuit breaker healthy and record metrics
		trail := d.recordChannelSuccess(ctx, attempted, ch.Name, time.Since(start), 0, 0)

		return &UpstreamResponse{
			StatusCode:   httpResp.StatusCode,
			ProviderName: ch.Name, ChannelID: ch.ID, BaseURL: ch.BaseURL,
			Headers: httpResp.Header,
			Stream:  &cancelOnCloseReadCloser{ReadCloser: httpResp.Body, cancel: cancelCall},
			Channel: trail,
		}, nil
	}

	if lastErr == nil {
		lastErr = skippedOnlyError(req.Model, skipped)
	}
	return nil, newExhaustedError(ctx, req.Model, exhaustedTrail(attempted, skipped), fmt.Errorf("all %d providers failed for model '%s'. Last error: %w", len(channels), req.Model, lastErr))
}

// dispatchEmbeddingOnce routes one embedding request across matching candidate
// channels. The exported DispatchEmbedding wrapper adds the total request budget
// and the cross-model fallback.
func (d *Dispatcher) dispatchEmbeddingOnce(ctx context.Context, req *model.EmbeddingRequest) (*model.EmbeddingResponse, error) {
	channels := d.ResolveCandidateChannels(ctx, req.Model, "embeddings")
	if len(channels) == 0 {
		return nil, newNoRouteError(ctx, req.Model, fmt.Errorf("no upstream provider available for embedding model '%s'", req.Model))
	}

	var lastErr error
	var attempted []string // channels actually called and failed
	var skipped []string   // channels bypassed by the circuit breaker, never called
	for i, ch := range channels {
		// Stop at the gateway's total budget instead of paying the next channel
		// timeout as well (B-01).
		if ctxErr := ctx.Err(); ctxErr != nil {
			if requestBudgetExceeded(ctx, ctxErr) {
				return nil, newRequestBudgetError(req.Model, attempted, lastErr)
			}
			return nil, newCanceledError(ctx, attempted, "", 0)
		}
		if !d.circuitBreaker.CanExecute(ch.Name) {
			telemetry.WarnLogger(routerLog(ctx), "breaker-open:"+ch.Name, 30*time.Second,
				"circuit breaker is OPEN, bypassing dead upstream",
				"channel", ch.Name, "attempt", i+1)
			skipped = append(skipped, ch.Name)
			continue
		}

		start := time.Now()
		var resp *model.EmbeddingResponse
		var err error

		d.mu.RLock()
		prov, exists := d.providers[ch.Type]
		d.mu.RUnlock()

		if !exists {
			lastErr = fmt.Errorf("unsupported provider type '%s' on channel %s", ch.Type, ch.Name)
			d.circuitBreaker.AbortCanary(ch.Name)
			continue
		}

		embedProv, ok := prov.(provider.EmbeddingProvider)
		if !ok {
			lastErr = fmt.Errorf("provider '%s' on channel %s does not support embedding", ch.Type, ch.Name)
			d.circuitBreaker.AbortCanary(ch.Name)
			continue
		}
		callCtx, cancelCall := channelCallContext(ctx, ch)
		resp, err = embedProv.Embed(callCtx, req, ch)
		cancelCall()

		if err != nil {
			if ctx.Err() == context.Canceled || errors.Is(err, context.Canceled) {
				d.circuitBreaker.AbortCanary(ch.Name)
				return nil, newCanceledError(ctx, attempted, ch.Name, req.EstimatedPromptTokens())
			}
			// A budget expiry arrives as DeadlineExceeded on the attempt context.
			if ctxErr := ctx.Err(); ctxErr != nil {
				d.circuitBreaker.AbortCanary(ch.Name)
				if requestBudgetExceeded(ctx, ctxErr) {
					return nil, newRequestBudgetError(req.Model, append(attempted, ch.Name), err)
				}
				return nil, newCanceledError(ctx, attempted, ch.Name, req.EstimatedPromptTokens())
			}
			lastErr = err
			// 404 is node-specific: the next candidate channel may host the model
			// (B-03). Only a deterministic payload error ends the fan-out.
			if provider.IsModelNotFound(err) {
				d.circuitBreaker.AbortCanary(ch.Name)
				attempted = append(attempted, ch.Name)
				continue
			}
			if provider.IsFatalPayloadError(err) {
				d.circuitBreaker.AbortCanary(ch.Name)
				return nil, err
			}
			if !provider.IsChannelFailure(err) {
				d.circuitBreaker.AbortCanary(ch.Name)
				attempted = append(attempted, ch.Name)
				continue
			}
			attempted = append(attempted, d.recordChannelFailure(ctx, ch.Name, err, i+1))
			continue
		}

		promptTokens := 0
		if resp.Usage.PromptTokens > 0 {
			promptTokens = resp.Usage.PromptTokens
		}
		resp.Channel = d.recordChannelSuccess(ctx, attempted, ch.Name, time.Since(start), promptTokens, 0)
		return resp, nil
	}

	if lastErr == nil {
		lastErr = skippedOnlyError(req.Model, skipped)
	}
	return nil, newExhaustedError(ctx, req.Model, exhaustedTrail(attempted, skipped), fmt.Errorf("all %d providers failed for embedding model '%s'. Last error: %w", len(channels), req.Model, lastErr))
}

// dispatchRerankOnce routes one rerank request across candidate channels. The
// exported DispatchRerank wrapper adds the total request budget and the
// cross-model fallback.
func (d *Dispatcher) dispatchRerankOnce(ctx context.Context, req *model.RerankRequest) (*model.RerankResponse, error) {
	channels := d.ResolveCandidateChannels(ctx, req.Model, "rerank")
	if len(channels) == 0 {
		return nil, newNoRouteError(ctx, req.Model, fmt.Errorf("no upstream provider available for rerank model '%s'", req.Model))
	}

	var lastErr error
	var attempted []string // channels actually called and failed
	var skipped []string   // channels bypassed by the circuit breaker, never called
	for i, ch := range channels {
		// Stop at the gateway's total budget instead of paying the next channel
		// timeout as well (B-01).
		if ctxErr := ctx.Err(); ctxErr != nil {
			if requestBudgetExceeded(ctx, ctxErr) {
				return nil, newRequestBudgetError(req.Model, attempted, lastErr)
			}
			return nil, newCanceledError(ctx, attempted, "", 0)
		}
		if !d.circuitBreaker.CanExecute(ch.Name) {
			telemetry.WarnLogger(routerLog(ctx), "breaker-open:"+ch.Name, 30*time.Second,
				"circuit breaker is OPEN, bypassing dead upstream",
				"channel", ch.Name, "attempt", i+1)
			skipped = append(skipped, ch.Name)
			continue
		}

		start := time.Now()
		d.mu.RLock()
		prov, exists := d.providers[ch.Type]
		d.mu.RUnlock()

		if !exists {
			lastErr = fmt.Errorf("unsupported provider type '%s' on channel %s", ch.Type, ch.Name)
			d.circuitBreaker.AbortCanary(ch.Name)
			continue
		}

		rerankProv, ok := prov.(provider.RerankProvider)
		if !ok {
			lastErr = fmt.Errorf("provider '%s' on channel %s does not support reranking", ch.Type, ch.Name)
			d.circuitBreaker.AbortCanary(ch.Name)
			continue
		}

		callCtx, cancelCall := channelCallContext(ctx, ch)
		resp, err := rerankProv.Rerank(callCtx, req, ch)
		cancelCall()
		if err != nil {
			if ctx.Err() == context.Canceled || errors.Is(err, context.Canceled) {
				d.circuitBreaker.AbortCanary(ch.Name)
				return nil, newCanceledError(ctx, attempted, ch.Name, req.EstimatedPromptTokens())
			}
			// A budget expiry arrives as DeadlineExceeded on the attempt context.
			if ctxErr := ctx.Err(); ctxErr != nil {
				d.circuitBreaker.AbortCanary(ch.Name)
				if requestBudgetExceeded(ctx, ctxErr) {
					return nil, newRequestBudgetError(req.Model, append(attempted, ch.Name), err)
				}
				return nil, newCanceledError(ctx, attempted, ch.Name, req.EstimatedPromptTokens())
			}
			lastErr = err
			// 404 is node-specific: the next candidate channel may host the model
			// (B-03). Only a deterministic payload error ends the fan-out.
			if provider.IsModelNotFound(err) {
				d.circuitBreaker.AbortCanary(ch.Name)
				attempted = append(attempted, ch.Name)
				continue
			}
			if provider.IsFatalPayloadError(err) {
				d.circuitBreaker.AbortCanary(ch.Name)
				return nil, err
			}
			if !provider.IsChannelFailure(err) {
				d.circuitBreaker.AbortCanary(ch.Name)
				attempted = append(attempted, ch.Name)
				continue
			}
			attempted = append(attempted, d.recordChannelFailure(ctx, ch.Name, err, i+1))
			continue
		}

		totalTokens := 0
		if resp.Usage.TotalTokens > 0 {
			totalTokens = resp.Usage.TotalTokens
		}
		resp.Channel = d.recordChannelSuccess(ctx, attempted, ch.Name, time.Since(start), totalTokens, 0)
		return resp, nil
	}

	if lastErr == nil {
		lastErr = skippedOnlyError(req.Model, skipped)
	}
	return nil, newExhaustedError(ctx, req.Model, exhaustedTrail(attempted, skipped), fmt.Errorf("all %d providers failed for rerank model '%s'. Last error: %w", len(channels), req.Model, lastErr))
}
