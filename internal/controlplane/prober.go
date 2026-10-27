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
		client = safeProbeClient(15 * time.Second)
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

	// If every catalogue probe failed, the host may still be reachable — a proxy
	// that requires a credential answers 401/404/405 to all of them. Confirm that
	// with one request to the service root before reporting anything: the old code
	// returned a best-effort success-looking result unconditionally, so an offline
	// proxy (DNS failure, connection refused, timeout) also produced a result the
	// console treated as a usable channel. A host that never answered now fails
	// the probe, which the handler renders as 400.
	reachStatus, reachErr := p.probeReachability(ctx, baseURL)
	if reachErr != nil {
		return nil, fmt.Errorf("无法连接下游服务 %s: %w", baseURL, reachErr)
	}

	dur := time.Since(start).Milliseconds()
	detectedType := inferProviderType(baseURL, "", nil)
	reason := "模型列表接口未返回可解析的清单"
	if err != nil {
		reason = err.Error()
	}
	// Deliberately does not claim success: the host answered, but nothing about
	// the catalogue or the credential was verified.
	msg := fmt.Sprintf("下游服务主机已响应 (HTTP %d)，但未能自动读取模型列表（%s）。请填写 API Key 后重试，或在下方手动添加模型。", reachStatus, reason)
	return &ProbeResult{
		Type:          detectedType,
		SuggestedName: suggestProviderName(detectedType, baseURL, ""),
		Models:        []string{},
		Protocols:     []string{"openai_chat", "openai_response", "openai_text"},
		LatencyMs:     dur,
		Message:       msg,
	}, nil
}

// probeReachability asks the service root whether anything is listening.
//
// Any HTTP response counts as reachable, including 401, 403 and 404: they prove a
// proxy or upstream answered. A transport error (DNS, connection refused, TLS
// failure, timeout) means the host is not usable as a channel. A response is
// returned even when the client also reports an error, because a redirect policy
// refusal carries the redirecting response.
func (p *DownstreamProber) probeReachability(ctx context.Context, baseURL string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL, nil)
	if err != nil {
		return 0, err
	}
	resp, err := p.client.Do(req)
	if resp != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		return resp.StatusCode, nil
	}
	if err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("no response from %s", baseURL)
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
		// Consume and close each response immediately: `defer` inside this loop
		// would hold every candidate connection open until the probe returns.
		// The read is bounded so a hostile endpoint cannot exhaust memory.
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}

		if resp.StatusCode == http.StatusOK {
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
