package controlplane

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// validateProbeURL verifies that the downstream URL does not target forbidden metadata endpoints (SSRF prevention).
func validateProbeURL(targetURL string) error {
	u, err := url.Parse(targetURL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	hostname := u.Hostname()
	if hostname == "" {
		return fmt.Errorf("empty hostname in URL")
	}

	// Always block cloud metadata link-local addresses (AWS/GCP/Azure/Alibaba metadata endpoint 169.254.169.254)
	if hostname == "169.254.169.254" || strings.HasPrefix(hostname, "169.254.") {
		return fmt.Errorf("probing cloud metadata IP (%s) is strictly forbidden for security", hostname)
	}

	// Strict SSRF protection mode if enabled
	if os.Getenv("GATEWAY_BLOCK_PRIVATE_PROBE") == "true" {
		ips, err := net.LookupIP(hostname)
		if err == nil {
			for _, ip := range ips {
				if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
					return fmt.Errorf("probing private/loopback IP address (%s) is forbidden in strict mode", ip.String())
				}
			}
		}
	}
	return nil
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

	// Order canonically
	ordered := []string{"openai_chat", "openai_response", "openai_text", "anthropic_messages", "embeddings", "rerank", "images", "audio_speech", "audio_transcription", "videos"}
	var list []string
	for _, p := range ordered {
		if protocolsMap[p] {
			list = append(list, p)
		}
	}
	return list
}
