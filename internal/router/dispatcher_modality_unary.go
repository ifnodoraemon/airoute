package router

import (
	"context"
	"fmt"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

type unaryProviderCall[T any] func(context.Context, *model.ChannelConfig) (*T, error)
type unaryModality[T any] struct {
	protocol, label, capability string
	bind                        func(provider.Provider) (unaryProviderCall[T], bool)
	tokens                      func(*T) int
}

func (d *Dispatcher) dispatchEmbeddingOnce(ctx context.Context, req *model.EmbeddingRequest) (*model.EmbeddingResponse, error) {
	resp, channel, err := dispatchUnaryModality(d, ctx, req.Model, req.EstimatedPromptTokens, unaryModality[model.EmbeddingResponse]{
		protocol: "embeddings", label: "embedding", capability: "embedding",
		bind: func(p provider.Provider) (unaryProviderCall[model.EmbeddingResponse], bool) {
			embedding, ok := p.(provider.EmbeddingProvider)
			if !ok {
				return nil, false
			}
			return func(ctx context.Context, ch *model.ChannelConfig) (*model.EmbeddingResponse, error) {
				return embedding.Embed(ctx, req, ch)
			}, true
		},
		tokens: func(resp *model.EmbeddingResponse) int { return resp.Usage.PromptTokens },
	})
	if err == nil {
		resp.Channel = channel
	}
	return resp, err
}

func (d *Dispatcher) dispatchRerankOnce(ctx context.Context, req *model.RerankRequest) (*model.RerankResponse, error) {
	resp, channel, err := dispatchUnaryModality(d, ctx, req.Model, req.EstimatedPromptTokens, unaryModality[model.RerankResponse]{
		protocol: "rerank", label: "rerank", capability: "reranking",
		bind: func(p provider.Provider) (unaryProviderCall[model.RerankResponse], bool) {
			rerank, ok := p.(provider.RerankProvider)
			if !ok {
				return nil, false
			}
			return func(ctx context.Context, ch *model.ChannelConfig) (*model.RerankResponse, error) {
				return rerank.Rerank(ctx, req, ch)
			}, true
		},
		tokens: func(resp *model.RerankResponse) int { return resp.Usage.TotalTokens },
	})
	if err == nil {
		resp.Channel = channel
	}
	return resp, err
}

// dispatchUnaryModality shares only fully consumed unary responses. Streaming
// and raw HTTP bodies retain their own ownership and cancellation lifetimes.
func dispatchUnaryModality[T any](d *Dispatcher, ctx context.Context, name string, estimate func() int, modality unaryModality[T]) (*T, string, error) {
	channels := d.ResolveCandidateChannels(ctx, name, modality.protocol)
	if len(channels) == 0 {
		return nil, "", newNoRouteError(ctx, name, fmt.Errorf("no upstream provider available for %s model '%s'", modality.label, name))
	}
	state := unaryAttemptState{dispatcher: d, ctx: ctx, model: name, estimate: estimate}
	for i, ch := range channels {
		available, err := state.available(ch, i+1)
		if err != nil {
			return nil, "", err
		}
		if !available {
			continue
		}
		start := time.Now()
		d.mu.RLock()
		prov, exists := d.providers[ch.Type]
		d.mu.RUnlock()
		if !exists {
			state.unsupported(ch, fmt.Errorf("unsupported provider type '%s' on channel %s", ch.Type, ch.Name))
			continue
		}
		call, supported := modality.bind(prov)
		if !supported {
			state.unsupported(ch, fmt.Errorf("provider '%s' on channel %s does not support %s", ch.Type, ch.Name, modality.capability))
			continue
		}
		callCtx, cancelCall, err := d.beginChannelCall(ctx, ch, name)
		if err != nil {
			if fatal := state.admissionFailure(ch, err); fatal != nil {
				return nil, "", fatal
			}
			continue
		}
		resp, err := call(callCtx, ch)
		cancelCall()
		if err != nil {
			if fatal := state.failure(ch, err, i+1); fatal != nil {
				return nil, "", fatal
			}
			continue
		}
		d.rememberSessionChannel(ctx, name, ch.Name)
		channel := d.recordChannelSuccess(ctx, state.attempted, ch.Name, time.Since(start), max(0, modality.tokens(resp)), 0)
		return resp, channel, nil
	}
	if state.lastErr == nil {
		state.lastErr = skippedOnlyError(name, state.skipped)
	}
	err := fmt.Errorf("all %d providers failed for %s model '%s'. Last error: %w", len(channels), modality.label, name, state.lastErr)
	return nil, "", newExhaustedError(ctx, name, exhaustedTrail(state.attempted, state.skipped), err)
}
