package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// Regression test for BUG-RESPONSES-INCOMPLETE on the streaming entrypoint.
//
// The streaming emitter always sent response.completed with status "completed",
// so a max_output_tokens cut looked identical to a finished answer. The Responses
// protocol reports truncation as status "incomplete" + incomplete_details.reason;
// an agent keys its continue/retry decision off exactly that.

const responsesIncompleteModel = "responses-incomplete-model"

// runResponsesStreamProbe drives handleResponsesStreaming against an
// OpenAI-compatible upstream and returns the SSE frames the client received.
func runResponsesStreamProbe(t *testing.T, sseBody string) []sseEvent {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sseBody)
	}))
	defer upstream.Close()

	_ = edgeBillingRepo(t, &storage.ModelPriceRecord{
		Model: responsesIncompleteModel, GroupName: "default",
		PromptPrice: 1, CompletionPrice: 2, Currency: "CNY",
	})
	dispatcher := router.NewDispatcher([]model.ChannelConfig{{
		Name: "openai-for-responses", Type: model.ProviderOpenAI, BaseURL: upstream.URL,
		Models: []string{responsesIncompleteModel}, Status: "active", Weight: 1,
	}})
	handler := NewHandler(dispatcher)

	chatReq := &model.ChatCompletionRequest{
		Model:    responsesIncompleteModel,
		Stream:   true,
		Messages: []model.ChatMessage{{Role: "user", Content: "explain everything"}},
	}
	req := model.ResponseRequest{Model: responsesIncompleteModel, Stream: true}
	c, w := newTestContext(http.MethodPost, "/v1/responses")
	handler.handleResponsesStreaming(c, c.Request.Context(), req, chatReq, "sess-responses-incomplete", time.Now())

	return parseSSEEvents(t, w.Body.String())
}

// sseEvent is one parsed Server-Sent Event.
type sseEvent struct {
	Event string
	Data  map[string]any
}

// parseSSEEvents parses named SSE events (event: X + data: {...}) out of a body.
func parseSSEEvents(t *testing.T, body string) []sseEvent {
	t.Helper()
	var events []sseEvent
	current := ""
	for _, rawLine := range strings.Split(body, "\n") {
		line := strings.TrimSpace(rawLine)
		switch {
		case strings.HasPrefix(line, "event:"):
			current = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "[DONE]" {
				continue
			}
			var data map[string]any
			if err := json.Unmarshal([]byte(payload), &data); err != nil {
				t.Fatalf("data frame is not JSON: %v (%q)", err, payload)
			}
			events = append(events, sseEvent{Event: current, Data: data})
		}
	}
	return events
}

// terminalResponse returns the "response" object of the last response.completed
// event, or nil when the stream never emitted one.
func terminalResponse(events []sseEvent) map[string]any {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Event != "response.completed" {
			continue
		}
		response, _ := events[i].Data["response"].(map[string]any)
		return response
	}
	return nil
}

// TestResponsesStreamReportsTruncationAsIncomplete is the streaming counterpart
// of the unary regression test: finish_reason=length must produce a terminal
// response with status incomplete and the documented reason.
func TestResponsesStreamReportsTruncationAsIncomplete(t *testing.T) {
	body := runResponsesStreamProbe(t, `data: {"id":"c1","choices":[{"index":0,"delta":{"content":"The answer is 42 and"},"finish_reason":null}]}

data: {"id":"c1","choices":[{"index":0,"delta":{},"finish_reason":"length"}]}

data: {"id":"c1","choices":[],"usage":{"prompt_tokens":12,"completion_tokens":5,"total_tokens":17}}

data: [DONE]

`)

	response := terminalResponse(body)
	if response == nil {
		t.Fatal("stream emitted no response.completed event")
	}
	if response["status"] != model.ResponseStatusIncomplete {
		t.Fatalf("terminal response status = %v, want %q for finish_reason=length", response["status"], model.ResponseStatusIncomplete)
	}
	details, _ := response["incomplete_details"].(map[string]any)
	if details == nil || details["reason"] != "max_output_tokens" {
		t.Fatalf("incomplete_details = %v, want reason=max_output_tokens", response["incomplete_details"])
	}
}

// TestResponsesStreamNormalCompletionStaysCompleted pins the no-regression case.
func TestResponsesStreamNormalCompletionStaysCompleted(t *testing.T) {
	body := runResponsesStreamProbe(t, `data: {"id":"c1","choices":[{"index":0,"delta":{"content":"done"},"finish_reason":null}]}

data: {"id":"c1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: {"id":"c1","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}

data: [DONE]

`)

	response := terminalResponse(body)
	if response == nil {
		t.Fatal("stream emitted no response.completed event")
	}
	if response["status"] != model.ResponseStatusCompleted {
		t.Fatalf("terminal response status = %v, want completed", response["status"])
	}
	if _, ok := response["incomplete_details"]; ok {
		t.Fatalf("completed response must not carry incomplete_details: %v", response)
	}
}

// TestResponsesStreamAndUnaryUsageShapeAgree pins that both entrypoints report
// the same usage members (the unary one used to drop the cache breakdown).
func TestResponsesStreamAndUnaryUsageShapeAgree(t *testing.T) {
	events := runResponsesStreamProbe(t, `data: {"id":"c1","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":"stop"}]}

data: {"id":"c1","choices":[],"usage":{"prompt_tokens":9,"completion_tokens":2,"total_tokens":11,"prompt_tokens_details":{"cached_tokens":7}}}

data: [DONE]

`)
	response := terminalResponse(events)
	if response == nil {
		t.Fatal("stream emitted no response.completed event")
	}
	usage, _ := response["usage"].(map[string]any)
	if usage == nil {
		t.Fatal("terminal response has no usage")
	}
	streamingDetails, _ := usage["input_tokens_details"].(map[string]any)
	if streamingDetails == nil || streamingDetails["cached_tokens"] != float64(7) {
		t.Fatalf("streaming usage input_tokens_details = %v, want cached_tokens=7", usage["input_tokens_details"])
	}

	// The unary conversion must expose the same members for the same consumption.
	result := model.ConvertChatResponseToResponseResult(&model.ChatCompletionResponse{
		ID:      "chatcmpl-1",
		Created: 1,
		Model:   responsesIncompleteModel,
		Choices: []model.ChatCompletionChoice{{Message: model.ChatMessage{Role: "assistant", Content: "hi"}}},
		Usage: &model.Usage{
			PromptTokens: 9, CompletionTokens: 2, TotalTokens: 11,
			PromptTokensDetails: &model.PromptTokensDetails{CachedTokens: 7},
		},
	})
	if result.Usage.InputTokensDetails == nil || result.Usage.InputTokensDetails.CachedTokens != 7 {
		t.Fatalf("unary usage input_tokens_details = %+v, want cached_tokens=7 (shape must match the stream)",
			result.Usage.InputTokensDetails)
	}
}
