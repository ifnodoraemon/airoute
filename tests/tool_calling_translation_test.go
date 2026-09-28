package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/router"
)

func TestOpenAIToAnthropic_ToolCallingTranslation(t *testing.T) {
	var capturedPayload map[string]any

	anthropicServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedPayload)

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"id": "msg_anthropic_tools_123",
			"type": "message",
			"role": "assistant",
			"model": "claude-3-5-sonnet",
			"content": [
				{
					"type": "text",
					"text": "I will check the weather for you."
				},
				{
					"type": "tool_use",
					"id": "toolu_weather_001",
					"name": "get_current_weather",
					"input": {
						"location": "Tokyo",
						"unit": "celsius"
					}
				}
			],
			"stop_reason": "tool_use",
			"usage": {
				"input_tokens": 55,
				"output_tokens": 42
			}
		}`)
	}))
	defer anthropicServer.Close()

	ch := &model.ChannelConfig{
		Name:     "anthropic-upstream",
		Type:     model.ProviderAnthropic,
		BaseURL:  anthropicServer.URL,
		APIKey:   "sk-ant-test",
		Models:   []string{"claude-3-5-sonnet"},
		Priority: 1,
		Weight:   10,
	}

	prov := provider.NewAnthropicProvider(nil)

	req := &model.ChatCompletionRequest{
		Model: "claude-3-5-sonnet",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "What is the weather in Tokyo?"},
		},
		Tools: []model.Tool{
			{
				Type: "function",
				Function: map[string]any{
					"name":        "get_current_weather",
					"description": "Get current weather conditions in a given location",
					"parameters": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"location": map[string]any{"type": "string"},
							"unit":     map[string]any{"type": "string", "enum": []any{"celsius", "fahrenheit"}},
						},
						"required": []any{"location"},
					},
				},
			},
		},
	}

	resp, err := prov.ChatComplete(context.Background(), req, ch)
	if err != nil {
		t.Fatalf("Anthropic ChatComplete error: %v", err)
	}

	// 1. Verify outbound request translation to Anthropic format
	toolsAny, exists := capturedPayload["tools"]
	if !exists {
		t.Fatalf("Tools not forwarded to Anthropic upstream: %+v", capturedPayload)
	}
	toolsSlice, ok := toolsAny.([]any)
	if !ok || len(toolsSlice) == 0 {
		t.Fatalf("Expected non-empty tools array in Anthropic payload: %+v", toolsAny)
	}
	firstTool := toolsSlice[0].(map[string]any)
	if firstTool["name"] != "get_current_weather" {
		t.Errorf("Expected tool name 'get_current_weather', got %v", firstTool["name"])
	}
	if firstTool["input_schema"] == nil {
		t.Errorf("Expected tool input_schema to be populated")
	}

	// 2. Verify response translation from Anthropic tool_use to OpenAI ToolCalls
	if len(resp.Choices) == 0 {
		t.Fatalf("No choices in response")
	}
	choice := resp.Choices[0]
	if choice.FinishReason == nil || *choice.FinishReason != "tool_calls" {
		t.Errorf("Expected finish_reason 'tool_calls', got %v", choice.FinishReason)
	}
	if len(choice.Message.ToolCalls) == 0 {
		t.Fatalf("Expected tool_calls in message, got 0")
	}
	tc := choice.Message.ToolCalls[0]
	if tc.ID != "toolu_weather_001" {
		t.Errorf("Expected tool call ID 'toolu_weather_001', got %s", tc.ID)
	}
	if tc.Function.Name != "get_current_weather" {
		t.Errorf("Expected function name 'get_current_weather', got %s", tc.Function.Name)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		t.Fatalf("Invalid JSON in tool call arguments: %s", tc.Function.Arguments)
	}
	if args["location"] != "Tokyo" {
		t.Errorf("Expected location 'Tokyo', got %v", args["location"])
	}
}

func TestOpenAIToGemini_ToolCallingTranslation(t *testing.T) {
	var capturedPayload map[string]any

	geminiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedPayload)

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"candidates": [
				{
					"content": {
						"parts": [
							{
								"functionCall": {
									"name": "lookup_stock_price",
									"args": {
										"symbol": "GOOG"
									}
								}
							}
						],
						"role": "model"
					},
					"finishReason": "STOP",
					"index": 0
				}
			],
			"usageMetadata": {
				"promptTokenCount": 30,
				"candidatesTokenCount": 15,
				"totalTokenCount": 45
			}
		}`)
	}))
	defer geminiServer.Close()

	ch := &model.ChannelConfig{
		Name:     "gemini-upstream",
		Type:     model.ProviderGemini,
		BaseURL:  geminiServer.URL,
		APIKey:   "test-gemini-key",
		Models:   []string{"gemini-1.5-flash"},
		Priority: 1,
		Weight:   10,
	}

	prov := provider.NewGeminiProvider(nil)

	req := &model.ChatCompletionRequest{
		Model: "gemini-1.5-flash",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "What is Google stock price?"},
		},
		Tools: []model.Tool{
			{
				Type: "function",
				Function: map[string]any{
					"name":        "lookup_stock_price",
					"description": "Fetch real-time stock ticker quote",
					"parameters": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"symbol": map[string]any{"type": "string"},
						},
						"required": []any{"symbol"},
					},
				},
			},
		},
	}

	resp, err := prov.ChatComplete(context.Background(), req, ch)
	if err != nil {
		t.Fatalf("Gemini ChatComplete error: %v", err)
	}

	// 1. Verify functionDeclarations sent to Gemini
	toolsAny, exists := capturedPayload["tools"]
	if !exists {
		t.Fatalf("tools not sent to Gemini upstream: %+v", capturedPayload)
	}
	toolsSlice := toolsAny.([]any)
	if len(toolsSlice) == 0 {
		t.Fatalf("empty tools array sent to Gemini")
	}

	// 2. Verify response parsed from Gemini functionCall to OpenAI ToolCalls
	if len(resp.Choices) == 0 {
		t.Fatalf("no choices returned")
	}
	choice := resp.Choices[0]
	if choice.FinishReason == nil || *choice.FinishReason != "tool_calls" {
		t.Errorf("Expected finish_reason 'tool_calls', got %v", choice.FinishReason)
	}
	if len(choice.Message.ToolCalls) == 0 {
		t.Fatalf("Expected tool_calls, got 0")
	}
	tc := choice.Message.ToolCalls[0]
	if tc.Function.Name != "lookup_stock_price" {
		t.Errorf("Expected function 'lookup_stock_price', got %s", tc.Function.Name)
	}
}

func TestAnthropicInbound_ToolCallingPipeline(t *testing.T) {
	// Setup upstream OpenAI mock that returns a ToolCall
	openAIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"id": "chatcmpl-tools-upstream",
			"object": "chat.completion",
			"created": 1700000000,
			"model": "gpt-4o",
			"choices": [
				{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "Checking flight status...",
						"tool_calls": [
							{
								"id": "call_flight_999",
								"type": "function",
								"function": {
									"name": "check_flight",
									"arguments": "{\"flight_number\":\"UA888\"}"
								}
							}
						]
					},
					"finish_reason": "tool_calls"
				}
			],
			"usage": {
				"prompt_tokens": 40,
				"completion_tokens": 20,
				"total_tokens": 60
			}
		}`)
	}))
	defer openAIServer.Close()

	channels := []model.ChannelConfig{
		{
			Name:     "upstream-openai",
			Type:     model.ProviderOpenAI,
			BaseURL:  openAIServer.URL,
			Models:   []string{"claude-inbound-model"},
			Priority: 1,
			Weight:   10,
		},
	}

	dispatcher := router.NewDispatcher(channels)
	handler := api.NewHandler(dispatcher)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/v1/messages", handler.HandleAnthropicMessages)

	anthropicPayload := `{
		"model": "claude-inbound-model",
		"max_tokens": 1024,
		"messages": [
			{"role": "user", "content": "Can you check UA888?"}
		],
		"tools": [
			{
				"name": "check_flight",
				"description": "Check real-time flight status",
				"input_schema": {
					"type": "object",
					"properties": {
						"flight_number": {"type": "string"}
					}
				}
			}
		]
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(anthropicPayload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Anthropic Inbound failed with %d: %s", w.Code, w.Body.String())
	}

	var anthropicResp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &anthropicResp); err != nil {
		t.Fatalf("failed to parse Anthropic response: %v", err)
	}

	// Verify stop_reason is "tool_use"
	if anthropicResp["stop_reason"] != "tool_use" {
		t.Errorf("Expected stop_reason 'tool_use', got %v", anthropicResp["stop_reason"])
	}

	// Verify content blocks contains "tool_use" block
	contentSlice := anthropicResp["content"].([]any)
	var foundToolUse bool
	for _, block := range contentSlice {
		b := block.(map[string]any)
		if b["type"] == "tool_use" {
			foundToolUse = true
			if b["id"] != "call_flight_999" {
				t.Errorf("Expected tool_use id 'call_flight_999', got %v", b["id"])
			}
			if b["name"] != "check_flight" {
				t.Errorf("Expected tool_use name 'check_flight', got %v", b["name"])
			}
			inputMap := b["input"].(map[string]any)
			if inputMap["flight_number"] != "UA888" {
				t.Errorf("Expected flight_number 'UA888', got %v", inputMap["flight_number"])
			}
		}
	}

	if !foundToolUse {
		t.Fatalf("tool_use block not found in Anthropic response: %+v", anthropicResp)
	}
}
