package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
)

func googleBatchOutputFiles(body []byte) []string {
	var object map[string]json.RawMessage
	if json.Unmarshal(body, &object) != nil {
		return nil
	}
	var files []string
	for key, raw := range object {
		if key == "responsesFile" || key == "responses_file" {
			var name string
			_ = json.Unmarshal(raw, &name)
			if name = geminiFileName(name); name != "" {
				files = append(files, name)
			}
		} else if key == "response" || key == "metadata" || key == "output" {
			files = append(files, googleBatchOutputFiles(raw)...)
		}
	}
	seen := map[string]bool{}
	unique := files[:0]
	for _, file := range files {
		if !seen[file] {
			seen[file] = true
			unique = append(unique, file)
		}
	}
	return unique
}

func (h *Handler) googleBatchFileUsage(c *gin.Context, r *googleResource, file string) (*model.Usage, error) {
	response, err := h.dispatcher.DispatchHTTP(c.Request.Context(), &router.UpstreamRequest{
		PinnedChannel: r.channel.Name, PinnedChannelID: r.channel.ID, PinnedBaseURL: r.channel.BaseURL, PinnedIdentity: model.UpstreamIdentity(r.channel),
		Model: r.model, Method: http.MethodGet, GeminiPath: "/v1beta/" + file + ":download?alt=media", NoRetry: true, NoRewriteBody: true,
	})
	if err != nil {
		return nil, err
	}
	defer response.Stream.Close()
	scanner := bufio.NewScanner(response.Stream)
	scanner.Buffer(make([]byte, 64<<10), 64<<20)
	total := &model.Usage{}
	for scanner.Scan() {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		if !json.Valid(scanner.Bytes()) {
			return nil, fmt.Errorf("invalid batch result JSONL")
		}
		usage, _ := googleBatchUsage(scanner.Bytes())
		total.PromptTokens += usage.PromptTokens
		total.CompletionTokens += usage.CompletionTokens
		total.TotalTokens += usage.TotalTokens
	}
	return total, scanner.Err()
}
