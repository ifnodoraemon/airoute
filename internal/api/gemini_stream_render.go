package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

func writeGeminiCandidate(w io.Writer, flusher http.Flusher, index int, parts []gin.H, reason string, usage gin.H) {
	if parts == nil {
		parts = []gin.H{}
	}
	candidate := gin.H{"index": index, "content": gin.H{"role": "model", "parts": parts}}
	if reason != "" {
		candidate["finishReason"] = reason
	}
	chunk := gin.H{"candidates": []gin.H{candidate}}
	if usage != nil {
		chunk["usageMetadata"] = usage
	}
	if body, err := json.Marshal(chunk); err == nil {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", body)
		flusher.Flush()
	}
}

func geminiFunctionCallParts(calls []geminiFunctionCall) []gin.H {
	parts := make([]gin.H, 0, len(calls))
	for _, call := range calls {
		function := gin.H{"name": call.Name, "args": call.Args}
		if call.ID != "" {
			function["id"] = call.ID
		}
		part := gin.H{"functionCall": function}
		if call.Signature != "" {
			part["thoughtSignature"] = call.Signature
		}
		parts = append(parts, part)
	}
	return parts
}

func geminiStreamUsage(prompt, completion, cached, thoughts int) gin.H {
	return gin.H{
		"promptTokenCount":        prompt,
		"candidatesTokenCount":    max(0, completion-thoughts),
		"thoughtsTokenCount":      thoughts,
		"cachedContentTokenCount": cached,
		"totalTokenCount":         prompt + completion,
	}
}
