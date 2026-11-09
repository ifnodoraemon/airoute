package model

import (
	"encoding/json"
	"fmt"
	"strings"
)

// responseContentToChatContent converts a Responses message `content` (a string
// or an array of content parts) into chat content.
//
// An all-text part list collapses to a plain string because that is what chat
// models expect for the common case; mixing in an image switches the whole
// content to the chat part array so the image survives.
func responseContentToChatContent(content any) (any, bool, error) {
	switch typed := content.(type) {
	case nil:
		return nil, false, nil
	case string:
		if typed == "" {
			return nil, false, nil
		}
		return typed, true, nil
	case []any:
		return responseContentPartsToChatContent(typed)
	case []map[string]any:
		parts := make([]any, len(typed))
		for i := range typed {
			parts[i] = typed[i]
		}
		return responseContentPartsToChatContent(parts)
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return nil, false, err
		}
		var parts []any
		if err := json.Unmarshal(encoded, &parts); err != nil {
			return nil, false, fmt.Errorf("message 'content' must be a string or an array of content parts, got %T", content)
		}
		return responseContentPartsToChatContent(parts)
	}
}

// responseContentPart is one converted content part in its original order.
type responseContentPart struct {
	text  string
	image map[string]any
}

// responseContentPartsToChatContent converts Responses content parts.
func responseContentPartsToChatContent(parts []any) (any, bool, error) {
	converted := make([]responseContentPart, 0, len(parts))
	for _, raw := range parts {
		part, ok := raw.(map[string]any)
		if !ok {
			// A bare string is tolerated as a text part.
			if text, isText := raw.(string); isText && text != "" {
				converted = append(converted, responseContentPart{text: text})
			}
			continue
		}
		partType, _ := part["type"].(string)
		switch partType {
		case "", "text", "input_text", "output_text":
			if text, ok := part["text"].(string); ok && text != "" {
				converted = append(converted, responseContentPart{text: text})
			}
		case "image_url", "input_image":
			if image := responseImagePartToChatPart(part); image != nil {
				converted = append(converted, responseContentPart{image: image})
			}
		default:
			// input_file / input_audio / refusal parts have no chat text or image
			// representation here; dropping them is better than corrupting the turn.
		}
	}
	if len(converted) == 0 {
		return nil, false, nil
	}

	allText := true
	for _, part := range converted {
		if part.image != nil {
			allText = false
			break
		}
	}
	if allText {
		texts := make([]string, 0, len(converted))
		for _, part := range converted {
			texts = append(texts, part.text)
		}
		return strings.Join(texts, "\n"), true, nil
	}

	chatParts := make([]any, 0, len(converted))
	for _, part := range converted {
		if part.image != nil {
			chatParts = append(chatParts, part.image)
			continue
		}
		chatParts = append(chatParts, map[string]any{"type": "text", "text": part.text})
	}
	return chatParts, true, nil
}

// responseImagePartToChatPart maps a Responses input_image part to the chat
// image_url part shape. `detail` is carried on the part itself (Responses) or
// inside the nested image_url object (chat clients), so both are accepted.
func responseImagePartToChatPart(part map[string]any) map[string]any {
	var url, detail string
	switch raw := part["image_url"].(type) {
	case string:
		url = strings.TrimSpace(raw)
	case map[string]any:
		url, _ = raw["url"].(string)
		detail, _ = raw["detail"].(string)
	}
	if detail == "" {
		detail, _ = part["detail"].(string)
	}
	if url == "" {
		// A file_id-only image cannot be resolved by the gateway.
		return nil
	}
	imageURL := map[string]any{"url": url}
	if detail != "" {
		imageURL["detail"] = detail
	}
	return map[string]any{"type": "image_url", "image_url": imageURL}
}
