package provider

import "github.com/ifnodoraemon/airoute/internal/model"

// Native Messages has already been attempted before this check. Conversion may
// not silently discard native thinking fields or a different protocol's intent.
func validateAnthropicThinkingTranslation(req *model.ChatCompletionRequest) error {
	if prepared := req.PreparedThinking; prepared != nil {
		wire := prepared.Profile.Wire
		if prepared.Control.Mode != "auto" && wire != "none" && wire != "claude-budget" && wire != "claude-adaptive" {
			return &UnsupportedTranslationError{Reason: "configured thinking profile cannot be translated to Anthropic Messages"}
		}
		return nil
	}
	if hasNativeThinking(req) {
		return &UnsupportedTranslationError{Reason: "native thinking parameters cannot be translated to Anthropic Messages; use thinking_control or the native Messages endpoint"}
	}
	return nil
}
