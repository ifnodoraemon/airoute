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

// ParseMessageContent normalizes any message content (string or array of parts) into []ContentPart.
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

	// 2. Slice of parts
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

	// 3. Fallback to JSON text
	b, _ := json.Marshal(raw)
	return []ContentPart{
		{
			Type: ContentPartText,
			Text: string(b),
		},
	}
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
