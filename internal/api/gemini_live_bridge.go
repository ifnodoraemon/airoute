package api

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/nativestate"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func (h *Handler) bridgeGoogleLive(c *gin.Context, ctx context.Context, client, upstream *websocket.Conn, ch *model.ChannelConfig, name string, meter *liveQuotaMeter) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	start := time.Now()
	done := make(chan struct{}, 2)
	quotaFailure := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer func() { done <- struct{}{} }()
		for {
			kind, data, err := client.ReadMessage()
			if err != nil {
				return
			}
			if err := meter.admitInput(ctx, data); err != nil {
				quotaFailure <- err
				return
			}
			if upstream.WriteMessage(kind, data) != nil {
				return
			}
		}
	}()
	stats := &googleLiveUsage{}
	go func() {
		defer wg.Done()
		defer func() { done <- struct{}{} }()
		observed := false
		for {
			kind, data, err := upstream.ReadMessage()
			if err != nil {
				return
			}
			stats.observe(data)
			if err := meter.observe(ctx, stats); err != nil {
				quotaFailure <- err
				return
			}
			if !observed && stats.accepted && stats.failure == nil {
				h.dispatcher.ObserveNativeSession(ctx, ch, name, nil)
				observed = true
			}
			if stats.resume != "" && callerOwner(c) != "" {
				if err := nativestate.Save(ctx, storage.NativeState{Namespace: "gemini-live", ID: stats.resume, Owner: callerOwner(c),
					Identity: model.UpstreamIdentity(ch), Model: name, Data: []byte(`{}`), ExpiresAt: time.Now().Add(24 * time.Hour)}); err != nil {
					return
				}
				stats.resume = ""
			}
			if client.WriteMessage(kind, data) != nil {
				return
			}
		}
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
	cancel()
	select {
	case <-quotaFailure:
		_ = client.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "Billing limit reached"), time.Now().Add(time.Second))
	default:
	}
	_ = client.Close()
	_ = upstream.Close()
	// Both readers must stop before the stats are read and the handler returns.
	wg.Wait()
	stats.finishTurn()
	status := h.observeGoogleLiveResult(ctx, ch, name, stats)
	recordGoogleLiveUsage(c, ch, name, stats, start, status)
}

type googleLiveUsage struct {
	accepted                                         bool
	failure                                          error
	input, output, cached, prompt, completion, cache int
	resume                                           string
}

func (s *googleLiveUsage) observe(data []byte) {
	var message struct {
		Setup json.RawMessage `json:"setupComplete"`
		Error json.RawMessage `json:"error"`
		Usage struct {
			Prompt  int `json:"promptTokenCount"`
			Output  int `json:"responseTokenCount"`
			Thought int `json:"thoughtsTokenCount"`
			Cached  int `json:"cachedContentTokenCount"`
		} `json:"usageMetadata"`
		Content struct {
			Done bool `json:"turnComplete"`
		} `json:"serverContent"`
		Resume struct {
			Handle string `json:"newHandle"`
		} `json:"sessionResumptionUpdate"`
	}
	_ = json.Unmarshal(provider.CanonicalGeminiJSON(data), &message)
	if len(message.Setup) > 0 {
		s.accepted = true
	}
	if len(message.Error) > 0 && string(message.Error) != "null" {
		var failure struct {
			Code int `json:"code"`
		}
		_ = json.Unmarshal(message.Error, &failure)
		if failure.Code < 400 {
			failure.Code = 502
		}
		s.failure = provider.NewUpstreamError("", failure.Code, message.Error)
	}
	s.prompt = max(s.prompt, message.Usage.Prompt)
	s.completion = max(s.completion, message.Usage.Output+message.Usage.Thought)
	s.cache = max(s.cache, message.Usage.Cached)
	s.resume = message.Resume.Handle
	if message.Content.Done {
		s.finishTurn()
	}
}

func (s *googleLiveUsage) finishTurn() {
	s.input += s.prompt
	s.output += s.completion
	s.cached += s.cache
	s.prompt, s.completion, s.cache = 0, 0, 0
}

func recordGoogleLiveUsage(c *gin.Context, ch *model.ChannelConfig, name string, stats *googleLiveUsage, start time.Time, status int) {
	RecordUsage(c, AuditRecordParams{ChatID: "live_" + nativestate.NewID(), Model: name, Channel: ch.Name,
		SessionID: resolveGenericSessionID(c, "live"), PromptTokens: stats.input, CompletionTokens: stats.output, CachedTokens: stats.cached,
		TotalTokens: stats.input + stats.output, Duration: time.Since(start), StatusCode: status, UsageSource: usageSourceUpstream, ErrorMessage: sanitizeErrorMessage(stats.failure)})
}

func (h *Handler) observeGoogleLiveResult(ctx context.Context, ch *model.ChannelConfig, name string, stats *googleLiveUsage) int {
	if stats.failure != nil {
		h.dispatcher.ObserveNativeSession(ctx, ch, name, stats.failure)
		return 502
	}
	return 200
}
