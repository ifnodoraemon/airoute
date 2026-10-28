package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ResponseRequest represents the OpenAI Responses API (/v1/responses) request payload.
type ResponseRequest struct {
	Model              string            `json:"model"`
	Input              any               `json:"input"`                  // string or []ChatMessage or []map[string]any
	Instructions       string            `json:"instructions,omitempty"` // system instructions
	PreviousResponseID string            `json:"previous_response_id,omitempty"`
	User               string            `json:"user,omitempty"`
	Stream             bool              `json:"stream,omitempty"`
	Temperature        *float64          `json:"temperature,omitempty"`
	TopP               *float64          `json:"top_p,omitempty"`
	MaxOutputTokens    int               `json:"max_output_tokens,omitempty"`
	Tools              []any             `json:"tools,omitempty"`
	ToolChoice         any               `json:"tool_choice,omitempty"`
	ParallelToolCalls  *bool             `json:"parallel_tool_calls,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
}

// ResponseContentPart represents a part of the content in a message item.
type ResponseContentPart struct {
	Type string `json:"type"` // "output_text" or "text"
	Text string `json:"text"`
}

// ResponseOutputItem represents an item in the response output array.
type ResponseOutputItem struct {
	ID      string                `json:"id"`
	Type    string                `json:"type"` // "message" or "function_call"
	Status  string                `json:"status"`
	Role    string                `json:"role,omitempty"`
	Content []ResponseContentPart `json:"content,omitempty"`

	// function_call fields (empty for message items).
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// ResponseInputTokensDetails is the documented input-token breakdown.
type ResponseInputTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

// ResponseOutputTokensDetails is the documented output-token breakdown.
type ResponseOutputTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

// ResponseUsage represents token consumption in a ResponseResult.
//
// The detail blocks keep the unary payload shaped like the streaming one (which
// always emitted input_tokens_details): without them the same request reported a
// cached-token breakdown when streamed and dropped it when not.
type ResponseUsage struct {
	InputTokens         int                          `json:"input_tokens"`
	InputTokensDetails  *ResponseInputTokensDetails  `json:"input_tokens_details,omitempty"`
	OutputTokens        int                          `json:"output_tokens"`
	OutputTokensDetails *ResponseOutputTokensDetails `json:"output_tokens_details,omitempty"`
	TotalTokens         int                          `json:"total_tokens"`
}

// ResponseIncompleteDetails explains why a response is not complete.
type ResponseIncompleteDetails struct {
	Reason string `json:"reason"` // "max_output_tokens" or "content_filter"
}

// ResponseResult represents the standard OpenAI Responses API response payload.
type ResponseResult struct {
	ID                string                     `json:"id"`
	Object            string                     `json:"object"` // "response"
	CreatedAt         int64                      `json:"created_at"`
	Status            string                     `json:"status"` // "completed", "incomplete", ...
	IncompleteDetails *ResponseIncompleteDetails `json:"incomplete_details,omitempty"`
	Model             string                     `json:"model"`
	Output            []ResponseOutputItem       `json:"output"`
	Usage             ResponseUsage              `json:"usage"`
}

// Response status values.
const (
	ResponseStatusCompleted  = "completed"
	ResponseStatusIncomplete = "incomplete"
)

// responseStatusFromFinishReason maps an upstream chat finish_reason onto the
// Responses API status. The Responses protocol distinguishes a truncated answer
// (status "incomplete" + incomplete_details.reason) from a normal end, which is
// what lets an agent decide whether to continue or retry; a chat finish_reason is
// the only signal the gateway receives.
func responseStatusFromFinishReason(finishReason string) (string, *ResponseIncompleteDetails) {
	switch finishReason {
	case "length":
		return ResponseStatusIncomplete, &ResponseIncompleteDetails{Reason: "max_output_tokens"}
	case "content_filter":
		return ResponseStatusIncomplete, &ResponseIncompleteDetails{Reason: "content_filter"}
	default:
		return ResponseStatusCompleted, nil
	}
}

// IncompleteResponseStatus is the exported helper the streaming emitter uses so
// the unary and streaming paths agree on the mapping.
func IncompleteResponseStatus(finishReason string) (string, *ResponseIncompleteDetails) {
	return responseStatusFromFinishReason(finishReason)
}

// ToChatCompletionRequest converts a ResponseRequest into a canonical ChatCompletionRequest.
func (r *ResponseRequest) ToChatCompletionRequest() (*ChatCompletionRequest, error) {
	var maxTokens *int
	if r.MaxOutputTokens > 0 {
		maxTokens = &r.MaxOutputTokens
	}

	chatTools := responseToolsToChatTools(r.Tools)
	chatToolChoice := responseToolChoiceToChatToolChoice(r.ToolChoice)
	if len(chatTools) == 0 {
		// Built-in Responses tools (web_search, ...) have no chat equivalent. If
		// none of the requested tools survived the conversion, forwarding
		// tool_choice:"required" to an upstream that received no tools is a
		// guaranteed 400; drop the selector instead.
		chatToolChoice = nil
	}
	chatReq := &ChatCompletionRequest{
		Model:             r.Model,
		Stream:            r.Stream,
		Temperature:       r.Temperature,
		TopP:              r.TopP,
		MaxTokens:         maxTokens,
		Tools:             chatTools,
		ToolChoice:        chatToolChoice,
		ParallelToolCalls: r.ParallelToolCalls,
	}

	var messages []ChatMessage

	if r.Instructions != "" {
		messages = append(messages, ChatMessage{
			Role:    "system",
			Content: r.Instructions,
		})
	}

	inputMessages, err := ResponseInputToChatMessages(r.Input)
	if err != nil {
		return nil, err
	}
	messages = append(messages, inputMessages...)

	if len(messages) == 0 {
		return nil, fmt.Errorf("request must specify non-empty 'input'")
	}

	chatReq.Messages = messages
	return chatReq, nil
}

// ResponseInputToChatMessages converts the Responses API `input` field into
// canonical chat-completion messages.
//
// The Responses wire format is an item list, not a chat message list: an input
// turn can be a message carrying content parts (input_text, input_image), the
// function_call the assistant previously emitted, or the function_call_output
// that answers it. Chat completions represents the same history as user/tool
// messages and assistant tool_calls, so every shape has to be translated. When
// the shapes are not understood the item is skipped: forwarding an unknown item
// as an empty user message actively corrupts the conversation (the upstream sees
// a blank turn) instead of merely losing the item.
func ResponseInputToChatMessages(input any) ([]ChatMessage, error) {
	switch typed := input.(type) {
	case nil:
		return nil, nil
	case string:
		return []ChatMessage{{Role: "user", Content: typed}}, nil
	case ChatMessage:
		return responseChatMessageItems([]ChatMessage{typed})
	case []ChatMessage:
		return responseChatMessageItems(typed)
	case []any:
		return responseItemsToChatMessages(typed)
	case []map[string]any:
		items := make([]any, len(typed))
		for i := range typed {
			items[i] = typed[i]
		}
		return responseItemsToChatMessages(items)
	default:
		return nil, fmt.Errorf("field 'input' must be a string or an array of input items, got %T", input)
	}
}

// responseChatMessageItems normalizes already-structured chat messages. They
// still pass through the content-part conversion because a Responses client can
// send chat-shaped messages whose content uses Responses parts.
func responseChatMessageItems(messages []ChatMessage) ([]ChatMessage, error) {
	out := make([]ChatMessage, 0, len(messages))
	for _, message := range messages {
		converted, err := responseMessageToChatMessage(map[string]any{
			"role":         message.Role,
			"content":      message.Content,
			"name":         message.Name,
			"tool_calls":   message.ToolCalls,
			"tool_call_id": message.ToolCallID,
		})
		if err != nil {
			return nil, err
		}
		if converted == nil {
			continue
		}
		out = appendResponseChatMessage(out, *converted)
	}
	return out, nil
}

// responseItemsToChatMessages converts a Responses input item array.
func responseItemsToChatMessages(items []any) ([]ChatMessage, error) {
	out := make([]ChatMessage, 0, len(items))
	for index, raw := range items {
		item, err := responseItemObject(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid 'input' item %d: %w", index, err)
		}
		converted, err := responseItemToChatMessages(item)
		if err != nil {
			return nil, fmt.Errorf("invalid 'input' item %d: %w", index, err)
		}
		for _, message := range converted {
			out = appendResponseChatMessage(out, message)
		}
	}
	return out, nil
}

// responseItemObject normalizes one input item to a JSON object. Items decoded
// from JSON are already map[string]any; typed callers can pass structs.
func responseItemObject(raw any) (map[string]any, error) {
	switch typed := raw.(type) {
	case nil:
		return map[string]any{}, nil
	case map[string]any:
		return typed, nil
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return nil, err
		}
		var object map[string]any
		if err := json.Unmarshal(encoded, &object); err != nil {
			return nil, fmt.Errorf("expected an object, got %T", raw)
		}
		return object, nil
	}
}

// responseItemToChatMessages converts one Responses input item. It returns nil
// for items with no chat representation (reasoning, item_reference, built-in
// tool calls, ...); those must never surface as an empty user message.
func responseItemToChatMessages(item map[string]any) ([]ChatMessage, error) {
	if len(item) == 0 {
		return nil, nil
	}
	itemType, _ := item["type"].(string)
	switch itemType {
	case "function_call":
		return responseFunctionCallItems(item)
	case "function_call_output":
		message, ok := responseFunctionCallOutputMessage(item)
		if !ok {
			return nil, nil
		}
		return []ChatMessage{message}, nil
	case "input_text", "text", "output_text", "input_image", "image_url":
		// Content parts are normally nested inside a message item, but some
		// clients send a bare part as an item. Treat it as a user message part.
		content, ok, err := responseContentToChatContent([]any{item})
		if err != nil || !ok {
			return nil, err
		}
		return []ChatMessage{{Role: "user", Content: content}}, nil
	}

	role, _ := item["role"].(string)
	if role == "" {
		if itemType != "" && itemType != "message" {
			// Unknown Responses item type: skip instead of fabricating a user turn.
			return nil, nil
		}
		_, hasContent := item["content"]
		_, hasText := item["text"].(string)
		if !hasContent && !hasText {
			// An item with neither a role nor content carries nothing a chat
			// request can express.
			return nil, nil
		}
	}
	message, err := responseMessageToChatMessage(item)
	if err != nil || message == nil {
		return nil, err
	}
	return []ChatMessage{*message}, nil
}

// responseMessageToChatMessage converts a message-shaped item (or a plain chat
// message) into a ChatMessage.
func responseMessageToChatMessage(item map[string]any) (*ChatMessage, error) {
	role, _ := item["role"].(string)
	role = normalizeResponseRole(role)

	content, hasContent := item["content"]
	if !hasContent {
		content, hasContent = item["text"]
	}

	var chatContent any
	if hasContent {
		converted, ok, err := responseContentToChatContent(content)
		if err != nil {
			return nil, err
		}
		if ok {
			chatContent = converted
		}
	}

	message := &ChatMessage{Role: role, Content: chatContent}
	if toolCallID, ok := item["tool_call_id"].(string); ok && toolCallID != "" {
		message.ToolCallID = toolCallID
	}
	if name, ok := item["name"].(string); ok && name != "" {
		message.Name = name
	}
	if toolCalls, ok := responseToolCalls(item["tool_calls"]); ok {
		message.ToolCalls = toolCalls
	}
	if message.Content == nil && len(message.ToolCalls) == 0 {
		// Nothing to say: an empty turn would only confuse the upstream.
		return nil, nil
	}
	return message, nil
}

// responseContentToChatContent converts a Responses message `content` (a string
// or an array of content parts) into chat content.
//
// An all-text part list collapses to a plain string because that is what chat
// models expect for the common case; mixing in an image switches the whole
// content to the chat part array so the image survives.
func responseContentToChatContent(content any) (any, bool, error) {
	switch typed := content.(type) {
	case nil:
		return nil, false, nil
	case string:
		if typed == "" {
			return nil, false, nil
		}
		return typed, true, nil
	case []any:
		return responseContentPartsToChatContent(typed)
	case []map[string]any:
		parts := make([]any, len(typed))
		for i := range typed {
			parts[i] = typed[i]
		}
		return responseContentPartsToChatContent(parts)
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return nil, false, err
		}
		var parts []any
		if err := json.Unmarshal(encoded, &parts); err != nil {
			return nil, false, fmt.Errorf("message 'content' must be a string or an array of content parts, got %T", content)
		}
		return responseContentPartsToChatContent(parts)
	}
}

// responseContentPart is one converted content part in its original order.
type responseContentPart struct {
	text  string
	image map[string]any
}

// responseContentPartsToChatContent converts Responses content parts.
func responseContentPartsToChatContent(parts []any) (any, bool, error) {
	converted := make([]responseContentPart, 0, len(parts))
	for _, raw := range parts {
		part, ok := raw.(map[string]any)
		if !ok {
			// A bare string is tolerated as a text part.
			if text, isText := raw.(string); isText && text != "" {
				converted = append(converted, responseContentPart{text: text})
			}
			continue
		}
		partType, _ := part["type"].(string)
		switch partType {
		case "", "text", "input_text", "output_text":
			if text, ok := part["text"].(string); ok && text != "" {
				converted = append(converted, responseContentPart{text: text})
			}
		case "image_url", "input_image":
			if image := responseImagePartToChatPart(part); image != nil {
				converted = append(converted, responseContentPart{image: image})
			}
		default:
			// input_file / input_audio / refusal parts have no chat text or image
			// representation here; dropping them is better than corrupting the turn.
		}
	}
	if len(converted) == 0 {
		return nil, false, nil
	}

	allText := true
	for _, part := range converted {
		if part.image != nil {
			allText = false
			break
		}
	}
	if allText {
		texts := make([]string, 0, len(converted))
		for _, part := range converted {
			texts = append(texts, part.text)
		}
		return strings.Join(texts, "\n"), true, nil
	}

	chatParts := make([]any, 0, len(converted))
	for _, part := range converted {
		if part.image != nil {
			chatParts = append(chatParts, part.image)
			continue
		}
		chatParts = append(chatParts, map[string]any{"type": "text", "text": part.text})
	}
	return chatParts, true, nil
}

// responseImagePartToChatPart maps a Responses input_image part to the chat
// image_url part shape. `detail` is carried on the part itself (Responses) or
// inside the nested image_url object (chat clients), so both are accepted.
func responseImagePartToChatPart(part map[string]any) map[string]any {
	var url, detail string
	switch raw := part["image_url"].(type) {
	case string:
		url = strings.TrimSpace(raw)
	case map[string]any:
		url, _ = raw["url"].(string)
		detail, _ = raw["detail"].(string)
	}
	if detail == "" {
		detail, _ = part["detail"].(string)
	}
	if url == "" {
		// A file_id-only image cannot be resolved by the gateway.
		return nil
	}
	imageURL := map[string]any{"url": url}
	if detail != "" {
		imageURL["detail"] = detail
	}
	return map[string]any{"type": "image_url", "image_url": imageURL}
}

// responseFunctionCallItems converts a Responses function_call item into an
// assistant tool_calls turn.
func responseFunctionCallItems(item map[string]any) ([]ChatMessage, error) {
	name, _ := item["name"].(string)
	callID, _ := item["call_id"].(string)
	if callID == "" {
		// SDKs sometimes round-trip the Responses item id instead of call_id.
		callID, _ = item["id"].(string)
	}
	arguments, ok := responseArgumentsString(item["arguments"])
	if name == "" || callID == "" || !ok {
		// Without a name/call_id the call cannot be referenced by its output.
		return nil, nil
	}
	return []ChatMessage{{
		Role: "assistant",
		ToolCalls: []ToolCall{{
			ID:       callID,
			Type:     "function",
			Function: FunctionCall{Name: name, Arguments: arguments},
		}},
	}}, nil
}

// responseFunctionCallOutputMessage converts a function_call_output item into a
// chat tool message. The output can be a string or any JSON value (object/array
// for structured tool results), which becomes the tool message content.
func responseFunctionCallOutputMessage(item map[string]any) (ChatMessage, bool) {
	callID, _ := item["call_id"].(string)
	if callID == "" {
		return ChatMessage{}, false
	}
	content := ""
	switch output := item["output"].(type) {
	case nil:
		// Empty output is still a completed tool call.
	case string:
		content = output
	default:
		encoded, err := json.Marshal(output)
		if err != nil {
			return ChatMessage{}, false
		}
		content = string(encoded)
	}
	return ChatMessage{Role: "tool", ToolCallID: callID, Content: content}, true
}

// responseArgumentsString renders a function_call `arguments` value as the JSON
// string chat completions require.
func responseArgumentsString(raw any) (string, bool) {
	switch typed := raw.(type) {
	case nil:
		return "{}", true
	case string:
		return typed, true
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return "", false
		}
		return string(encoded), true
	}
}

// responseToolCalls decodes chat-style tool_calls echoed inside an input item.
func responseToolCalls(raw any) ([]ToolCall, bool) {
	if raw == nil {
		return nil, false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	var calls []ToolCall
	if err := json.Unmarshal(encoded, &calls); err != nil || len(calls) == 0 {
		return nil, false
	}
	return calls, true
}

// normalizeResponseRole maps Responses roles onto chat roles. "developer" is the
// newer name for the system turn and is not accepted by every chat upstream.
func normalizeResponseRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "", "user":
		return "user"
	case "developer", "system":
		return "system"
	default:
		return strings.ToLower(strings.TrimSpace(role))
	}
}

// appendResponseChatMessage appends one converted message, coalescing
// consecutive function_call items into a single assistant tool_calls turn. Chat
// completions expects every call of a turn in one assistant message before the
// tool results that answer them.
func appendResponseChatMessage(out []ChatMessage, message ChatMessage) []ChatMessage {
	if len(out) == 0 || message.Role != "assistant" || len(message.ToolCalls) == 0 || message.Content != nil {
		return append(out, message)
	}
	previous := &out[len(out)-1]
	if previous.Role == "assistant" && previous.Content == nil && len(previous.ToolCalls) > 0 {
		previous.ToolCalls = append(previous.ToolCalls, message.ToolCalls...)
		return out
	}
	return append(out, message)
}

// MergeResponseConversation prepends a stored previous-response conversation
// after the new request's leading system instructions and before its new input.
//
// System messages are dropped from the stored side: the caller's new
// `instructions` are authoritative for this turn, and replaying an old system
// prompt after them would silently override it.
func MergeResponseConversation(newMessages, previous []ChatMessage) []ChatMessage {
	if len(previous) == 0 {
		return newMessages
	}
	merged := make([]ChatMessage, 0, len(newMessages)+len(previous))
	leading := 0
	for leading < len(newMessages) && newMessages[leading].Role == "system" {
		merged = append(merged, newMessages[leading])
		leading++
	}
	for _, message := range previous {
		if message.Role == "system" {
			continue
		}
		merged = append(merged, message)
	}
	merged = append(merged, newMessages[leading:]...)
	return merged
}

// ResponseConversationMessages builds the message list persisted for a
// completed response: the non-system messages that produced it plus the
// assistant turn itself. Storing system instructions is deliberately avoided so
// a later request's `instructions` cannot be silently overridden by replaying
// the old prompt.
func ResponseConversationMessages(conversation []ChatMessage, assistant *ChatMessage) []ChatMessage {
	out := make([]ChatMessage, 0, len(conversation)+1)
	for _, message := range conversation {
		if message.Role == "system" {
			continue
		}
		out = append(out, message)
	}
	if assistant != nil {
		out = append(out, *assistant)
	}
	return out
}

// AssistantMessageFromChatResponse extracts the assistant turn of the first
// choice so it can be stored as conversation context.
func AssistantMessageFromChatResponse(resp *ChatCompletionResponse) *ChatMessage {
	if resp == nil || len(resp.Choices) == 0 {
		return nil
	}
	source := resp.Choices[0].Message
	message := &ChatMessage{Role: "assistant"}
	if text, ok := source.Content.(string); ok {
		if strings.TrimSpace(text) != "" {
			message.Content = text
		}
	} else if source.Content != nil {
		message.Content = source.Content
	}
	if len(source.ToolCalls) > 0 {
		message.ToolCalls = source.ToolCalls
	}
	if message.Content == nil && len(message.ToolCalls) == 0 {
		return nil
	}
	return message
}

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

	created := chatResp.Created
	if created == 0 {
		created = time.Now().Unix()
	}

	usage := ResponseUsage{}
	if chatResp.Usage != nil {
		usage.InputTokens = chatResp.Usage.PromptTokens
		usage.OutputTokens = chatResp.Usage.CompletionTokens
		usage.TotalTokens = chatResp.Usage.TotalTokens
		usage.InputTokensDetails = &ResponseInputTokensDetails{CachedTokens: chatResp.Usage.GetCachedTokens()}
		usage.OutputTokensDetails = &ResponseOutputTokensDetails{ReasoningTokens: 0}
	}

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
