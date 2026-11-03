package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

var googleImplementedMethods = map[string]bool{"generateContent": true, "streamGenerateContent": true, "countTokens": true, "embedContent": true, "batchEmbedContents": true, "batchGenerateContent": true, "asyncBatchEmbedContent": true}

func (h *Handler) HandleGeminiModels(c *gin.Context) {
	names := h.dispatcher.GetAllSupportedModels()
	sort.Strings(names)
	var visible []string
	for _, name := range names {
		if middleware.ValidateModelAllowed(c, name) {
			visible = append(visible, name)
		}
	}
	items := make([]map[string]any, len(visible))
	semaphore := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, name := range visible {
		semaphore <- struct{}{}
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			defer func() { <-semaphore }()
			items[i] = h.googleModelInfo(c, name)
		}(i, name)
	}
	wg.Wait()
	if items == nil {
		items = []map[string]any{}
	}
	c.JSON(200, gin.H{"models": items})
}

func (h *Handler) HandleGeminiModelDetail(c *gin.Context) {
	name := strings.TrimPrefix(strings.TrimPrefix(c.Param("modelAction"), "/"), "models/")
	if !middleware.ValidateModelAllowed(c, name) {
		RespondGeminiError(c, 403, fmt.Sprintf("API key not allowed to access model '%s'", name), "PERMISSION_DENIED")
		return
	}
	if len(h.dispatcher.ListChannelsForModel(name, "")) == 0 {
		RespondGeminiError(c, 404, "Model not found", "NOT_FOUND")
		return
	}
	c.JSON(http.StatusOK, h.googleModelInfo(c, name))
}

func (h *Handler) googleModelInfo(c *gin.Context, name string) map[string]any {
	info := map[string]any{"name": "models/" + name, "displayName": name, "version": "001"}
	methods := make(map[string]bool)
	for _, ch := range h.dispatcher.ListChannelsForModel(name, "") {
		if model.IsGeminiProvider(ch.Type) {
			var native map[string]json.RawMessage
			if json.Unmarshal(provider.GoogleModelInfo(c.Request.Context(), ch, name), &native) == nil && native["supportedGenerationMethods"] != nil {
				var supported []string
				_ = json.Unmarshal(native["supportedGenerationMethods"], &supported)
				for _, method := range supported {
					if googleImplementedMethods[method] {
						methods[method] = true
					}
				}
				for key, value := range native {
					if key != "name" && key != "supportedGenerationMethods" {
						info[key] = value
					}
				}
				continue
			}
		}
		actual := strings.ToLower(ch.GetUpstreamModel(name))
		embedding := strings.Contains(actual, "embed") || (len(ch.Protocols) > 0 && ch.SupportsProtocol("embeddings") && !ch.SupportsProtocol("openai_chat") && !ch.SupportsProtocol("messages"))
		if embedding {
			methods["embedContent"] = true
			if model.IsGeminiProvider(ch.Type) {
				methods["batchEmbedContents"] = true
			}
			continue
		}
		if model.IsGeminiProvider(ch.Type) || len(ch.Protocols) == 0 || ch.SupportsProtocol("openai_chat") || ch.SupportsProtocol("messages") || ch.SupportsProtocol("responses") {
			for _, method := range []string{"generateContent", "streamGenerateContent", "countTokens"} {
				methods[method] = true
			}
		}
	}
	var supported []string
	for method := range methods {
		supported = append(supported, method)
	}
	sort.Strings(supported)
	info["name"] = "models/" + name
	info["supportedGenerationMethods"] = supported
	return info
}
