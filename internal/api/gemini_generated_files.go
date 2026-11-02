package api

import (
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

func (h *Handler) retainGeneratedFiles(c *gin.Context, name, identity string, body []byte) error {
	if callerOwner(c) == "" {
		return nil
	}
	body = provider.CanonicalGeminiJSON(googleSSEData(body))
	var response struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					File struct {
						URI  string `json:"fileUri"`
						MIME string `json:"mimeType"`
					} `json:"fileData"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if json.Unmarshal(body, &response) != nil {
		return nil
	}
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			id := geminiFileName(part.File.URI)
			if id == "" {
				continue
			}
			channel, _, err := h.selectGoogleChannel(c, name, identity)
			if err != nil {
				return err
			}
			data, _ := json.Marshal(map[string]string{"name": id, "uri": part.File.URI, "mimeType": part.File.MIME})
			r := &googleResource{namespace: "gemini-file", collection: strings.SplitN(id, "/", 2)[0], model: name, channel: channel}
			if err := saveGoogleResource(c, r, id, data); err != nil {
				return err
			}
		}
	}
	return nil
}
