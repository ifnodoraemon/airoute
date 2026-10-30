package model

import (
	"encoding/json"
	"strings"
)

// ContentPartType defines the modality category.
type ContentPartType string

const (
	ContentPartText       ContentPartType = "text"
	ContentPartImageURL   ContentPartType = "image_url"
	ContentPartInputAudio ContentPartType = "input_audio"
)

// ImageURLPart holds image payload.
type ImageURLPart struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"` // low, high, auto
}

// InputAudioPart holds audio payload.
type InputAudioPart struct {
	Data   string `json:"data"`
	Format string `json:"format"` // wav, mp3
}

// ContentPart represents an item in a multimodal content array.
type ContentPart struct {
	Type       ContentPartType `json:"type"`
	Text       string          `json:"text,omitempty"`
	ImageURL   *ImageURLPart   `json:"image_url,omitempty"`
	InputAudio *InputAudioPart `json:"input_audio,omitempty"`
}

// ParseMessageContent normalizes any message content (string, array of parts,
// or typed content parts) into []ContentPart.
//
// The typed []ContentPart form must be handled here rather than through the JSON
// fallback: the protocol adapters build message content this way (see
// internal/adapter/anthropic.go and internal/adapter/gemini.go), and the fallback
// serializes the value into ONE text part. That misclassifies a data: URI image as
// prompt text, which both inflates the token estimate by ~100x and makes the
// outbound converters send the picture as prose (F-01/LEAD-01).
func ParseMessageContent(raw any) []ContentPart {
	if raw == nil {
		return nil
	}

	// 1. Plain string
	if str, ok := raw.(string); ok {
		return []ContentPart{
			{
				Type: ContentPartText,
				Text: str,
			},
		}
	}

	// 2. Typed content parts: reuse them (normalised) instead of round-tripping
	// through JSON, which would turn them into a single opaque text part.
	switch typed := raw.(type) {
	case []ContentPart:
		return normalizeContentParts(typed)
	case ContentPart:
		return normalizeContentParts([]ContentPart{typed})
	case *ContentPart:
		if typed == nil {
			return nil
		}
		return normalizeContentParts([]ContentPart{*typed})
	case []*ContentPart:
		parts := make([]ContentPart, 0, len(typed))
		for _, part := range typed {
			if part != nil {
				parts = append(parts, *part)
			}
		}
		return normalizeContentParts(parts)
	}

	// 3. Slice of parts
	if slice, ok := raw.([]any); ok {
		var parts []ContentPart
		for _, item := range slice {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			partType, _ := m["type"].(string)
			switch partType {
			case "text":
				if text, ok := m["text"].(string); ok {
					parts = append(parts, ContentPart{
						Type: ContentPartText,
						Text: text,
					})
				}
			case "image_url":
				if imgObj, ok := m["image_url"].(map[string]any); ok {
					url, _ := imgObj["url"].(string)
					detail, _ := imgObj["detail"].(string)
					parts = append(parts, ContentPart{
						Type: ContentPartImageURL,
						ImageURL: &ImageURLPart{
							URL:    url,
							Detail: detail,
						},
					})
				}
			case "input_audio":
				if audioObj, ok := m["input_audio"].(map[string]any); ok {
					data, _ := audioObj["data"].(string)
					format, _ := audioObj["format"].(string)
					parts = append(parts, ContentPart{
						Type: ContentPartInputAudio,
						InputAudio: &InputAudioPart{
							Data:   data,
							Format: format,
						},
					})
				}
			}
		}
		return parts
	}

	// 4. Fallback to JSON text (unknown representation; see the typed branches
	// above for the shapes the gateway itself produces).
	b, _ := json.Marshal(raw)
	return []ContentPart{
		{
			Type: ContentPartText,
			Text: string(b),
		},
	}
}

// normalizeContentParts clones typed content parts and fills in a part type that
// the producer left implicit, so callers can switch on Type alone.
func normalizeContentParts(parts []ContentPart) []ContentPart {
	if len(parts) == 0 {
		return nil
	}
	out := make([]ContentPart, 0, len(parts))
	for _, part := range parts {
		switch {
		case part.Type == "" && part.ImageURL != nil:
			part.Type = ContentPartImageURL
		case part.Type == "" && part.InputAudio != nil:
			part.Type = ContentPartInputAudio
		case part.Type == "":
			part.Type = ContentPartText
		}
		out = append(out, part)
	}
	return out
}

// ParseDataURI splits a data URI (e.g. data:image/png;base64,iVBORw0KGgo...) into mimeType and raw base64.
func ParseDataURI(uri string) (mimeType string, base64Data string) {
	if !strings.HasPrefix(uri, "data:") {
		return "image/jpeg", uri // raw base64 or remote URL
	}
	parts := strings.SplitN(strings.TrimPrefix(uri, "data:"), ";base64,", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "image/jpeg", uri
}

// IsRemoteContentURL reports whether a content URL is an http(s) resource instead
// of inline data.
//
// Providers must forward such a value as a URL, never inside a base64 field
// (Anthropic's source.type=base64 / Gemini's inlineData.data): those fields reject
// a URL, and ParseDataURI's "remote URL" pass-through made the gateway send
// exactly that (F-07).
func IsRemoteContentURL(uri string) bool {
	lower := strings.ToLower(strings.TrimSpace(uri))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// =============================================================================
// Image Generation & Editing Models
// =============================================================================

// ImageGenerationRequest represents an image generation prompt.
type ImageGenerationRequest struct {
	Prompt         string `json:"prompt"`
	Model          string `json:"model"`
	N              int    `json:"n,omitempty"`
	Quality        string `json:"quality,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"` // url or b64_json
	Size           string `json:"size,omitempty"`            // 1024x1024, 512x512, etc.
	Style          string `json:"style,omitempty"`           // vivid or natural
	User           string `json:"user,omitempty"`
}

// ImageItem holds the resulting image data.
type ImageItem struct {
	B64JSON       string `json:"b64_json,omitempty"`
	URL           string `json:"url,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// ImageGenerationResponse represents the response containing generated images.
type ImageGenerationResponse struct {
	Created int64       `json:"created"`
	Data    []ImageItem `json:"data"`
}

// =============================================================================
// Audio Speech (TTS) & Transcription (STT) Models
// =============================================================================

// AudioSpeechRequest represents a text-to-speech request.
type AudioSpeechRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice"`
	ResponseFormat string  `json:"response_format,omitempty"` // mp3, opus, aac, flac, wav, pcm
	Speed          float64 `json:"speed,omitempty"`           // 0.25 to 4.0
}

// AudioTranscriptionResponse represents a speech-to-text response.
type AudioTranscriptionResponse struct {
	Text     string  `json:"text"`
	Language string  `json:"language,omitempty"`
	Duration float64 `json:"duration,omitempty"`
}

// =============================================================================
// Video Generation Models
// =============================================================================

// VideoGenerationRequest represents a text-to-video or image-to-video request.
type VideoGenerationRequest struct {
	Prompt      string `json:"prompt"`
	Model       string `json:"model"`
	ImageURL    string `json:"image_url,omitempty"`
	Duration    int    `json:"duration,omitempty"`     // duration in seconds
	AspectRatio string `json:"aspect_ratio,omitempty"` // 16:9, 9:16, 1:1
	Resolution  string `json:"resolution,omitempty"`   // 720p, 1080p
}

// VideoTaskResponse represents an asynchronous or synchronous video generation task.
type VideoTaskResponse struct {
	ID        string `json:"id"`
	Status    string `json:"status"` // pending, processing, succeeded, failed
	Progress  int    `json:"progress,omitempty"`
	VideoURL  string `json:"video_url,omitempty"`
	Error     string `json:"error,omitempty"`
	CreatedAt int64  `json:"created_at"`
}
