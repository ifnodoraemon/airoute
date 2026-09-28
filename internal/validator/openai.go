package validator

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/model"
)

var validOpenAIRoles = map[string]bool{
	"system":    true,
	"developer": true,
	"user":      true,
	"assistant": true,
	"tool":      true,
	"function":  true,
}

// ValidateOpenAI validates and sanitizes an OpenAI ChatCompletionRequest.
func ValidateOpenAI(req *model.ChatCompletionRequest, rule *config.ModelValidationRule, level string) *ValidationResult {
	res := &ValidationResult{
		Level: level,
		Valid: true,
	}

	if level == LevelOff || req == nil {
		return res
	}

	// 1. Validate 'messages' presence and non-emptiness
	if len(req.Messages) == 0 {
		res.Valid = false
		res.Error = &ValidationError{
			Protocol:   ProtocolOpenAI,
			Field:      "messages",
			Message:    "Missing required parameter 'messages' or 'messages' array is empty",
			Code:       "missing_required_parameter",
			StatusCode: http.StatusBadRequest,
		}
		return res
	}

	// 2. Validate messages structure and role consistency
	for i := range req.Messages {
		msg := &req.Messages[i]
		roleLower := strings.ToLower(strings.TrimSpace(msg.Role))

		if !validOpenAIRoles[roleLower] {
			if level == LevelStrict {
				res.Valid = false
				res.Error = &ValidationError{
					Protocol:   ProtocolOpenAI,
					Field:      fmt.Sprintf("messages[%d].role", i),
					Message:    fmt.Sprintf("Invalid role '%s'. Supported roles are: system, developer, user, assistant, tool, function", msg.Role),
					Code:       "invalid_role",
					StatusCode: http.StatusBadRequest,
				}
				return res
			}
			// Lenient: default to user
			msg.Role = "user"
			res.AddWarning(fmt.Sprintf("messages[%d]: converted invalid role '%s' to 'user'", i, roleLower))
		} else {
			msg.Role = roleLower
		}

		// Tool role must have tool_call_id
		if msg.Role == "tool" && strings.TrimSpace(msg.ToolCallID) == "" {
			if level == LevelStrict {
				res.Valid = false
				res.Error = &ValidationError{
					Protocol:   ProtocolOpenAI,
					Field:      fmt.Sprintf("messages[%d].tool_call_id", i),
					Message:    "messages with role 'tool' must have a non-empty 'tool_call_id'",
					Code:       "missing_tool_call_id",
					StatusCode: http.StatusBadRequest,
				}
				return res
			}
			msg.ToolCallID = fmt.Sprintf("call_synth_%d", i)
			res.AddWarning(fmt.Sprintf("messages[%d]: generated synthetic tool_call_id", i))
		}

		// Assistant with tool_calls must have valid id and function.name
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			for j := range msg.ToolCalls {
				tc := &msg.ToolCalls[j]
				if strings.TrimSpace(tc.ID) == "" || strings.TrimSpace(tc.Function.Name) == "" {
					if level == LevelStrict {
						res.Valid = false
						res.Error = &ValidationError{
							Protocol:   ProtocolOpenAI,
							Field:      fmt.Sprintf("messages[%d].tool_calls[%d]", i, j),
							Message:    "Each tool_call must contain non-empty 'id' and 'function.name'",
							Code:       "invalid_tool_call",
							StatusCode: http.StatusBadRequest,
						}
						return res
					}
					if strings.TrimSpace(tc.ID) == "" {
						tc.ID = fmt.Sprintf("call_%d_%d", i, j)
					}
					res.AddWarning(fmt.Sprintf("messages[%d].tool_calls[%d]: sanitized incomplete tool_call", i, j))
				}
			}
		}
	}

	// 3. Parameter validation: Temperature
	disallowTemp := (rule != nil && rule.DisallowTemperature)
	if disallowTemp {
		if req.Temperature != nil && *req.Temperature != 1.0 {
			if level == LevelStrict {
				res.Valid = false
				res.Error = &ValidationError{
					Protocol:   ProtocolOpenAI,
					Field:      "temperature",
					Message:    fmt.Sprintf("Model '%s' is a reasoning model and does not support custom temperature. Omit temperature or set to 1.0", req.Model),
					Code:       "unsupported_parameter",
					StatusCode: http.StatusBadRequest,
				}
				return res
			}
			req.Temperature = nil
			res.AddWarning("omitted custom temperature for reasoning model")
		}
	} else if req.Temperature != nil {
		if *req.Temperature < 0.0 || *req.Temperature > 2.0 {
			if level == LevelStrict {
				res.Valid = false
				res.Error = &ValidationError{
					Protocol:   ProtocolOpenAI,
					Field:      "temperature",
					Message:    fmt.Sprintf("temperature must be between 0.0 and 2.0, got %.2f", *req.Temperature),
					Code:       "parameter_out_of_range",
					StatusCode: http.StatusBadRequest,
				}
				return res
			}
			clamped := clampFloat(*req.Temperature, 0.0, 2.0)
			req.Temperature = &clamped
			res.AddWarning(fmt.Sprintf("clamped temperature to %.2f", clamped))
		}
	}

	// 4. Parameter validation: TopP
	if req.TopP != nil {
		if *req.TopP < 0.0 || *req.TopP > 1.0 {
			if level == LevelStrict {
				res.Valid = false
				res.Error = &ValidationError{
					Protocol:   ProtocolOpenAI,
					Field:      "top_p",
					Message:    fmt.Sprintf("top_p must be between 0.0 and 1.0, got %.2f", *req.TopP),
					Code:       "parameter_out_of_range",
					StatusCode: http.StatusBadRequest,
				}
				return res
			}
			clamped := clampFloat(*req.TopP, 0.0, 1.0)
			req.TopP = &clamped
			res.AddWarning(fmt.Sprintf("clamped top_p to %.2f", clamped))
		}
	}

	// 5. Parameter validation: PresencePenalty & FrequencyPenalty
	if req.PresencePenalty != nil {
		if *req.PresencePenalty < -2.0 || *req.PresencePenalty > 2.0 {
			if level == LevelStrict {
				res.Valid = false
				res.Error = &ValidationError{
					Protocol:   ProtocolOpenAI,
					Field:      "presence_penalty",
					Message:    fmt.Sprintf("presence_penalty must be between -2.0 and 2.0, got %.2f", *req.PresencePenalty),
					Code:       "parameter_out_of_range",
					StatusCode: http.StatusBadRequest,
				}
				return res
			}
			clamped := clampFloat(*req.PresencePenalty, -2.0, 2.0)
			req.PresencePenalty = &clamped
			res.AddWarning(fmt.Sprintf("clamped presence_penalty to %.2f", clamped))
		}
	}
	if req.FrequencyPenalty != nil {
		if *req.FrequencyPenalty < -2.0 || *req.FrequencyPenalty > 2.0 {
			if level == LevelStrict {
				res.Valid = false
				res.Error = &ValidationError{
					Protocol:   ProtocolOpenAI,
					Field:      "frequency_penalty",
					Message:    fmt.Sprintf("frequency_penalty must be between -2.0 and 2.0, got %.2f", *req.FrequencyPenalty),
					Code:       "parameter_out_of_range",
					StatusCode: http.StatusBadRequest,
				}
				return res
			}
			clamped := clampFloat(*req.FrequencyPenalty, -2.0, 2.0)
			req.FrequencyPenalty = &clamped
			res.AddWarning(fmt.Sprintf("clamped frequency_penalty to %.2f", clamped))
		}
	}

	// 6. Parameter validation: MaxTokens
	if req.MaxTokens != nil && *req.MaxTokens <= 0 {
		if level == LevelStrict {
			res.Valid = false
			res.Error = &ValidationError{
				Protocol:   ProtocolOpenAI,
				Field:      "max_tokens",
				Message:    fmt.Sprintf("max_tokens must be greater than 0, got %d", *req.MaxTokens),
				Code:       "parameter_out_of_range",
				StatusCode: http.StatusBadRequest,
			}
			return res
		}
		req.MaxTokens = nil
		res.AddWarning("cleared non-positive max_tokens")
	}

	return res
}

func clampFloat(val, min, max float64) float64 {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}
