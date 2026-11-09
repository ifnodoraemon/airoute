package adapter

import (
	"github.com/ifnodoraemon/airoute/internal/model"
)

// AnthropicToCanonical converts a typed Anthropic request into a chat request.
func AnthropicToCanonical(req *model.AnthropicInboundRequest) (*model.ChatCompletionRequest, error) {

	canonicalMsgs := anthropicCanonicalMessages(req)

	var maxTokens *int
	if req.MaxTokens > 0 {
		maxTokens = &req.MaxTokens
	}

	canonicalTools := anthropicCanonicalTools(req)

	var stop any
	if len(req.StopSequences) > 0 {
		stop = req.StopSequences
	}

	// OpenAI-compatible upstreams only emit the final usage chunk when the
	// request opts in via stream_options.include_usage. Without it, streaming
	// logs end up with zero prompt/cached tokens even though the upstream knows
	// the real numbers.
	var streamOptions *model.StreamOptions
	if req.Stream {
		streamOptions = &model.StreamOptions{IncludeUsage: true}
	}

	return &model.ChatCompletionRequest{
		Model:               req.Model,
		Messages:            canonicalMsgs,
		Temperature:         req.Temperature,
		TopP:                req.TopP,
		MaxTokens:           maxTokens,
		Stream:              req.Stream,
		StreamOptions:       streamOptions,
		Tools:               canonicalTools,
		ToolChoice:          convertAnthropicToolChoice(req.ToolChoice),
		Stop:                stop,
		Protocol:            "anthropic_messages",
		AnthropicOriginal:   req,
		AnthropicNativeOnly: requiresNativeAnthropic(req),
	}, nil
}

func anthropicCanonicalMessages(req *model.AnthropicInboundRequest) []model.ChatMessage {
	var messages []model.ChatMessage
	if system := req.GetSystemPrompt(); system != "" {
		messages = append(messages, model.ChatMessage{Role: "system", Content: system})
	}
	for _, message := range req.Messages {
		messages = append(messages, anthropicMessageToCanonical(message)...)
	}
	return messages
}
