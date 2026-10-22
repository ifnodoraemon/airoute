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
	Path          string            // e.g. "/v1/images/generations", "/v1/audio/speech"
	Method        string            // "POST", "GET"
	Headers       map[string]string // custom headers
	Body          []byte            // serialized payload
	ContentType   string            // "application/json", "multipart/form-data"
	Model         string            // requested model for routing
	Protocol      string            // e.g. "images", "audio_speech", "videos"
	BillableUnits int               // fixed-price units; e.g. requested image count
}

// UpstreamResponse encapsulates a generic HTTP response across any modality.
type UpstreamResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
	Stream     io.ReadCloser
	// Channel is the routing chain that served this request (e.g. "A" or
	// "A→B" after fallback). Internal-only metadata for audit logging.
	Channel string
}

// DispatchHTTP routes an HTTP request across matching candidate channels with Circuit Breaking and Safe Fallback.
func (d *Dispatcher) DispatchHTTP(ctx context.Context, req *UpstreamRequest) (*UpstreamResponse, error) {
	channels := d.ResolveCandidateChannels(ctx, req.Model, req.Protocol)
	if len(channels) == 0 {
		return nil, newNoRouteError(ctx, req.Model, fmt.Errorf("no upstream provider available for model '%s' and protocol '%s'", req.Model, req.Protocol))
	}

	var lastErr error
	var attempted []string // channels actually called and failed
	var skipped []string   // channels bypassed by the circuit breaker, never called
	for i, ch := range channels {
		// Circuit Breaker: fail fast if upstream is down
		if !d.circuitBreaker.CanExecute(ch.Name) {
			telemetry.ScopedLogger("router").Warn("circuit breaker is OPEN, bypassing dead upstream", "channel", ch.Name)
			skipped = append(skipped, ch.Name)
			continue
		}

		targetModel := ch.GetUpstreamModel(req.Model)
		targetBody := req.Body
		if strings.Contains(req.ContentType, "application/json") && len(targetBody) > 0 {
			targetBody = RewriteJSONModel(targetBody, targetModel)
		}

		targetURL := strings.TrimRight(ch.BaseURL, "/") + req.Path
		httpReq, err := http.NewRequestWithContext(ctx, req.Method, targetURL, bytes.NewReader(targetBody))
		if err != nil {
			lastErr = err
			d.circuitBreaker.AbortCanary(ch.Name)
			continue
		}

		if req.ContentType != "" {
			httpReq.Header.Set("Content-Type", req.ContentType)
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
			if ctx.Err() == context.Canceled || errors.Is(err, context.Canceled) {
				d.circuitBreaker.AbortCanary(ch.Name)
				return nil, ctx.Err()
			}
			lastErr = err
			if provider.IsClientError(err) {
				d.circuitBreaker.AbortCanary(ch.Name)
				return nil, err
			}
			if !provider.IsChannelFailure(err) {
				d.circuitBreaker.AbortCanary(ch.Name)
				attempted = append(attempted, ch.Name)
				continue
			}
			attempted = append(attempted, d.recordChannelFailure(ch.Name, err, i+1))
			continue
		}

		// Check if response indicates server error or rate limit
		if httpResp.StatusCode == http.StatusTooManyRequests || httpResp.StatusCode >= http.StatusInternalServerError || httpResp.StatusCode == http.StatusUnauthorized || httpResp.StatusCode == http.StatusForbidden {
			bodyBytes, _ := io.ReadAll(io.LimitReader(httpResp.Body, 64<<10))
			httpResp.Body.Close()
			lastErr = provider.NewUpstreamError(ch.Name, httpResp.StatusCode, bodyBytes)
			attempted = append(attempted, d.recordChannelFailure(ch.Name, lastErr, i+1))
			continue
		}

		// Deterministic 4xx: the caller's request is at fault, replaying it on
		// another channel is pointless and would only add load. Return the
		// upstream error body so the caller keeps the diagnostic detail.
		if httpResp.StatusCode >= 400 && httpResp.StatusCode < 500 {
			bodyBytes, _ := io.ReadAll(io.LimitReader(httpResp.Body, 1<<20))
			httpResp.Body.Close()
			d.circuitBreaker.AbortCanary(ch.Name)

			// The buffered body must be replayed with a matching Content-Length;
			// the original header would be stale once the body was truncated.
			headers := httpResp.Header.Clone()
			headers.Del("Content-Length")
			headers.Del("Transfer-Encoding")
			headers.Del("Content-Encoding")
			headers.Set("Content-Length", strconv.Itoa(len(bodyBytes)))
			return &UpstreamResponse{
				StatusCode: httpResp.StatusCode,
				Headers:    headers,
				Stream:     io.NopCloser(bytes.NewReader(bodyBytes)),
				Channel:    formatChannelTrail(ctx, attempted, ch.Name),
			}, nil
		}

		// Success! Mark circuit breaker healthy and record metrics
		trail := d.recordChannelSuccess(ctx, attempted, ch.Name, time.Since(start), 0, 0)

		return &UpstreamResponse{
			StatusCode: httpResp.StatusCode,
			Headers:    httpResp.Header,
			Stream:     httpResp.Body,
			Channel:    trail,
		}, nil
	}

	telemetry.GlobalMetrics.RecordRequest(false, 0, 0, 0)
	if lastErr == nil {
		lastErr = skippedOnlyError(req.Model, skipped)
	}
	return nil, newExhaustedError(ctx, req.Model, attempted, fmt.Errorf("all %d providers failed for model '%s'. Last error: %w", len(channels), req.Model, lastErr))
}

// DispatchEmbedding routes an embedding request across matching candidate channels with Circuit Breaking and Safe Fallback.
func (d *Dispatcher) DispatchEmbedding(ctx context.Context, req *model.EmbeddingRequest) (*model.EmbeddingResponse, error) {
	channels := d.ResolveCandidateChannels(ctx, req.Model, "embeddings")
	if len(channels) == 0 {
		return nil, newNoRouteError(ctx, req.Model, fmt.Errorf("no upstream provider available for embedding model '%s'", req.Model))
	}

	var lastErr error
	var attempted []string // channels actually called and failed
	var skipped []string   // channels bypassed by the circuit breaker, never called
	for i, ch := range channels {
		if !d.circuitBreaker.CanExecute(ch.Name) {
			telemetry.ScopedLogger("router").Warn("circuit breaker is OPEN, bypassing dead upstream", "channel", ch.Name)
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
		resp, err = embedProv.Embed(ctx, req, ch)

		if err != nil {
			if ctx.Err() == context.Canceled || errors.Is(err, context.Canceled) {
				d.circuitBreaker.AbortCanary(ch.Name)
				return nil, ctx.Err()
			}
			lastErr = err
			if provider.IsClientError(err) {
				d.circuitBreaker.AbortCanary(ch.Name)
				return nil, err
			}
			if !provider.IsChannelFailure(err) {
				d.circuitBreaker.AbortCanary(ch.Name)
				attempted = append(attempted, ch.Name)
				continue
			}
			attempted = append(attempted, d.recordChannelFailure(ch.Name, err, i+1))
			continue
		}

		promptTokens := 0
		if resp.Usage.PromptTokens > 0 {
			promptTokens = resp.Usage.PromptTokens
		}
		resp.Channel = d.recordChannelSuccess(ctx, attempted, ch.Name, time.Since(start), promptTokens, 0)
		return resp, nil
	}

	telemetry.GlobalMetrics.RecordRequest(false, 0, 0, 0)
	if lastErr == nil {
		lastErr = skippedOnlyError(req.Model, skipped)
	}
	return nil, newExhaustedError(ctx, req.Model, attempted, fmt.Errorf("all %d providers failed for embedding model '%s'. Last error: %w", len(channels), req.Model, lastErr))
}

// DispatchRerank routes a cross-encoder rerank request across candidate channels with Circuit Breaking and Safe Fallback.
func (d *Dispatcher) DispatchRerank(ctx context.Context, req *model.RerankRequest) (*model.RerankResponse, error) {
	channels := d.ResolveCandidateChannels(ctx, req.Model, "rerank")
	if len(channels) == 0 {
		return nil, newNoRouteError(ctx, req.Model, fmt.Errorf("no upstream provider available for rerank model '%s'", req.Model))
	}

	var lastErr error
	var attempted []string // channels actually called and failed
	var skipped []string   // channels bypassed by the circuit breaker, never called
	for i, ch := range channels {
		if !d.circuitBreaker.CanExecute(ch.Name) {
			telemetry.ScopedLogger("router").Warn("circuit breaker is OPEN, bypassing dead upstream", "channel", ch.Name)
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

		resp, err := rerankProv.Rerank(ctx, req, ch)
		if err != nil {
			if ctx.Err() == context.Canceled || errors.Is(err, context.Canceled) {
				d.circuitBreaker.AbortCanary(ch.Name)
				return nil, ctx.Err()
			}
			lastErr = err
			if provider.IsClientError(err) {
				d.circuitBreaker.AbortCanary(ch.Name)
				return nil, err
			}
			if !provider.IsChannelFailure(err) {
				d.circuitBreaker.AbortCanary(ch.Name)
				attempted = append(attempted, ch.Name)
				continue
			}
			attempted = append(attempted, d.recordChannelFailure(ch.Name, err, i+1))
			continue
		}

		totalTokens := 0
		if resp.Usage.TotalTokens > 0 {
			totalTokens = resp.Usage.TotalTokens
		}
		resp.Channel = d.recordChannelSuccess(ctx, attempted, ch.Name, time.Since(start), totalTokens, 0)
		return resp, nil
	}

	telemetry.GlobalMetrics.RecordRequest(false, 0, 0, 0)
	if lastErr == nil {
		lastErr = skippedOnlyError(req.Model, skipped)
	}
	return nil, newExhaustedError(ctx, req.Model, attempted, fmt.Errorf("all %d providers failed for rerank model '%s'. Last error: %w", len(channels), req.Model, lastErr))
}
