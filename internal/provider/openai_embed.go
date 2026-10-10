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

// Embed executes an embedding generation request against standard OpenAI-compatible endpoints.
func (p *OpenAIProvider) Embed(ctx context.Context, req *model.EmbeddingRequest, channel *model.ChannelConfig) (*model.EmbeddingResponse, error) {
	targetModel := channel.GetUpstreamModel(req.Model)
	cloned := *req
	cloned.Model = targetModel

	payloadBytes, err := json.Marshal(cloned)
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request error: %w", err)
	}

	targetURL := strings.TrimRight(channel.BaseURL, "/") + "/embeddings"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("create embedding http request error: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if channel.APIKey != "" && channel.APIKey != "none" {
		httpReq.Header.Set("Authorization", "Bearer "+channel.APIKey)
		httpReq.Header.Set("x-api-key", channel.APIKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do embedding request error: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read embedding response error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream embedding %s returned %d: %s", channel.Name, resp.StatusCode, string(bodyBytes))
	}

	var embResp model.EmbeddingResponse
	if err := json.Unmarshal(bodyBytes, &embResp); err != nil {
		return nil, fmt.Errorf("unmarshal embedding response error: %w", err)
	}

	// Restore user's requested model in the response
	embResp.Model = req.Model
	return &embResp, nil
}

// Rerank executes a cross-encoder rerank request against standard rerank endpoints.
func (p *OpenAIProvider) Rerank(ctx context.Context, req *model.RerankRequest, channel *model.ChannelConfig) (*model.RerankResponse, error) {
	targetModel := channel.GetUpstreamModel(req.Model)
	cloned := *req
	cloned.Model = targetModel

	payloadBytes, err := json.Marshal(cloned)
	if err != nil {
		return nil, fmt.Errorf("marshal rerank request error: %w", err)
	}

	baseURL := strings.TrimRight(channel.BaseURL, "/")
	var targetURL string
	if strings.HasSuffix(baseURL, "/v1") {
		targetURL = baseURL + "/rerank"
	} else {
		targetURL = baseURL + "/v1/rerank"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("create rerank http request error: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if channel.APIKey != "" && channel.APIKey != "none" {
		httpReq.Header.Set("Authorization", "Bearer "+channel.APIKey)
		httpReq.Header.Set("x-api-key", channel.APIKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do rerank request error: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read rerank response error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream rerank %s returned %d: %s", channel.Name, resp.StatusCode, string(bodyBytes))
	}

	var rerankResp model.RerankResponse
	if err := json.Unmarshal(bodyBytes, &rerankResp); err != nil {
		return nil, fmt.Errorf("unmarshal rerank response error: %w", err)
	}

	// Restore user's requested model in the response
	rerankResp.Model = req.Model
	return &rerankResp, nil
}
