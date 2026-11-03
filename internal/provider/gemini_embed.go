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
	if req.Native != nil && req.Native.Protocol == model.NativeGeminiEmbedding {
		return p.embedNative(ctx, req, channel)
	}
	targetModel := channel.GetUpstreamModel(req.Model)
	texts := req.GetInputStrings()
	if len(texts) == 0 {
		return nil, fmt.Errorf("input text is required for embedding")
	}

	baseURL := GeminiBaseURL(channel)
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}

	var items []model.EmbeddingItem
	totalTokens := 0

	if len(texts) == 1 {
		// Single embedContent call
		targetURL := GeminiActionURL(baseURL, targetModel, "embedContent")
		type singleReq struct {
			OutputDimensionality *int   `json:"outputDimensionality,omitempty"`
			Model                string `json:"model,omitempty"`
			Content              struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		}
		var sReq singleReq
		sReq.OutputDimensionality = req.Dimensions
		sReq.Model = "models/" + geminiModelPath(targetModel)
		sReq.Content.Parts = []struct {
			Text string `json:"text"`
		}{{Text: texts[0]}}

		payload, _ := json.Marshal(sReq)
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if err := ConfigureGoogleRequest(ctx, httpReq, channel); err != nil {
			return nil, err
		}

		resp, err := p.client.Do(httpReq)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return nil, NewUpstreamError(channel.Name, resp.StatusCode, []byte(body), resp.Header)
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
		totalTokens = model.EstimateTokensFromChars(len(texts[0]))
		if totalTokens == 0 {
			totalTokens = 1
		}
	} else {
		// batchEmbedContents call
		targetURL := GeminiActionURL(baseURL, targetModel, "batchEmbedContents")
		type itemReq struct {
			OutputDimensionality *int   `json:"outputDimensionality,omitempty"`
			Model                string `json:"model"`
			Content              struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		}
		type batchReq struct {
			Requests []itemReq `json:"requests"`
		}
		var bReq batchReq
		modelPath := "models/" + geminiModelPath(targetModel)
		for _, t := range texts {
			toks := model.EstimateTokensFromChars(len(t))
			if toks == 0 {
				toks = 1
			}
			totalTokens += toks
			bReq.Requests = append(bReq.Requests, itemReq{
				OutputDimensionality: req.Dimensions,
				Model:                modelPath,
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
		if err := ConfigureGoogleRequest(ctx, httpReq, channel); err != nil {
			return nil, err
		}

		resp, err := p.client.Do(httpReq)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return nil, NewUpstreamError(channel.Name, resp.StatusCode, []byte(body), resp.Header)
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
			Estimated:    true,
			PromptTokens: totalTokens,
			TotalTokens:  totalTokens,
		},
	}, nil
}
