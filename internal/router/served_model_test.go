package router

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func TestResolveServedModelFollowsChannelMapping(t *testing.T) {
	dispatcher := NewDispatcher([]model.ChannelConfig{{
		Name:         "罗总",
		Type:         model.ProviderOpenAI,
		Models:       []string{"deepseek-v4.1-flash", "volcengine/deepseek-v4-1-flash-261017"},
		ModelMapping: map[string]string{"deepseek-v4.1-flash": "volcengine/deepseek-v4-1-flash-261017"},
		Status:       "active",
		Weight:       1,
	}})

	if got := dispatcher.ResolveServedModel("罗总", "deepseek-v4.1-flash"); got != "volcengine/deepseek-v4-1-flash-261017" {
		t.Fatalf("served model = %q, want the mapped upstream model", got)
	}
	if got := dispatcher.ResolveServedModel("罗总", "aliyun/glm-5.3"); got != "aliyun/glm-5.3" {
		t.Fatalf("unmapped model = %q, want the requested model unchanged", got)
	}
	if got := dispatcher.ResolveServedModel("missing", "deepseek-v4.1-flash"); got != "" {
		t.Fatalf("unknown channel = %q, want empty", got)
	}
}
