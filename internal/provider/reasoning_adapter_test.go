package provider

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func TestSeparateReasoningInResponse(t *testing.T) {
	tests := []struct {
		name          string
		inputContent  string
		inputReason   string
		expectedCont  string
		expectedReas  string
	}{
		{
			name:         "GPUStack style without leading think tag",
			inputContent: "我们需要思考...一句话回答。</think>中国的首都是北京。",
			inputReason:  "",
			expectedCont: "中国的首都是北京。",
			expectedReas: "我们需要思考...一句话回答。",
		},
		{
			name:         "Standard think tags",
			inputContent: "<think>深度推导中...</think>\n\n答案是42。",
			inputReason:  "",
			expectedCont: "答案是42。",
			expectedReas: "深度推导中...",
		},
		{
			name:         "Already separated by upstream",
			inputContent: "直接答案",
			inputReason:  "上游推理过程",
			expectedCont: "直接答案",
			expectedReas: "上游推理过程",
		},
		{
			name:         "Normal model without thinking",
			inputContent: "你好，请问有什么可以帮助您？",
			inputReason:  "",
			expectedCont: "你好，请问有什么可以帮助您？",
			expectedReas: "",
		},
		{
			name:         "Truncated thinking without closing tag",
			inputContent: "<think>思考到一半被截断",
			inputReason:  "",
			expectedCont: "",
			expectedReas: "思考到一半被截断",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &model.ChatCompletionResponse{
				Choices: []model.ChatCompletionChoice{
					{
						Message: model.ChatMessage{
							Content:          tt.inputContent,
							ReasoningContent: tt.inputReason,
						},
					},
				},
			}

			SeparateReasoningInResponse(resp)

			actualCont := resp.Choices[0].Message.Content.(string)
			actualReas := resp.Choices[0].Message.ReasoningContent

			if actualCont != tt.expectedCont {
				t.Errorf("content mismatch: got %q, want %q", actualCont, tt.expectedCont)
			}
			if actualReas != tt.expectedReas {
				t.Errorf("reasoning mismatch: got %q, want %q", actualReas, tt.expectedReas)
			}
		})
	}
}

func TestReasoningStreamAdapter(t *testing.T) {
	t.Run("DeepSeek v4.1 reasoning stream with cross-chunk think end", func(t *testing.T) {
		adapter := NewReasoningStreamAdapter("deepseek-v4.1-flash", nil)

		// Simulate chunks from GPUStack
		chunks := []string{
			"我们需要",
			"回答用户中文问题：“9.11和9.8哪个大？”",
			"这是思考尾声。</", // partial </think> across boundary
			"think>按数值比较：",
			"9.8 比 9.11 大。",
		}

		var collectedReasoning string
		var collectedContent string

		for _, chunkText := range chunks {
			rawChunk := &model.ChatCompletionChunk{
				Choices: []model.ChunkChoice{
					{
						Delta: model.ChunkDelta{
							Content: chunkText,
						},
					},
				},
			}

			processed := adapter.ProcessChunk(rawChunk)
			for _, pc := range processed {
				if pc.Choices[0].Delta.ReasoningContent != "" {
					collectedReasoning += pc.Choices[0].Delta.ReasoningContent
				}
				if pc.Choices[0].Delta.Content != "" {
					collectedContent += pc.Choices[0].Delta.Content
				}
			}
		}

		if collectedReasoning != "我们需要回答用户中文问题：“9.11和9.8哪个大？”这是思考尾声。" {
			t.Errorf("collectedReasoning mismatch: got %q", collectedReasoning)
		}
		if collectedContent != "按数值比较：9.8 比 9.11 大。" {
			t.Errorf("collectedContent mismatch: got %q", collectedContent)
		}
	})

	t.Run("Normal model non-reasoning stream", func(t *testing.T) {
		adapter := NewReasoningStreamAdapter("gpt-4o", nil)

		chunks := []string{"Hello", ", ", "world", "!"}
		var collectedReasoning string
		var collectedContent string

		for _, chunkText := range chunks {
			rawChunk := &model.ChatCompletionChunk{
				Choices: []model.ChunkChoice{
					{
						Delta: model.ChunkDelta{
							Content: chunkText,
						},
					},
				},
			}
			for _, pc := range adapter.ProcessChunk(rawChunk) {
				collectedReasoning += pc.Choices[0].Delta.ReasoningContent
				collectedContent += pc.Choices[0].Delta.Content
			}
		}

		if collectedReasoning != "" {
			t.Errorf("expected no reasoning, got %q", collectedReasoning)
		}
		if collectedContent != "Hello, world!" {
			t.Errorf("content mismatch: got %q", collectedContent)
		}
	})

	t.Run("Upstream already emitting reasoning_content passthrough", func(t *testing.T) {
		adapter := NewReasoningStreamAdapter("deepseek-r1", nil)

		rawChunk := &model.ChatCompletionChunk{
			Choices: []model.ChunkChoice{
				{
					Delta: model.ChunkDelta{
						ReasoningContent: "native reasoning",
					},
				},
			},
		}

		processed := adapter.ProcessChunk(rawChunk)
		if len(processed) != 1 || processed[0].Choices[0].Delta.ReasoningContent != "native reasoning" {
			t.Errorf("expected native reasoning passthrough")
		}
	})
}
