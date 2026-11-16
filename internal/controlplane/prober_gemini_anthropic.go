package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// Gemini and Anthropic catalogue probes, extracted from prober_adapters.go to
// keep the probe adapter file under the size red line.

// probeGemini discovers models from a Gemini-compatible endpoint.
//
// The base URL is honoured, including a custom proxy, and a request is issued
// even without an API key: a reachability failure must fail the probe instead of
// reporting a configured channel that can never be reached.
func (p *DownstreamProber) probeGemini(ctx context.Context, baseURL, apiKey string, start time.Time) (*ProbeResult, error) {
	apiRoot := normalizeProbeAPIRoot(baseURL, "/v1beta/models", "/v1/models", "/v1beta", "/v1")
	targetURL := apiRoot + "/v1beta/models"
	suggestedName := suggestProviderName(model.ProviderGemini, baseURL, "")

	hasKey := apiKey != "" && apiKey != "none"
	requestURL := targetURL
	if hasKey {
		requestURL += "?key=" + url.QueryEscape(apiKey)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	dur := time.Since(start).Milliseconds()

	if resp != nil {
		defer resp.Body.Close()
		if hasKey && resp.StatusCode == http.StatusOK {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
			var parsed struct {
				Models []struct {
					Name string `json:"name"`
				} `json:"models"`
			}
			if readErr == nil && json.Unmarshal(body, &parsed) == nil && len(parsed.Models) > 0 {
				var list []string
				for _, m := range parsed.Models {
					cleanName := strings.TrimPrefix(m.Name, "models/")
					list = append(list, cleanName)
				}
				return &ProbeResult{
					Type:          model.ProviderGemini,
					SuggestedName: suggestedName,
					Models:        list,
					Protocols:     inferProtocols(list),
					LatencyMs:     dur,
					Message:       fmt.Sprintf("成功连接 Gemini 协议下游 %s，读取到 %d 个模型", apiRoot, len(list)),
				}, nil
			}
		}
	}

	// A transport error with no response means the host itself is unreachable;
	// surface it so the handler returns 400 rather than a plausible-looking row.
	if resp == nil && err != nil {
		return nil, fmt.Errorf("无法连接 Gemini 协议下游服务 %s: %w", targetURL, err)
	}

	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	// The host answered, but the catalogue was not readable. Do not claim success.
	msg := fmt.Sprintf("Gemini 协议下游服务已响应 (HTTP %d)，但未能读取在线模型列表。请检查账号分组、API Key 与 base_url 是否正确，或在下方手动添加模型。", status)
	if !hasKey {
		msg = fmt.Sprintf("Gemini 协议下游服务已响应 (HTTP %d)，但未填写 API Key，无法读取在线模型列表。请输入 API Key 后重试，或在下方手动添加模型。", status)
	}
	return &ProbeResult{
		Type:          model.ProviderGemini,
		SuggestedName: suggestedName,
		Models:        []string{},
		Protocols:     []string{"openai_chat", "anthropic_messages", "embeddings"},
		LatencyMs:     dur,
		Message:       msg,
	}, nil
}

// probeAnthropic discovers models from an Anthropic-compatible endpoint.
//
// Like probeGemini it uses the configured base URL for both the keyed and the
// keyless probe, and a keyless probe is still a real request so an offline custom
// proxy is reported as a failure.
func (p *DownstreamProber) probeAnthropic(ctx context.Context, baseURL, apiKey string, start time.Time) (*ProbeResult, error) {
	apiRoot := normalizeProbeAPIRoot(baseURL, "/v1/models", "/v1")
	targetURL := apiRoot + "/v1/models"
	suggestedName := suggestProviderName(model.ProviderAnthropic, baseURL, "")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	hasKey := apiKey != "" && apiKey != "none"
	if hasKey {
		req.Header.Set("x-api-key", apiKey)
	}
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(req)
	dur := time.Since(start).Milliseconds()

	if resp != nil {
		defer resp.Body.Close()
		if hasKey && resp.StatusCode == http.StatusOK {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
			var parsed struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			if readErr == nil && json.Unmarshal(body, &parsed) == nil && len(parsed.Data) > 0 {
				var list []string
				for _, m := range parsed.Data {
					list = append(list, m.ID)
				}
				return &ProbeResult{
					Type:          model.ProviderAnthropic,
					SuggestedName: suggestedName,
					Models:        list,
					Protocols:     []string{"openai_chat", "anthropic_messages"},
					LatencyMs:     dur,
					Message:       fmt.Sprintf("已成功连接 Anthropic 协议下游 %s，在线发现 %d 个模型", apiRoot, len(list)),
				}, nil
			}
		}
	}

	if resp == nil && err != nil {
		return nil, fmt.Errorf("无法连接 Anthropic 协议下游服务 %s: %w", targetURL, err)
	}

	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	msg := fmt.Sprintf("Anthropic 协议下游服务已响应 (HTTP %d)，但未能读取在线模型列表。请检查账号分组、API Key 与 base_url 是否正确，或在下方手动添加模型。", status)
	if !hasKey {
		msg = fmt.Sprintf("Anthropic 协议下游服务已响应 (HTTP %d)，但未填写 API Key，无法读取在线模型列表。请输入 API Key 后重试，或在下方手动添加模型。", status)
	}
	return &ProbeResult{
		Type:          model.ProviderAnthropic,
		SuggestedName: suggestedName,
		Models:        []string{},
		Protocols:     []string{"openai_chat", "anthropic_messages"},
		LatencyMs:     dur,
		Message:       msg,
	}, nil
}