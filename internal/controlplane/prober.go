package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

// ProbeRequest contains connection parameters for probing downstream services.
type ProbeRequest struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Type    string `json:"type,omitempty"`
}

// ProbeResult contains automatically discovered downstream capabilities.
type ProbeResult struct {
	Type             model.ProviderType `json:"type"`
	SuggestedName    string             `json:"suggested_name"`
	SuggestedBaseURL string             `json:"suggested_base_url,omitempty"`
	Models           []string           `json:"models"`
	Protocols        []string           `json:"protocols"`
	LatencyMs        int64              `json:"latency_ms"`
	ServerHeader     string             `json:"server_header,omitempty"`
	Message          string             `json:"message"`
}

// DownstreamProber handles automatic probing and discovery of downstream LLM engines.
type DownstreamProber struct {
	client *http.Client
}

// NewDownstreamProber creates a DownstreamProber.
func NewDownstreamProber(client *http.Client) *DownstreamProber {
	if client == nil {
		client = provider.SharedDefaultHTTPClient
	}
	return &DownstreamProber{client: client}
}

// Probe automatically tests and inspects an upstream service.
func (p *DownstreamProber) Probe(ctx context.Context, req *ProbeRequest) (*ProbeResult, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("base_url is required")
	}

	// Auto-normalize URL scheme if user only entered hostname/ip/port
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		if strings.Contains(baseURL, "api.") || strings.Contains(baseURL, ".com") || strings.Contains(baseURL, ".org") || strings.Contains(baseURL, ".net") || strings.Contains(baseURL, ".ai") || strings.Contains(baseURL, "google") {
			baseURL = "https://" + baseURL
		} else {
			baseURL = "http://" + baseURL
		}
	}

	if err := validateProbeURL(baseURL); err != nil {
		return nil, err
	}

	start := time.Now()

	// 1. Check for GPUStack cluster (probe /version or root / for GPUStack signatures)
	if gpustackRes, _ := p.probeGPUStack(ctx, baseURL, req.APIKey, start); gpustackRes != nil {
		return gpustackRes, nil
	}

	// 2. Check for Google Gemini Developer API
	if strings.Contains(baseURL, "generativelanguage.googleapis.com") || req.Type == "gemini" {
		return p.probeGemini(ctx, baseURL, req.APIKey, start)
	}

	// 3. Check for Anthropic Claude direct API
	if strings.Contains(baseURL, "api.anthropic.com") || req.Type == "anthropic" {
		return p.probeAnthropic(ctx, baseURL, req.APIKey, start)
	}

	// 4. Probe standard OpenAI-compatible endpoints (/v1/models or /models)
	res, err := p.probeOpenAICompatible(ctx, baseURL, req.APIKey, start)
	if err == nil {
		return res, nil
	}

	// 5. Probe Ollama /api/tags
	ollamaRes, errOllama := p.probeOllama(ctx, baseURL, start)
	if errOllama == nil {
		return ollamaRes, nil
	}

	// If all probes fail, return a best-effort default result with latency
	dur := time.Since(start).Milliseconds()
	serverHeader := ""
	detectedType := inferProviderType(baseURL, serverHeader, nil)
	msg := "已连接下游服务，但该服务未开放无凭证的模型列表接口。您可以输入 API Key 重新拉取，或在下方手动添加模型。"
	if err != nil && (strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "Unauthorized")) {
		msg = "已成功连通下游服务，但该服务开启了访问鉴权保护 (401 Unauthorized)。请在上方填入 API Key 后再次获取模型。"
	} else if err != nil {
		msg = fmt.Sprintf("服务已连通 (响应延迟 %dms)，但自动读取模型列表未成功: %v。请在下方手动输入所需模型。", dur, err)
	}
	return &ProbeResult{
		Type:          detectedType,
		SuggestedName: suggestProviderName(detectedType, baseURL, ""),
		Models:        []string{},
		Protocols:     []string{"openai_chat", "openai_response", "openai_text"},
		LatencyMs:     dur,
		Message:       msg,
	}, nil
}

func (p *DownstreamProber) probeOpenAICompatible(ctx context.Context, baseURL, apiKey string, start time.Time) (*ProbeResult, error) {
	testEndpoints := []string{
		baseURL + "/v1-openai/models",
		baseURL + "/v1/models",
		baseURL + "/models",
	}
	if strings.HasSuffix(baseURL, "/v1") || strings.HasSuffix(baseURL, "/v1-openai") {
		testEndpoints = []string{baseURL + "/models"}
	}

	var lastErr error
	for _, targetURL := range testEndpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
		if err != nil {
			lastErr = err
			continue
		}

		if apiKey != "" && apiKey != "none" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}

		resp, err := p.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			var parsed struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			if err := json.Unmarshal(body, &parsed); err == nil && len(parsed.Data) > 0 {
				var modelIDs []string
				for _, d := range parsed.Data {
					if d.ID != "" {
						modelIDs = append(modelIDs, d.ID)
					}
				}

				dur := time.Since(start).Milliseconds()
				serverHeader := resp.Header.Get("Server")
				detectedType := inferProviderType(baseURL, serverHeader, modelIDs)
				protocols := inferProtocols(modelIDs)

				// Lightweight active check for /v1/rerank if not yet detected from model names
				if !contains(protocols, "rerank") {
					rerankURL := strings.TrimRight(baseURL, "/") + "/rerank"
					if !strings.HasSuffix(baseURL, "/v1") {
						rerankURL = strings.TrimRight(baseURL, "/") + "/v1/rerank"
					}
					rReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, rerankURL, bytes.NewReader([]byte("{}")))
					rReq.Header.Set("Content-Type", "application/json")
					if apiKey != "" && apiKey != "none" {
						rReq.Header.Set("Authorization", "Bearer "+apiKey)
					}
					if rResp, err := p.client.Do(rReq); err == nil {
						rResp.Body.Close()
						if rResp.StatusCode == http.StatusBadRequest || rResp.StatusCode == http.StatusUnprocessableEntity || rResp.StatusCode == http.StatusOK {
							protocols = append(protocols, "rerank")
						}
					}
				}

				suggestedName := suggestProviderName(detectedType, baseURL, serverHeader)

				return &ProbeResult{
					Type:          detectedType,
					SuggestedName: suggestedName,
					Models:        modelIDs,
					Protocols:     protocols,
					LatencyMs:     dur,
					ServerHeader:  serverHeader,
					Message:       fmt.Sprintf("成功检测到 %d 个下游模型", len(modelIDs)),
				}, nil
			}
		} else if resp.StatusCode == http.StatusUnauthorized {
			lastErr = fmt.Errorf("status code 401 (Unauthorized: 下游开启了访问凭证保护，请输入 API Key 提取在线模型列表)")
		} else {
			lastErr = fmt.Errorf("status code %d", resp.StatusCode)
		}
	}

	return nil, lastErr
}
