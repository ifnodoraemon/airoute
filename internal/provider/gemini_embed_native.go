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

func (p *GeminiProvider) embedNative(ctx context.Context, req *model.EmbeddingRequest, channel *model.ChannelConfig) (*model.EmbeddingResponse, error) {
	target := channel.GetUpstreamModel(req.Model)
	body := RewriteGeminiActionModel(req.Native.Body, target)
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, GeminiActionURL(GeminiBaseURL(channel), target, "embedContent"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	if err := ConfigureGoogleRequest(ctx, r, channel); err != nil {
		return nil, err
	}
	resp, err := p.client.Do(r)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, NewUpstreamError(channel.Name, resp.StatusCode, data, resp.Header)
	}
	var parsed struct {
		Embedding struct {
			Values []float64 `json:"values"`
		} `json:"embedding"`
		UsageMetadata *GeminiUsageMetadata `json:"usageMetadata"`
		Error         json.RawMessage      `json:"error"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Error) > 0 && string(parsed.Error) != "null" {
		return nil, nativeEventError(channel.Name, parsed.Error)
	}
	if len(parsed.Embedding.Values) == 0 {
		return nil, fmt.Errorf("Gemini response is missing embedding values")
	}
	result := &model.EmbeddingResponse{Model: req.Model, Object: "list", NativeResponse: data,
		Data: []model.EmbeddingItem{{Object: "embedding", Index: 0, Embedding: parsed.Embedding.Values}},
	}
	if parsed.UsageMetadata != nil {
		result.Usage.PromptTokens = parsed.UsageMetadata.PromptTokenCount
		result.Usage.TotalTokens = parsed.UsageMetadata.TotalTokenCount
	}
	return result, nil
}
