package router

// The two unary modality paths (embedding and rerank) live here rather than in
// dispatcher_modality.go, which keeps the generic JSON/HTTP path readable and
// both files under the repository's file-size guideline. The loops are
// intentionally parallel: they share the budget check, the breaker admission and
// the 404-vs-fatal-payload classification, and keeping them side by side makes a
// divergence visible at review time.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

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
