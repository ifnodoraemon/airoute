package provider

import (
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// buildCompletionURL joins the configured API root with /completions using the
// shared APIURL semantics. An operator who pasted the chat endpoint into the base
// URL field gets the text-completion endpoint rather than a doubled path.
func buildCompletionURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	base = strings.TrimSuffix(base, "/chat/completions")
	return APIURL(base, "/v1/completions")
}

// ErrTextAdapterUnsupported is returned when a request needs capabilities the
// legacy /v1/completions protocol cannot express (tool calling, multimodal
// input). It is deliberately NOT an *UpstreamError: the channel is healthy, the
// protocol simply cannot carry the request, so the router should try another
// channel and only surface this message if none can serve it.
type ErrTextAdapterUnsupported struct {
	Reason string
}

func (e *ErrTextAdapterUnsupported) Error() string {
	return "text-completion channel cannot serve this request: " + e.Reason
}

// requiresChatCapabilities reports whether the request needs a chat-capable
// upstream. Previously such requests were silently flattened into a plain
// prompt: tool definitions were dropped (so the model could never call a tool)
// and image parts were serialised as base64 text into the prompt, which blew up
// the token count and usually overflowed the context window.
func requiresChatCapabilities(req *model.ChatCompletionRequest) string {
	if req == nil {
		return ""
	}
	if len(req.Tools) > 0 {
		return "request declares tools, which /v1/completions cannot express"
	}
	if tc, ok := req.ToolChoice.(string); ok && tc != "" && tc != "none" {
		return "request sets tool_choice, which /v1/completions cannot express"
	}
	for _, m := range req.Messages {
		parts := model.ParseMessageContent(m.Content)
		for _, part := range parts {
			switch part.Type {
			case model.ContentPartImageURL, model.ContentPartInputAudio:
				return "request contains non-text content, which /v1/completions cannot express"
			}
		}
	}
	return ""
}

func convertChatToPrompt(messages []model.ChatMessage) string {
	var sb strings.Builder
	for _, m := range messages {
		role := strings.ToLower(m.Role)
		content := m.GetContentString()
		switch role {
		case "system":
			sb.WriteString("System: " + content + "\n")
		case "user":
			sb.WriteString("User: " + content + "\n")
		case "assistant":
			sb.WriteString("Assistant: " + content + "\n")
		default:
			sb.WriteString(content + "\n")
		}
	}
	sb.WriteString("Assistant: ")
	return sb.String()
}
