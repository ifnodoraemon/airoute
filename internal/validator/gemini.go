package validator

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

// ValidateGemini validates and sanitizes a GeminiRequest.
func ValidateGemini(req *provider.GeminiRequest, rule *config.ModelValidationRule, level string) *ValidationResult {
	res := &ValidationResult{
		Level: level,
		Valid: true,
	}

	if level == LevelOff || req == nil {
		return res
	}

	// 1. Mandatory contents check
	if len(req.Contents) == 0 {
		res.Valid = false
		res.Error = &ValidationError{
			Protocol:   ProtocolGemini,
			Field:      "contents",
			Message:    "contents: must contain at least one content block",
			StatusCode: http.StatusBadRequest,
		}
		return res
	}

	// 2. Check for misplaced system messages in contents
	var filteredContents []provider.GeminiContent
	for i, c := range req.Contents {
		roleLower := strings.ToLower(strings.TrimSpace(c.Role))
		if roleLower == "system" {
			if level == LevelStrict {
				res.Valid = false
				res.Error = &ValidationError{
					Protocol:   ProtocolGemini,
					Field:      fmt.Sprintf("contents[%d]", i),
					Message:    "System instruction must be placed in 'systemInstruction', not in 'contents'",
					StatusCode: http.StatusBadRequest,
				}
				return res
			}
			if req.SystemInstruction == nil {
				req.SystemInstruction = &provider.GeminiSystemInstruction{Parts: c.Parts}
			} else {
				req.SystemInstruction.Parts = append(req.SystemInstruction.Parts, c.Parts...)
			}
			res.AddWarning("moved system instruction from contents to 'systemInstruction'")
			continue
		}
		filteredContents = append(filteredContents, c)
	}

	if len(filteredContents) == 0 {
		res.Valid = false
		res.Error = &ValidationError{
			Protocol:   ProtocolGemini,
			Field:      "contents",
			Message:    "contents: must contain at least one non-system content block",
			StatusCode: http.StatusBadRequest,
		}
		return res
	}
	req.Contents = filteredContents

	// 3. Roles and Turn Alternation (user <-> model)
	var mergedContents []provider.GeminiContent
	for i := 0; i < len(req.Contents); i++ {
		cur := req.Contents[i]
		curRole := strings.ToLower(strings.TrimSpace(cur.Role))

		// Translate assistant to model or check role validity
		if curRole == "assistant" {
			if level == LevelStrict {
				res.Valid = false
				res.Error = &ValidationError{
					Protocol:   ProtocolGemini,
					Field:      fmt.Sprintf("contents[%d].role", i),
					Message:    "Role 'assistant' is invalid for Gemini; must be 'model'",
					StatusCode: http.StatusBadRequest,
				}
				return res
			}
			curRole = "model"
			cur.Role = "model"
			res.AddWarning(fmt.Sprintf("contents[%d]: converted role 'assistant' to 'model'", i))
		} else if curRole != "user" && curRole != "model" {
			if level == LevelStrict {
				res.Valid = false
				res.Error = &ValidationError{
					Protocol:   ProtocolGemini,
					Field:      fmt.Sprintf("contents[%d].role", i),
					Message:    fmt.Sprintf("Invalid role '%s'. Gemini requires 'user' or 'model'", cur.Role),
					StatusCode: http.StatusBadRequest,
				}
				return res
			}
			curRole = "user"
			cur.Role = "user"
			res.AddWarning(fmt.Sprintf("contents[%d]: converted invalid role to 'user'", i))
		} else {
			cur.Role = curRole
		}

		// Parts check
		if len(cur.Parts) == 0 {
			if level == LevelStrict {
				res.Valid = false
				res.Error = &ValidationError{
					Protocol:   ProtocolGemini,
					Field:      fmt.Sprintf("contents[%d].parts", i),
					Message:    "parts: content must contain at least one part",
					StatusCode: http.StatusBadRequest,
				}
				return res
			}
			cur.Parts = []provider.GeminiPart{{Text: " "}}
			res.AddWarning(fmt.Sprintf("contents[%d]: added missing part", i))
		}

		if len(mergedContents) == 0 {
			mergedContents = append(mergedContents, cur)
			continue
		}

		prev := &mergedContents[len(mergedContents)-1]
		if prev.Role == curRole {
			if level == LevelStrict {
				res.Valid = false
				res.Error = &ValidationError{
					Protocol:   ProtocolGemini,
					Field:      fmt.Sprintf("contents[%d]", i),
					Message:    fmt.Sprintf("Consecutive contents with role '%s' are forbidden in Gemini; turns must alternate between user and model", curRole),
					StatusCode: http.StatusBadRequest,
				}
				return res
			}
			// Lenient: merge parts
			prev.Parts = append(prev.Parts, cur.Parts...)
			res.AddWarning(fmt.Sprintf("merged consecutive '%s' contents into single turn", curRole))
		} else {
			mergedContents = append(mergedContents, cur)
		}
	}
	req.Contents = mergedContents

	// 4. GenerationConfig validation
	if req.GenerationConfig != nil {
		if req.GenerationConfig.Temperature != nil {
			tVal := *req.GenerationConfig.Temperature
			if tVal < 0.0 || tVal > 2.0 {
				if level == LevelStrict {
					res.Valid = false
					res.Error = &ValidationError{
						Protocol:   ProtocolGemini,
						Field:      "generationConfig.temperature",
						Message:    fmt.Sprintf("temperature must be between 0.0 and 2.0, got %.2f", tVal),
						StatusCode: http.StatusBadRequest,
					}
					return res
				}
				clamped := clampFloat(tVal, 0.0, 2.0)
				req.GenerationConfig.Temperature = &clamped
				res.AddWarning(fmt.Sprintf("clamped generationConfig.temperature to %.2f", clamped))
			}
		}

		if req.GenerationConfig.TopP != nil {
			pVal := *req.GenerationConfig.TopP
			if pVal < 0.0 || pVal > 1.0 {
				if level == LevelStrict {
					res.Valid = false
					res.Error = &ValidationError{
						Protocol:   ProtocolGemini,
						Field:      "generationConfig.topP",
						Message:    fmt.Sprintf("topP must be between 0.0 and 1.0, got %.2f", pVal),
						StatusCode: http.StatusBadRequest,
					}
					return res
				}
				clamped := clampFloat(pVal, 0.0, 1.0)
				req.GenerationConfig.TopP = &clamped
				res.AddWarning(fmt.Sprintf("clamped generationConfig.topP to %.2f", clamped))
			}
		}

		if req.GenerationConfig.MaxOutputTokens != nil && *req.GenerationConfig.MaxOutputTokens <= 0 {
			if level == LevelStrict {
				res.Valid = false
				res.Error = &ValidationError{
					Protocol:   ProtocolGemini,
					Field:      "generationConfig.maxOutputTokens",
					Message:    fmt.Sprintf("maxOutputTokens must be greater than 0, got %d", *req.GenerationConfig.MaxOutputTokens),
					StatusCode: http.StatusBadRequest,
				}
				return res
			}
			req.GenerationConfig.MaxOutputTokens = nil
			res.AddWarning("cleared non-positive maxOutputTokens")
		}
	}

	return res
}
