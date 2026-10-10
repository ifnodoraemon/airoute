package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// Embed executes embedding generation on Google Gemini Developer API.
func (p *GeminiProvider) Embed(ctx context.Context, req *model.EmbeddingRequest, channel *model.ChannelConfig) (*model.EmbeddingResponse, error) {
	targetModel := channel.GetUpstreamModel(req.Model)
	texts := req.GetInputStrings()
	if len(texts) == 0 {
		return nil, fmt.Errorf("input text is required for embedding")
	}

	baseURL := channel.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	var items []model.EmbeddingItem
	totalTokens := 0

	if len(texts) == 1 {
		// Single embedContent call
		targetURL := fmt.Sprintf("%s/v1beta/models/%s:embedContent?key=%s", baseURL, targetModel, channel.APIKey)
		type singleReq struct {
			Model   string `json:"model,omitempty"`
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		}
		var sReq singleReq
		sReq.Model = "models/" + strings.TrimPrefix(targetModel, "models/")
		sReq.Content.Parts = []struct {
			Text string `json:"text"`
		}{{Text: texts[0]}}

		payload, _ := json.Marshal(sReq)
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("x-goog-api-key", channel.APIKey)

		resp, err := p.client.Do(httpReq)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("gemini embed returned %d: %s", resp.StatusCode, string(body))
		}

		var parsed struct {
			Embedding struct {
				Values []float64 `json:"values"`
			} `json:"embedding"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, err
		}
		items = append(items, model.EmbeddingItem{
			Object:    "embedding",
			Index:     0,
			Embedding: parsed.Embedding.Values,
		})
		totalTokens = len(texts[0]) / 4
		if totalTokens == 0 {
			totalTokens = 1
		}
	} else {
		// batchEmbedContents call
		targetURL := fmt.Sprintf("%s/v1beta/models/%s:batchEmbedContents?key=%s", baseURL, targetModel, channel.APIKey)
		type itemReq struct {
			Model   string `json:"model"`
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		}
		type batchReq struct {
			Requests []itemReq `json:"requests"`
		}
		var bReq batchReq
		modelPath := "models/" + strings.TrimPrefix(targetModel, "models/")
		for _, t := range texts {
			toks := len(t) / 4
			if toks == 0 {
				toks = 1
			}
			totalTokens += toks
			bReq.Requests = append(bReq.Requests, itemReq{
				Model: modelPath,
				Content: struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				}{
					Parts: []struct {
						Text string `json:"text"`
					}{{Text: t}},
				},
			})
		}

		payload, _ := json.Marshal(bReq)
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("x-goog-api-key", channel.APIKey)

		resp, err := p.client.Do(httpReq)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("gemini batchEmbed returned %d: %s", resp.StatusCode, string(body))
		}

		var parsed struct {
			Embeddings []struct {
				Values []float64 `json:"values"`
			} `json:"embeddings"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, err
		}
		for idx, emb := range parsed.Embeddings {
			items = append(items, model.EmbeddingItem{
				Object:    "embedding",
				Index:     idx,
				Embedding: emb.Values,
			})
		}
	}

	return &model.EmbeddingResponse{
		Object: "list",
		Data:   items,
		Model:  req.Model,
		Usage: model.EmbeddingUsage{
			PromptTokens: totalTokens,
			TotalTokens:  totalTokens,
		},
	}, nil
}
