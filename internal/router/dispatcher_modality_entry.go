package router

import (
	"context"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// =============================================================================
// Entry points for the non-streaming modality paths (B-01 budget, B-11 fallback)
//
// The chat paths have always (a) bounded the total time a request may spend across
// channel attempts with GATEWAY_REQUEST_BUDGET_SEC and (b) retried the request on
// the configured fallback_model when routing found no candidate or every candidate
// failed. Embedding, rerank and the generic HTTP path silently skipped both, so a
// multi-channel modality request could wait for every candidate to time out and a
// configured fallback chain was never used (yet the topology view still drew it).
// These wrappers give the three paths the same two rules; the "*Once" functions
// below them keep the pure attempt loop.
// =============================================================================

// DispatchEmbedding routes an embedding request: total request budget plus
// cross-model fallback around one attempt pass.
func (d *Dispatcher) DispatchEmbedding(ctx context.Context, req *model.EmbeddingRequest) (*model.EmbeddingResponse, error) {
	ctx, cancelBudget := withRequestBudget(ctx)
	defer cancelBudget()
	return dispatchModalityWithFallback(d, ctx, req.Model, func(ctx context.Context, modelName string) (*model.EmbeddingResponse, error) {
		copied := *req
		copied.Model = modelName
		return d.dispatchEmbeddingOnce(ctx, &copied)
	})
}

// DispatchRerank routes a rerank request: total request budget plus cross-model
// fallback around one attempt pass.
func (d *Dispatcher) DispatchRerank(ctx context.Context, req *model.RerankRequest) (*model.RerankResponse, error) {
	ctx, cancelBudget := withRequestBudget(ctx)
	defer cancelBudget()
	return dispatchModalityWithFallback(d, ctx, req.Model, func(ctx context.Context, modelName string) (*model.RerankResponse, error) {
		copied := *req
		copied.Model = modelName
		return d.dispatchRerankOnce(ctx, &copied)
	})
}

// DispatchHTTP routes a generic modality HTTP request (images, audio, video,
// moderation...): total request budget plus cross-model fallback around one
// attempt pass.
//
// The budget cancel must NOT run when DispatchHTTP returns on success: the
// response body is streamed to the client afterwards and its request context is
// derived from the budget context. It is therefore chained into the same
// close-time release as the channel timeout (cancelOnCloseReadCloser), which is
// exactly when the caller is done with the body.
func (d *Dispatcher) DispatchHTTP(ctx context.Context, req *UpstreamRequest) (*UpstreamResponse, error) {
	budgetCtx, cancelBudget := withRequestBudget(ctx)
	released := false
	defer func() {
		if !released {
			cancelBudget()
		}
	}()

	attempt := func(ctx context.Context, modelName string) (*UpstreamResponse, error) {
		copied := *req
		copied.Model = modelName
		return d.dispatchHTTPOnce(ctx, &copied)
	}

	var resp *UpstreamResponse
	var err error
	if req.PinnedChannel != "" {
		// A pinned channel is an operator override: no routing, no fallback.
		resp, err = attempt(budgetCtx, req.Model)
	} else {
		resp, err = dispatchModalityWithFallback(d, budgetCtx, req.Model, attempt)
	}
	if err != nil {
		return nil, err
	}
	if resp != nil && resp.Stream != nil {
		resp.Stream = &cancelOnCloseReadCloser{ReadCloser: resp.Stream, cancel: cancelBudget}
		released = true
	}
	return resp, nil
}

// dispatchModalityWithFallback runs attempt for modelName and, when routing had
// no candidate or every candidate failed, retries against the configured fallback
// model — recursively, so a configured chain (a -> b -> c) is followed exactly
// like the chat paths do. nextFallbackContext carries the chain in the context and
// enforces the same cycle/depth guard; the budget context travels with it, so a
// fallback can never extend the request past GATEWAY_REQUEST_BUDGET_SEC.
func dispatchModalityWithFallback[T any](d *Dispatcher, ctx context.Context, modelName string, attempt func(context.Context, string) (T, error)) (T, error) {
	result, err := attempt(ctx, modelName)
	if err == nil || !modalityFallbackEligible(err) {
		return result, err
	}
	fallback := d.GetModelFallback(modelName)
	if fallback == "" || strings.EqualFold(fallback, modelName) {
		return result, err
	}
	nextCtx, fbErr := nextFallbackContext(ctx, modelName, fallback)
	if fbErr != nil {
		routerLog(ctx).Warn("aborting fallback to prevent cycle or depth exhaustion",
			"requested_model", modelName, "fallback_model", fallback, "error", fbErr.Error())
		return result, err
	}
	routerLog(ctx).Warn("all candidate channels failed, triggering cross-model fallback",
		"requested_model", modelName,
		"fallback_model", fallback,
		"last_error", err,
	)
	return dispatchModalityWithFallback(d, nextCtx, fallback, attempt)
}

// modalityFallbackEligible reports whether err is a routing outcome a different
// model could satisfy: no candidate channel, or every candidate failed.
//
// Client cancellations are excluded (Canceled flag on DispatchError): replaying a
// canceled request on another model only adds upstream load for a client that is
// already gone. Fatal payload errors and budget/cancel errors are not
// *DispatchError values at all, so they never reach a fallback either.
func modalityFallbackEligible(err error) bool {
	dispatchErr, ok := AsDispatchError(err)
	if !ok {
		return false
	}
	return !dispatchErr.Canceled
}
