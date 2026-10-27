package router

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

var ErrRoutingProbeTarget = errors.New("channel is not active for this model")

type RoutingProbe struct {
	ChannelID int64     `json:"channel_id"`
	Model     string    `json:"model"`
	Success   bool      `json:"success"`
	LatencyMs int64     `json:"latency_ms"`
	CheckedAt time.Time `json:"checked_at"`
	Error     string    `json:"error,omitempty"`
}

// ValidateModelChannel sends one bounded inference directly to the selected
// runtime channel. It does not exercise fallback or alter breaker/SWRR state.
func (d *Dispatcher) ValidateModelChannel(ctx context.Context, modelName string, id int64) (*RoutingProbe, error) {
	if InferModality(modelName) != "chat" {
		return nil, errors.New("仅支持对话模型验证，请在调试演练中验证其他模型")
	}
	for _, channel := range d.ListChannelsForModel(modelName, "") {
		if channel.ID != id {
			continue
		}
		limit := 8
		req := &model.ChatCompletionRequest{Model: modelName, MaxTokens: &limit,
			Messages: []model.ChatMessage{{Role: "user", Content: "Reply OK."}}}
		prov, ok := d.getProviderForChannel(channel)
		if !ok {
			return nil, errors.New("渠道类型没有对应的推理适配器")
		}
		start := time.Now()
		response, err := prov.ChatComplete(ctx, req, channel)
		result := &RoutingProbe{ChannelID: id, Model: modelName, LatencyMs: time.Since(start).Milliseconds(), CheckedAt: time.Now()}
		if err != nil {
			result.Error = "上游模型调用失败，请检查渠道配置或运行日志"
			var upstream *provider.UpstreamError
			if errors.As(err, &upstream) {
				result.Error = fmt.Sprintf("上游返回 HTTP %d", upstream.Status)
			}
			if errors.Is(err, context.DeadlineExceeded) {
				result.Error = "模型验证超时（15 秒）"
			}
			return result, nil
		}
		if response != nil {
			for _, choice := range response.Choices {
				if choice.Message.GetContentString() != "" || choice.Message.ReasoningContent != "" || len(choice.Message.ToolCalls) > 0 {
					result.Success = true
				}
			}
		}
		if !result.Success {
			result.Error = "上游没有返回有效生成内容"
		}
		return result, nil
	}
	return nil, ErrRoutingProbeTarget
}
