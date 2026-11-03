package provider

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/buildinfo"
)

func SetGeminiHeaders(request *http.Request, key string) {
	request.Header.Set("x-goog-api-client", "airoute/"+buildinfo.Version)
	if key != "" && key != "none" {
		request.Header.Set("x-goog-api-key", key)
	}
}

// CountTokens may nest the model inside generateContentRequest. Utility calls
// use Google's resource name, while generateContent keeps it in the URL.
func RewriteGeminiActionModel(body []byte, modelName string) []byte {
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return body
	}
	if _, exists := fields["model"]; exists {
		fields["model"], _ = json.Marshal("models/" + geminiModelPath(modelName))
	}
	for _, name := range []string{"generateContentRequest", "generate_content_request"} {
		if nested, exists := fields[name]; exists {
			fields[name] = RewriteGeminiActionModel(nested, modelName)
		}
	}
	out, _ := json.Marshal(fields)
	return out
}

// GeminiActionURL shares Gemini's API-root interpretation for utility calls.
func GeminiActionURL(baseURL, modelName, action string) string {
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	if strings.Contains(baseURL, "/publishers/google") {
		return strings.TrimRight(baseURL, "/") + "/models/" + geminiModelPath(modelName) + ":" + action
	}
	return joinAPIURLVersioned(baseURL, GeminiAPIVersion, "/v1beta/models/"+geminiModelPath(modelName)+":"+action, nil)
}
