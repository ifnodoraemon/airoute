package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// postJSON sends a JSON POST to path and decodes a successful response into out.
//
// Embedding and rerank share the whole request plumbing (upstream model rewrite,
// auth headers, upstream error mapping, response decode), so it lives here once.
// label only shapes the error text ("embedding"/"rerank").
func (p *OpenAIProvider) postJSON(ctx context.Context, channel *model.ChannelConfig, path, label string, payload, out any) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal %s request error: %w", label, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, APIURL(channel.BaseURL, path), bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("create %s http request error: %w", label, err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if channel.APIKey != "" && channel.APIKey != "none" {
		httpReq.Header.Set("Authorization", "Bearer "+channel.APIKey)
		httpReq.Header.Set("x-api-key", channel.APIKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("do %s request error: %w", label, err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read %s response error: %w", label, err)
	}

	if resp.StatusCode != http.StatusOK {
		return NewUpstreamError(channel.Name, resp.StatusCode, bodyBytes)
	}

	if err := json.Unmarshal(bodyBytes, out); err != nil {
		return fmt.Errorf("unmarshal %s response error: %w", label, err)
	}
	return nil
}

// Embed executes an embedding generation request against standard OpenAI-compatible endpoints.
func (p *OpenAIProvider) Embed(ctx context.Context, req *model.EmbeddingRequest, channel *model.ChannelConfig) (*model.EmbeddingResponse, error) {
	cloned := *req
	cloned.Model = channel.GetUpstreamModel(req.Model)

	var embResp model.EmbeddingResponse
	if err := p.postJSON(ctx, channel, "/v1/embeddings", "embedding", &cloned, &embResp); err != nil {
		return nil, err
	}

	// Restore user's requested model in the response
	embResp.Model = req.Model
	return &embResp, nil
}

// Rerank executes a cross-encoder rerank request against standard rerank endpoints.
func (p *OpenAIProvider) Rerank(ctx context.Context, req *model.RerankRequest, channel *model.ChannelConfig) (*model.RerankResponse, error) {
	cloned := *req
	cloned.Model = channel.GetUpstreamModel(req.Model)

	var rerankResp model.RerankResponse
	if err := p.postJSON(ctx, channel, "/v1/rerank", "rerank", &cloned, &rerankResp); err != nil {
		return nil, err
	}

	// Restore user's requested model in the response
	rerankResp.Model = req.Model
	return &rerankResp, nil
}
