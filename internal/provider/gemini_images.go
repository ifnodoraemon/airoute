package provider

import (
	"encoding/base64"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// Chat image_url does not carry a MIME type. Gemini fileData requires one,
// including for public HTTP URLs; a file suffix cannot establish the actual
// content type. Require inline data with explicit MIME or native Gemini parts
// rather than downloading caller URLs or labelling every image as JPEG.
func validateGeminiImages(req *model.ChatCompletionRequest) error {
	for _, msg := range req.Messages {
		for _, part := range model.ParseMessageContent(msg.Content) {
			if part.Type != model.ContentPartImageURL || part.ImageURL == nil {
				continue
			}
			uri := part.ImageURL.URL
			if !strings.HasPrefix(uri, "data:image/") {
				return &UnsupportedTranslationError{Reason: "Gemini image translation requires a base64 data URI with an explicit image MIME type; use inline image data or the native Gemini endpoint for fileData"}
			}
			mime, data := model.ParseDataURI(uri)
			if !strings.Contains(uri, ";base64,") || !supportedGeminiImageMIME(mime) {
				return &UnsupportedTranslationError{Reason: "Gemini image translation requires a supported base64 image MIME type"}
			}
			if decoded, err := base64.StdEncoding.DecodeString(data); err != nil || len(decoded) == 0 {
				return &UnsupportedTranslationError{Reason: "Gemini image translation requires valid non-empty base64 image data"}
			}
		}
	}
	return nil
}

func supportedGeminiImageMIME(mime string) bool {
	switch mime {
	case "image/png", "image/jpeg", "image/webp", "image/heic", "image/heif":
		return true
	}
	return false
}
