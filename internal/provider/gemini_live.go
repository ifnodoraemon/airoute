package provider

import (
	"net/url"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func GeminiLiveURL(ch *model.ChannelConfig, version string) string {
	u, err := url.Parse(GeminiBaseURL(ch))
	if err != nil {
		return ""
	}
	if u.Scheme == "http" {
		u.Scheme = "ws"
	} else if u.Scheme == "https" {
		u.Scheme = "wss"
	}
	prefix := strings.TrimSuffix(strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/v1beta"), "/v1")
	method := "/ws/google.ai.generativelanguage." + version + ".GenerativeService.BidiGenerateContent"
	if IsVertexChannel(ch) {
		if i := strings.Index(prefix, "/v1/projects/"); i >= 0 {
			prefix = prefix[:i]
		}
		method = "/ws/google.cloud.aiplatform.v1beta1.LlmBidiService/BidiGenerateContent"
	}
	u.Path = prefix + method
	u.RawPath = ""
	u.RawQuery = ""
	u.User = nil
	return u.String()
}

func GeminiModelResource(ch *model.ChannelConfig, name string) string {
	if !IsVertexChannel(ch) {
		return "models/" + ch.GetUpstreamModel(name)
	}
	u, _ := url.Parse(GeminiBaseURL(ch))
	return strings.TrimPrefix(strings.TrimPrefix(u.Path, "/v1beta1/"), "/v1/") + "/models/" + ch.GetUpstreamModel(name)
}
