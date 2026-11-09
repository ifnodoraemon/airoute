package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

type liveQuotaMeter struct {
	mu                                sync.Mutex
	quota                             *middleware.RequestQuota
	name, group                       string
	cap, bytes, prompt, output, turns int
}

func prepareLiveQuota(c *gin.Context, name string, wire []byte) ([]byte, *liveQuotaMeter, error) {
	quota := middleware.RequestQuotaFrom(c)
	if quota == nil {
		return wire, nil, nil
	}
	if billing.GlobalEngine == nil {
		return nil, nil, fmt.Errorf("billing prices are unavailable")
	}
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(provider.CanonicalGeminiJSON(wire), &outer); err != nil {
		return nil, nil, err
	}
	req := &model.ChatCompletionRequest{Model: name, Native: &model.NativeRequest{Protocol: model.NativeGemini, Body: outer["setup"]}}
	if err := boundPrepaidCompletion(req); err != nil {
		return nil, nil, err
	}
	outer["setup"] = req.Native.Body
	meter := &liveQuotaMeter{quota: quota, name: name, group: getKeyGroup(c), cap: *req.MaxTokens}
	if err := meter.reserve(c.Request.Context()); err != nil {
		return nil, nil, err
	}
	bounded, err := json.Marshal(outer)
	return bounded, meter, err
}

func (m *liveQuotaMeter) admitInput(ctx context.Context, data []byte) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var message struct {
		Setup   json.RawMessage `json:"setup"`
		Content struct {
			Done bool `json:"turnComplete"`
		} `json:"clientContent"`
	}
	if err := json.Unmarshal(data, &message); err != nil {
		return err
	}
	if len(message.Setup) > 0 {
		return fmt.Errorf("Live setup cannot be replaced during a prepaid session")
	}
	if len(data) > math.MaxInt-m.bytes {
		return fmt.Errorf("Live input exceeds billing limits")
	}
	m.bytes += len(data)
	if message.Content.Done {
		m.turns++
	}
	return m.reserve(ctx)
}

func (m *liveQuotaMeter) observe(ctx context.Context, stats *googleLiveUsage) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if stats.output > m.output && m.turns > 0 {
		m.turns--
	}
	m.prompt = stats.input + stats.prompt
	m.output = stats.output + stats.completion
	return m.reserve(ctx)
}

func (m *liveQuotaMeter) reserve(ctx context.Context) error {
	count := max(1, m.turns)
	if m.cap > (math.MaxInt-m.output)/count {
		return fmt.Errorf("Live output exceeds billing limits")
	}
	cost := estimateQuotaCost(m.name, m.group, max(m.bytes, m.prompt), m.output+m.cap*count, 1)
	return m.quota.Resize(ctx, cost)
}
