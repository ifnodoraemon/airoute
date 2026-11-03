package controlplane

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func TestSuggestProviderNameRecognizesDomesticClouds(t *testing.T) {
	cases := []struct {
		baseURL string
		want    string
	}{
		{"https://open.bigmodel.cn/api/paas/v4", "zhipu-glm-direct"},
		{"https://ark.cn-beijing.volces.com/api/v3", "volcengine-doubao-direct"},
		{"https://api.moonshot.cn/v1", "moonshot-kimi-direct"},
		{"https://dashscope.aliyuncs.com/compatible-mode/v1", "aliyun-dashscope-direct"},
		{"https://qianfan.baidubce.com/v2", "baidu-qianfan-direct"},
		{"https://hunyuan.tencentcloudapi.com/v1", "tencent-hunyuan-direct"},
		// Existing vendors must keep their names.
		{"https://api.deepseek.com", "deepseek-direct"},
		{"https://api.openai.com/v1", "openai-official-us"},
		{"https://api.anthropic.com", "anthropic-claude-direct"},
		{"https://generativelanguage.googleapis.com", "google-gemini-official"},
	}
	for _, tc := range cases {
		if got := suggestProviderName(model.ProviderOpenAI, tc.baseURL, ""); got != tc.want {
			t.Errorf("suggestProviderName(%q) = %q, want %q", tc.baseURL, got, tc.want)
		}
	}
}

func TestInferProviderTypeKeepsOpenAICompatibleDomesticClouds(t *testing.T) {
	// Domestic clouds speak the OpenAI wire protocol. A "custom" type would
	// change the adapter the dispatcher selects, so inference must leave them as
	// openai-compatible and only the display name specialises.
	for _, baseURL := range []string{
		"https://open.bigmodel.cn/api/paas/v4",
		"https://ark.cn-beijing.volces.com/api/v3",
		"https://api.moonshot.cn/v1",
		"https://dashscope.aliyuncs.com/compatible-mode/v1",
	} {
		if got := inferProviderType(baseURL, "", nil); got != model.ProviderOpenAI {
			t.Errorf("inferProviderType(%q) = %q, want %q", baseURL, got, model.ProviderOpenAI)
		}
	}
	if got := inferProviderType("https://api.deepseek.com", "", nil); got != model.ProviderDeepSeek {
		t.Errorf("deepseek inference regressed: %q", got)
	}
}
