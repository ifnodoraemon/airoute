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

	"github.com/ifnodoraemon/nano-gateway/internal/model"
	"github.com/ifnodoraemon/nano-gateway/internal/provider"
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

// inferProviderType guesses the specific provider category from headers, URL, and model IDs.
func inferProviderType(baseURL, serverHeader string, models []string) model.ProviderType {
	lowerURL := strings.ToLower(baseURL)
	lowerServer := strings.ToLower(serverHeader)

	if strings.Contains(lowerURL, "gpustack") || strings.Contains(lowerServer, "gpustack") {
		return model.ProviderGPUStack
	}
	if strings.Contains(lowerURL, "sub2api") {
		return model.ProviderSub2API
	}
	if strings.Contains(lowerURL, "deepseek.com") {
		return model.ProviderDeepSeek
	}
	if strings.Contains(lowerURL, "openai.com") {
		return model.ProviderOpenAI
	}
	if strings.Contains(lowerURL, "anthropic.com") {
		return model.ProviderAnthropic
	}
	if strings.Contains(lowerURL, "generativelanguage.googleapis.com") {
		return model.ProviderGemini
	}
	if strings.Contains(lowerServer, "vllm") || strings.Contains(lowerURL, ":8000") {
		return model.ProviderVLLM
	}
	if strings.Contains(lowerServer, "sglang") || strings.Contains(lowerURL, ":30000") {
		return model.ProviderSGLang
	}
	if strings.Contains(lowerServer, "ollama") || strings.Contains(lowerURL, ":11434") {
		return model.ProviderOllama
	}

	return model.ProviderOpenAI
}

// suggestProviderName creates a clean, recognizable name for the provider.
func suggestProviderName(detectedType model.ProviderType, baseURL, serverHeader string) string {
	lowerURL := strings.ToLower(baseURL)
	lowerServer := strings.ToLower(serverHeader)

	if strings.Contains(lowerServer, "gpustack") || strings.Contains(lowerURL, "gpustack") {
		return "gpustack-cluster"
	}
	if strings.Contains(lowerURL, "deepseek.com") {
		return "deepseek-direct"
	}
	if strings.Contains(lowerURL, "openai.com") {
		return "openai-official-us"
	}
	if strings.Contains(lowerURL, "anthropic.com") {
		return "anthropic-claude-direct"
	}
	if strings.Contains(lowerURL, "googleapis.com") {
		return "google-gemini-official"
	}
	if strings.Contains(lowerURL, "sub2api") {
		return "sub2api-upstream"
	}
	if strings.Contains(lowerURL, ":11434") || strings.Contains(lowerServer, "ollama") {
		return "ollama-local"
	}
	if strings.Contains(lowerServer, "vllm") || strings.Contains(lowerURL, ":8000") {
		return "vllm-engine"
	}
	if strings.Contains(lowerServer, "sglang") || strings.Contains(lowerURL, ":30000") {
		return "sglang-engine"
	}
	if strings.Contains(lowerURL, "localhost") || strings.Contains(lowerURL, "127.0.0.1") || strings.Contains(lowerURL, "192.168.") || strings.Contains(lowerURL, "10.") {
		return "local-inference-cluster"
	}
	return string(detectedType) + "-upstream"
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// inferProtocols determines supported modalities and protocols based on model names.
func inferProtocols(models []string) []string {
	protocolsMap := map[string]bool{
		"openai_chat":        true,
		"openai_response":    true,
		"openai_text":        true,
		"anthropic_messages": true,
	}

	for _, m := range models {
		lower := strings.ToLower(m)
		if strings.Contains(lower, "dall-e") || strings.Contains(lower, "flux") || strings.Contains(lower, "stable-diffusion") || strings.Contains(lower, "sd") || strings.Contains(lower, "seedream") || strings.Contains(lower, "midjourney") {
			protocolsMap["images"] = true
		}
		if strings.Contains(lower, "tts") || strings.Contains(lower, "speech") || strings.Contains(lower, "cosyvoice") || strings.Contains(lower, "chattts") {
			protocolsMap["audio_speech"] = true
		}
		if strings.Contains(lower, "whisper") || strings.Contains(lower, "transcription") || strings.Contains(lower, "sensevoice") || strings.Contains(lower, "funasr") {
			protocolsMap["audio_transcription"] = true
		}
		if strings.Contains(lower, "sora") || strings.Contains(lower, "cogvideo") || strings.Contains(lower, "kling") || strings.Contains(lower, "video") || strings.Contains(lower, "seedance") || strings.Contains(lower, "luma") || strings.Contains(lower, "runway") || strings.Contains(lower, "pika") || strings.Contains(lower, "wan") || strings.Contains(lower, "hunyuan") || strings.Contains(lower, "vidu") || strings.Contains(lower, "minimax-video") {
			protocolsMap["videos"] = true
		}
		if strings.Contains(lower, "embed") || strings.Contains(lower, "bge") || strings.Contains(lower, "e5") || strings.Contains(lower, "nomic") || strings.Contains(lower, "voyage") || strings.Contains(lower, "jina") || strings.Contains(lower, "text-embedding") {
			protocolsMap["embeddings"] = true
		}
		if strings.Contains(lower, "rerank") || strings.Contains(lower, "bge-rerank") || strings.Contains(lower, "colbert") || strings.Contains(lower, "gte-rerank") {
			protocolsMap["rerank"] = true
		}
	}

	var list []string
	// Order canonically
	ordered := []string{"openai_chat", "openai_response", "openai_text", "anthropic_messages", "embeddings", "rerank", "images", "audio_speech", "audio_transcription", "videos"}
	for _, p := range ordered {
		if protocolsMap[p] {
			list = append(list, p)
		}
	}
	return list
}
