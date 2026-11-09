package model

import (
	"encoding/json"
	"fmt"
	"time"
)

// ConvertChatResponseToResponseResult converts a ChatCompletionResponse into a ResponseResult.
func ConvertChatResponseToResponseResult(chatResp *ChatCompletionResponse) *ResponseResult {
	if chatResp == nil {
		return nil
	}

	resID := chatResp.ID
	if len(resID) > 9 && resID[:9] == "chatcmpl-" {
		resID = "resp_" + resID[9:]
	} else if resID == "" {
		resID = fmt.Sprintf("resp_%d", time.Now().UnixMicro())
	}

	outputItems := chatResponseOutputItems(chatResp, resID)

	created := chatResp.Created
	if created == 0 {
		created = time.Now().Unix()
	}

	usage := chatResponseUsage(chatResp)
	status, incompleteDetails := chatResponseStatus(chatResp)

	return &ResponseResult{
		ID:                resID,
		Object:            "response",
		CreatedAt:         created,
		Status:            status,
		IncompleteDetails: incompleteDetails,
		Model:             chatResp.Model,
		Output:            outputItems,
		Usage:             usage,
	}
}

func chatResponseOutputItems(chatResp *ChatCompletionResponse, resID string) []ResponseOutputItem {
	var outputItems []ResponseOutputItem
	for i, choice := range chatResp.Choices {
		msgID := fmt.Sprintf("msg_%s_%d", resID, i)
		text := ""
		if str, ok := choice.Message.Content.(string); ok {
			text = str
		} else if choice.Message.Content != nil {
			b, _ := json.Marshal(choice.Message.Content)
			text = string(b)
		}

		// A pure tool-call turn has no assistant text; emitting an empty message
		// item would make agents append a blank assistant message. Keep the
		// message item only when there is text or when the turn produced no
		// function calls at all (so a genuinely empty answer stays visible).
		if text != "" || len(choice.Message.ToolCalls) == 0 {
			role := choice.Message.Role
			if role == "" {
				role = "assistant"
			}
			outputItems = append(outputItems, ResponseOutputItem{
				ID:     msgID,
				Type:   "message",
				Status: "completed",
				Role:   role,
				Content: []ResponseContentPart{
					{
						Type: "output_text",
						Text: text,
					},
				},
			})
		}
		outputItems = append(outputItems, chatToolCallsToResponseItems(
			fmt.Sprintf("%s_%d", resID, i), choice.Message.ToolCalls)...)
	}

	return outputItems
}

func chatResponseUsage(chatResp *ChatCompletionResponse) ResponseUsage {
	usage := ResponseUsage{}
	if chatResp.Usage != nil {
		usage.InputTokens = chatResp.Usage.PromptTokens
		usage.OutputTokens = chatResp.Usage.CompletionTokens
		usage.TotalTokens = chatResp.Usage.TotalTokens
		usage.InputTokensDetails = &ResponseInputTokensDetails{CachedTokens: chatResp.Usage.GetCachedTokens()}
		// Report the reasoning share the upstream actually billed instead of a
		// hardcoded 0: the count is preserved in Usage.Extra and agents key
		// continuation/cost decisions off it (F-02).
		usage.OutputTokensDetails = &ResponseOutputTokensDetails{ReasoningTokens: chatResp.Usage.GetReasoningTokens()}
	}

	return usage
}

func chatResponseStatus(chatResp *ChatCompletionResponse) (string, *ResponseIncompleteDetails) {
	// A truncated or filtered answer must not be reported as a normally completed
	// response: clients key continuation/retry decisions off status +
	// incomplete_details.
	status := ResponseStatusCompleted
	var incompleteDetails *ResponseIncompleteDetails
	for _, choice := range chatResp.Choices {
		if choice.FinishReason == nil {
			continue
		}
		if mappedStatus, details := responseStatusFromFinishReason(*choice.FinishReason); mappedStatus != ResponseStatusCompleted {
			status = mappedStatus
			incompleteDetails = details
			break
		}
	}
	return status, incompleteDetails
}
