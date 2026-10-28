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
	} else if !clampRange(res, level, ProtocolOpenAI, "temperature", &req.Temperature, 0.0, 2.0) {
		return res
	}

	// 4. Parameter validation: TopP
	if !clampRange(res, level, ProtocolOpenAI, "top_p", &req.TopP, 0.0, 1.0) {
		return res
	}

	// 5. Parameter validation: PresencePenalty & FrequencyPenalty
	if !clampRange(res, level, ProtocolOpenAI, "presence_penalty", &req.PresencePenalty, -2.0, 2.0) {
		return res
	}
	if !clampRange(res, level, ProtocolOpenAI, "frequency_penalty", &req.FrequencyPenalty, -2.0, 2.0) {
		return res
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

// clampRange validates a bounded float parameter. In strict mode an out-of-range
// value is a hard error; in lenient mode it is clamped and reported as a warning.
// Returns false when the caller must return the (invalid) result.
func clampRange(res *ValidationResult, level, protocol, field string, v **float64, lo, hi float64) bool {
	if v == nil || *v == nil {
		return true
	}
	value := **v
	if value >= lo && value <= hi {
		return true
	}
	if level == LevelStrict {
		res.Valid = false
		res.Error = &ValidationError{
			Protocol:   protocol,
			Field:      field,
			Message:    fmt.Sprintf("%s must be between %.1f and %.1f, got %.2f", field, lo, hi, value),
			Code:       "parameter_out_of_range",
			StatusCode: http.StatusBadRequest,
		}
		return false
	}
	clamped := clampFloat(value, lo, hi)
	*v = &clamped
	res.AddWarning(fmt.Sprintf("clamped %s to %.2f", field, clamped))
	return true
}
