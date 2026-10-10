package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func (p *DownstreamProber) probeGemini(ctx context.Context, baseURL, apiKey string, start time.Time) (*ProbeResult, error) {
	targetURL := "https://generativelanguage.googleapis.com/v1beta/models"
	if apiKey != "" && apiKey != "none" {
		targetURL += "?key=" + apiKey
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	resp, err := p.client.Do(req)
	dur := time.Since(start).Milliseconds()

	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var parsed struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}
		if json.Unmarshal(body, &parsed) == nil && len(parsed.Models) > 0 {
			var list []string
			for _, m := range parsed.Models {
				cleanName := strings.TrimPrefix(m.Name, "models/")
				list = append(list, cleanName)
			}
			return &ProbeResult{
				Type:          model.ProviderGemini,
				SuggestedName: "google-gemini-official",
				Models:        list,
				Protocols:     inferProtocols(list),
				LatencyMs:     dur,
				Message:       fmt.Sprintf("成功连接 Google Gemini 官方接口，读取到 %d 个模型", len(list)),
			}, nil
		}
	}

	return &ProbeResult{
		Type:          model.ProviderGemini,
		SuggestedName: "google-gemini-official",
		Models:        []string{},
		Protocols:     []string{"openai_chat", "anthropic_messages", "embeddings"},
		LatencyMs:     dur,
		Message:       "已识别 Google Gemini 官方协议。请输入 API Key 后再次点击【连通测试并拉取模型】以同步在线模型列表，或在下方手动添加。",
	}, nil
}

func (p *DownstreamProber) probeAnthropic(ctx context.Context, baseURL, apiKey string, start time.Time) (*ProbeResult, error) {
	dur := time.Since(start).Milliseconds()

	if apiKey != "" && apiKey != "none" {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.anthropic.com/v1/models", nil)
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		resp, err := p.client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			var parsed struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			if json.Unmarshal(body, &parsed) == nil && len(parsed.Data) > 0 {
				var list []string
				for _, m := range parsed.Data {
					list = append(list, m.ID)
				}
				return &ProbeResult{
					Type:          model.ProviderAnthropic,
					SuggestedName: "anthropic-claude-direct",
					Models:        list,
					Protocols:     []string{"openai_chat", "anthropic_messages"},
					LatencyMs:     dur,
					Message:       fmt.Sprintf("已成功连接 Anthropic 官方 API，在线发现 %d 个模型", len(list)),
				}, nil
			}
		}
	}

	return &ProbeResult{
		Type:          model.ProviderAnthropic,
		SuggestedName: "anthropic-claude-direct",
		Models:        []string{},
		Protocols:     []string{"openai_chat", "anthropic_messages"},
		LatencyMs:     dur,
		Message:       "已识别 Anthropic Claude 原生协议。请输入 API Key 后再次点击【连通测试并拉取模型】以同步在线模型列表，或在下方手动添加。",
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
