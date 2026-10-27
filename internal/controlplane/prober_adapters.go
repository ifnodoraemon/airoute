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

// normalizeProbeAPIRoot strips a trailing slash and an API version suffix the
// operator may have included in the configured base URL, so exactly one version
// prefix is appended by the caller.
//
// This is what makes a custom proxy address usable: the previous Gemini and
// Anthropic probes ignored the supplied base URL entirely and always called the
// official host, so probing a self-hosted proxy reported the vendor's public
// status instead of the proxy's.
func normalizeProbeAPIRoot(baseURL string, versions ...string) string {
	root := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	for _, version := range versions {
		if strings.HasSuffix(strings.ToLower(root), version) {
			root = strings.TrimRight(root[:len(root)-len(version)], "/")
			break
		}
	}
	return root
}

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

func (p *DownstreamProber) probeOllama(ctx context.Context, baseURL string, start time.Time) (*ProbeResult, error) {
	targetURL := baseURL + "/api/tags"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	resp, err := p.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("not ollama")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var parsed struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Models) == 0 {
		return nil, fmt.Errorf("empty ollama models")
	}

	dur := time.Since(start).Milliseconds()
	var models []string
	for _, m := range parsed.Models {
		models = append(models, m.Name)
	}

	return &ProbeResult{
		Type:          model.ProviderOpenAI,
		SuggestedName: "ollama-local",
		Models:        models,
		Protocols:     inferProtocols(models),
		LatencyMs:     dur,
		Message:       fmt.Sprintf("已成功连接本地 Ollama 推理引擎，发现 %d 个模型", len(models)),
	}, nil
}

func (p *DownstreamProber) probeGPUStack(ctx context.Context, baseURL, apiKey string, start time.Time) (*ProbeResult, error) {
	rootURL := baseURL
	if idx := strings.Index(rootURL, "/v1-openai"); idx != -1 {
		rootURL = rootURL[:idx]
	} else if idx := strings.Index(rootURL, "/v1"); idx != -1 {
		rootURL = rootURL[:idx]
	}
	rootURL = strings.TrimRight(rootURL, "/")

	isGPUStack := false
	versionStr := ""
	dur := time.Since(start).Milliseconds()

	// 1. Probe /version
	vCtx, vCancel := context.WithTimeout(ctx, 3*time.Second)
	defer vCancel()

	versionURL := rootURL + "/version"
	vReq, err := http.NewRequestWithContext(vCtx, http.MethodGet, versionURL, nil)
	if err == nil {
		if vResp, err := p.client.Do(vReq); err == nil {
			defer vResp.Body.Close()
			if vResp.StatusCode == http.StatusOK {
				body, _ := io.ReadAll(vResp.Body)
				var vData struct {
					Version   string `json:"version"`
					GitCommit string `json:"git_commit"`
				}
				if json.Unmarshal(body, &vData) == nil && (vData.GitCommit != "" || strings.HasPrefix(vData.Version, "v")) {
					isGPUStack = true
					versionStr = vData.Version
				}
			}
		}
	}

	// 2. If not detected via /version, probe root / for HTML signatures
	if !isGPUStack {
		rCtx, rCancel := context.WithTimeout(ctx, 3*time.Second)
		defer rCancel()
		rootReq, err := http.NewRequestWithContext(rCtx, http.MethodGet, rootURL, nil)
		if err == nil {
			if rootResp, err := p.client.Do(rootReq); err == nil {
				defer rootResp.Body.Close()
				body, _ := io.ReadAll(io.LimitReader(rootResp.Body, 16384))
				bodyStr := string(body)
				if strings.Contains(bodyStr, "<title>GPUStack</title>") || strings.Contains(bodyStr, "data-version=\"v") {
					isGPUStack = true
					if idx := strings.Index(bodyStr, "data-version=\""); idx != -1 {
						sub := bodyStr[idx+14:]
						if end := strings.Index(sub, "\""); end != -1 {
							versionStr = sub[:end]
						}
					}
				}
			}
		}
	}

	if !isGPUStack && !strings.Contains(strings.ToLower(baseURL), "gpustack") {
		return nil, nil
	}

	// Definitively GPUStack!
	suggestedBaseURL := rootURL + "/v1-openai"
	if strings.HasSuffix(baseURL, "/v1-openai") {
		suggestedBaseURL = baseURL
	}

	// Try reading models if apiKey provided
	if apiKey != "" && apiKey != "none" {
		mCtx, mCancel := context.WithTimeout(ctx, 5*time.Second)
		defer mCancel()
		modelReq, err := http.NewRequestWithContext(mCtx, http.MethodGet, suggestedBaseURL+"/models", nil)
		if err == nil {
			modelReq.Header.Set("Authorization", "Bearer "+apiKey)
			if modelResp, err := p.client.Do(modelReq); err == nil {
				defer modelResp.Body.Close()
				if modelResp.StatusCode == http.StatusOK {
					body, _ := io.ReadAll(modelResp.Body)
					var parsed struct {
						Data []struct {
							ID string `json:"id"`
						} `json:"data"`
					}
					if json.Unmarshal(body, &parsed) == nil && len(parsed.Data) > 0 {
						var list []string
						for _, d := range parsed.Data {
							if d.ID != "" {
								list = append(list, d.ID)
							}
						}
						return &ProbeResult{
							Type:             model.ProviderGPUStack,
							SuggestedName:    "gpustack-cluster",
							SuggestedBaseURL: suggestedBaseURL,
							Models:           list,
							Protocols:        inferProtocols(list),
							LatencyMs:        time.Since(start).Milliseconds(),
							Message:          fmt.Sprintf("已成功识别 GPUStack 企业算力集群 (版本 %s) 并自动同步 %d 个已部署模型", versionStr, len(list)),
						}, nil
					}
				} else if modelResp.StatusCode == http.StatusUnauthorized {
					return &ProbeResult{
						Type:             model.ProviderGPUStack,
						SuggestedName:    "gpustack-cluster",
						SuggestedBaseURL: suggestedBaseURL,
						Models:           []string{},
						Protocols:        []string{"openai_chat", "openai_response", "openai_text", "embeddings", "rerank", "images"},
						LatencyMs:        time.Since(start).Milliseconds(),
						Message:          fmt.Sprintf("已成功识别 GPUStack 企业集群 (版本 %s)，但提供的 API Key 鉴权失败 (401 Unauthorized)，请检查 API Key 后重试。", versionStr),
					}, nil
				}
			}
		}
	}

	// Authentication required (no apiKey provided)
	verMsg := ""
	if versionStr != "" {
		verMsg = fmt.Sprintf("版本 %s，", versionStr)
	}

	return &ProbeResult{
		Type:             model.ProviderGPUStack,
		SuggestedName:    "gpustack-cluster",
		SuggestedBaseURL: suggestedBaseURL,
		Models:           []string{},
		Protocols:        []string{"openai_chat", "openai_response", "openai_text", "embeddings", "rerank", "images"},
		LatencyMs:        dur,
		Message:          fmt.Sprintf("已成功识别并连通 GPUStack 企业算力集群 (%s响应延迟 %dms)！检测到下游开启了访问密钥保护 (401 Unauthorized)，请在上方填入 API Key 后点击【获取模型列表】以拉取部署的模型。", verMsg, dur),
	}, nil
}
