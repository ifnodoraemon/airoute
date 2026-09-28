package validator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/model"
)

// ValidateAnthropic validates and sanitizes an AnthropicInboundRequest.
func ValidateAnthropic(req *model.AnthropicInboundRequest, rule *config.ModelValidationRule, level string) *ValidationResult {
	res := &ValidationResult{
		Level: level,
		Valid: true,
	}

	if level == LevelOff || req == nil {
		return res
	}

	// 1. Mandatory max_tokens check
	if req.MaxTokens <= 0 {
		if level == LevelStrict {
			res.Valid = false
			res.Error = &ValidationError{
				Protocol:   ProtocolAnthropic,
				Field:      "max_tokens",
				Message:    "max_tokens: field required and must be an integer greater than 0",
				StatusCode: http.StatusBadRequest,
			}
			return res
		}
		req.MaxTokens = 4096
		res.AddWarning("injected default max_tokens: 4096")
	}

	// 2. Mandatory messages check
	if len(req.Messages) == 0 {
		res.Valid = false
		res.Error = &ValidationError{
			Protocol:   ProtocolAnthropic,
			Field:      "messages",
			Message:    "messages: at least one message is required",
			StatusCode: http.StatusBadRequest,
		}
		return res
	}

	// 3. System prompt check (cannot be inside messages array)
	var filteredMsgs []model.AnthropicInboundMessage
	for i, msg := range req.Messages {
		roleLower := strings.ToLower(strings.TrimSpace(msg.Role))
		if roleLower == "system" {
			if level == LevelStrict {
				res.Valid = false
				res.Error = &ValidationError{
					Protocol:   ProtocolAnthropic,
					Field:      fmt.Sprintf("messages[%d]", i),
					Message:    "System prompt must be specified in the top-level 'system' parameter, not in 'messages'",
					StatusCode: http.StatusBadRequest,
				}
				return res
			}
			sysContent := extractStringContent(msg.Content)
			if req.System == "" {
				req.System = sysContent
			} else {
				req.System += "\n\n" + sysContent
			}
			res.AddWarning("moved system prompt from messages to top-level 'system' parameter")
			continue
		}
		filteredMsgs = append(filteredMsgs, msg)
	}

	if len(filteredMsgs) == 0 {
		res.Valid = false
		res.Error = &ValidationError{
			Protocol:   ProtocolAnthropic,
			Field:      "messages",
			Message:    "messages: at least one non-system message is required",
			StatusCode: http.StatusBadRequest,
		}
		return res
	}
	req.Messages = filteredMsgs

	// 4. Role validation & First turn check
	firstRole := strings.ToLower(strings.TrimSpace(req.Messages[0].Role))
	if firstRole != "user" {
		if level == LevelStrict {
			res.Valid = false
			res.Error = &ValidationError{
				Protocol:   ProtocolAnthropic,
				Field:      "messages[0]",
				Message:    fmt.Sprintf("First message must have role 'user', got '%s'", req.Messages[0].Role),
				StatusCode: http.StatusBadRequest,
			}
			return res
		}
		// Lenient: prepend dummy user message
		req.Messages = append([]model.AnthropicInboundMessage{
			{Role: "user", Content: "Hello"},
		}, req.Messages...)
		res.AddWarning("prepended dummy user turn to satisfy Anthropic role sequence")
	}

	// 5. Alternating turns check (user <-> assistant)
	var mergedMsgs []model.AnthropicInboundMessage
	for i := 0; i < len(req.Messages); i++ {
		cur := req.Messages[i]
		curRole := strings.ToLower(strings.TrimSpace(cur.Role))
		if curRole != "user" && curRole != "assistant" {
			if level == LevelStrict {
				res.Valid = false
				res.Error = &ValidationError{
					Protocol:   ProtocolAnthropic,
					Field:      fmt.Sprintf("messages[%d].role", i),
					Message:    fmt.Sprintf("Role '%s' is invalid. Anthropic messages only accept 'user' or 'assistant'", cur.Role),
					StatusCode: http.StatusBadRequest,
				}
				return res
			}
			curRole = "user"
			cur.Role = "user"
			res.AddWarning(fmt.Sprintf("messages[%d]: normalized invalid role to 'user'", i))
		}

		if len(mergedMsgs) == 0 {
			mergedMsgs = append(mergedMsgs, cur)
			continue
		}

		prev := &mergedMsgs[len(mergedMsgs)-1]
		if strings.ToLower(prev.Role) == curRole {
			if level == LevelStrict {
				res.Valid = false
				res.Error = &ValidationError{
					Protocol:   ProtocolAnthropic,
					Field:      fmt.Sprintf("messages[%d]", i),
					Message:    fmt.Sprintf("Consecutive messages with role '%s' are forbidden in Anthropic; turns must alternate between user and assistant", curRole),
					StatusCode: http.StatusBadRequest,
				}
				return res
			}
			// Lenient: merge contents
			prev.Content = mergeAnthropicContents(prev.Content, cur.Content)
			res.AddWarning(fmt.Sprintf("merged consecutive '%s' turns into single turn", curRole))
		} else {
			mergedMsgs = append(mergedMsgs, cur)
		}
	}
	req.Messages = mergedMsgs

	// 6. Parameter validation: Temperature (0.0 ~ 1.0)
	if req.Temperature != nil {
		if *req.Temperature < 0.0 || *req.Temperature > 1.0 {
			if level == LevelStrict {
				res.Valid = false
				res.Error = &ValidationError{
					Protocol:   ProtocolAnthropic,
					Field:      "temperature",
					Message:    fmt.Sprintf("temperature must be between 0.0 and 1.0 for Anthropic models, got %.2f", *req.Temperature),
					StatusCode: http.StatusBadRequest,
				}
				return res
			}
			clamped := clampFloat(*req.Temperature, 0.0, 1.0)
			req.Temperature = &clamped
			res.AddWarning(fmt.Sprintf("clamped temperature to %.2f", clamped))
		}
	}

	// 7. Parameter validation: TopP (0.0 ~ 1.0)
	if req.TopP != nil {
		if *req.TopP < 0.0 || *req.TopP > 1.0 {
			if level == LevelStrict {
				res.Valid = false
				res.Error = &ValidationError{
					Protocol:   ProtocolAnthropic,
					Field:      "top_p",
					Message:    fmt.Sprintf("top_p must be between 0.0 and 1.0, got %.2f", *req.TopP),
					StatusCode: http.StatusBadRequest,
				}
				return res
			}
			clamped := clampFloat(*req.TopP, 0.0, 1.0)
			req.TopP = &clamped
			res.AddWarning(fmt.Sprintf("clamped top_p to %.2f", clamped))
		}
	}

	return res
}

func extractStringContent(content any) string {
	if content == nil {
		return ""
	}
	if s, ok := content.(string); ok {
		return s
	}
	if blocks, ok := content.([]any); ok {
		var sb strings.Builder
		for _, b := range blocks {
			if m, ok := b.(map[string]any); ok {
				if m["type"] == "text" {
					if t, ok := m["text"].(string); ok {
						sb.WriteString(t)
					}
				}
			}
		}
		return sb.String()
	}
	b, _ := json.Marshal(content)
	return string(b)
}

func mergeAnthropicContents(c1, c2 any) any {
	s1, ok1 := c1.(string)
	s2, ok2 := c2.(string)
	if ok1 && ok2 {
		return s1 + "\n\n" + s2
	}

	b1 := toAnySlice(c1)
	b2 := toAnySlice(c2)
	return append(b1, b2...)
}

func toAnySlice(c any) []any {
	if c == nil {
		return []any{}
	}
	if slice, ok := c.([]any); ok {
		return slice
	}
	if s, ok := c.(string); ok {
		return []any{map[string]any{"type": "text", "text": s}}
	}
	return []any{c}
}
