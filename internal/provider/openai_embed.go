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
	nativeModel := ""
	switch request := payload.(type) {
	case *model.EmbeddingRequest:
		if request.Native != nil && request.Native.Protocol == model.NativeEmbeddings {
			payloadBytes = RewriteNativeModel(request.Native.Body, request.Model)
			nativeModel = request.Model
		}
	case *model.RerankRequest:
		if request.Native != nil && request.Native.Protocol == model.NativeRerank {
			payloadBytes = RewriteNativeModel(request.Native.Body, request.Model)
			nativeModel = request.Model
		}
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
		return NewUpstreamError(channel.Name, resp.StatusCode, bodyBytes, resp.Header)
	}

	decodeErr := json.Unmarshal(bodyBytes, out)
	if nativeModel != "" && json.Valid(bodyBytes) {
		switch response := out.(type) {
		case *model.EmbeddingResponse:
			response.NativeResponse = bodyBytes
			// base64 vectors are valid native responses even though the canonical
			// float-vector representation cannot decode them.
			var counters struct {
				Usage model.EmbeddingUsage `json:"usage"`
			}
			_ = json.Unmarshal(bodyBytes, &counters)
			response.Usage = counters.Usage
		case *model.RerankResponse:
			response.NativeResponse = bodyBytes
		}
		return nil
	}
	if decodeErr != nil {
		return fmt.Errorf("unmarshal %s response error: %w", label, decodeErr)
	}
	return nil
}

// Embed executes an embedding generation request against standard OpenAI-compatible endpoints.
func (p *OpenAIProvider) Embed(ctx context.Context, req *model.EmbeddingRequest, channel *model.ChannelConfig) (*model.EmbeddingResponse, error) {
	if req.Native != nil && req.Native.TranslationError != nil {
		return nil, &UnsupportedTranslationError{Reason: req.Native.TranslationError.Error()}
	}
	cloned := *req
	cloned.Model = channel.GetUpstreamModel(req.Model)

	var embResp model.EmbeddingResponse
	if err := p.postJSON(ctx, channel, "/v1/embeddings", "embedding", &cloned, &embResp); err != nil {
		return nil, err
	}

	// Restore user's requested model in the response
	embResp.Model = req.Model
	if len(embResp.NativeResponse) > 0 {
		embResp.NativeResponse = RewriteNativeModel(embResp.NativeResponse, req.Model)
	}
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
	if len(rerankResp.NativeResponse) > 0 {
		rerankResp.NativeResponse = RewriteNativeModel(rerankResp.NativeResponse, req.Model)
	}
	return &rerankResp, nil
}
