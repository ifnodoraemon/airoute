package provider

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func geminiFinishReason(reason string, hasTools bool) string {
	switch reason {
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII", "IMAGE_SAFETY", "IMAGE_PROHIBITED_CONTENT":
		return "content_filter"
	case "STOP", "":
		if hasTools {
			return "tool_calls"
		}
	}
	return "stop"
}

func geminiCandidateMessage(candidate GeminiCandidate) model.ChatMessage {
	message := model.ChatMessage{Role: "assistant", Content: ""}
	var answer, thought strings.Builder
	for _, part := range candidate.Content.Parts {
		if part.Thought {
			thought.WriteString(part.Text)
		} else {
			answer.WriteString(part.Text)
		}
		if part.FunctionCall == nil {
			continue
		}
		arguments := part.FunctionCall.Args
		if arguments == nil {
			arguments = map[string]any{}
		}
		args, _ := json.Marshal(arguments)
		id := part.FunctionCall.ID
		if id == "" {
			id = fmt.Sprintf("call_%d_%d", time.Now().UnixNano(), len(message.ToolCalls))
		}
		call := model.ToolCall{ID: id, Type: "function", Function: model.FunctionCall{
			Name: part.FunctionCall.Name, Arguments: string(args),
		}}
		call.SetThoughtSignature(part.ThoughtSignature)
		message.ToolCalls = append(message.ToolCalls, call)
	}
	message.Content, message.ReasoningContent = answer.String(), thought.String()
	var media []map[string]string
	for _, part := range candidate.Content.Parts {
		if part.InlineData != nil {
			media = append(media, map[string]string{"mime_type": part.InlineData.MimeType, "data": part.InlineData.Data})
		}
		if part.FileData != nil {
			media = append(media, map[string]string{"mime_type": part.FileData.MimeType, "uri": part.FileData.FileURI})
		}
	}
	if len(media) > 0 {
		message.Extra = make(model.ExtraFields)
		message.Extra["media"], _ = json.Marshal(media)
	}
	return message
}

func geminiResponseChoices(response *GeminiResponse) []model.ChatCompletionChoice {
	choices := make([]model.ChatCompletionChoice, 0, len(response.Candidates))
	for i, candidate := range response.Candidates {
		message := geminiCandidateMessage(candidate)
		reason := geminiFinishReason(candidate.FinishReason, len(message.ToolCalls) > 0)
		choices = append(choices, model.ChatCompletionChoice{
			Index: max(i, candidate.Index), Message: message, FinishReason: &reason,
		})
	}
	if len(choices) == 0 && response.PromptFeedback != nil && response.PromptFeedback.BlockReason != "" {
		reason := "content_filter"
		choices = append(choices, model.ChatCompletionChoice{
			Index: 0, Message: model.ChatMessage{Role: "assistant", Content: ""}, FinishReason: &reason,
		})
	}
	return choices
}

type geminiStreamChoices struct {
	nextTool map[int]int
	hasTools map[int]bool
}

func (s *geminiStreamChoices) convert(response *GeminiResponse) []model.ChunkChoice {
	if s.nextTool == nil {
		s.nextTool, s.hasTools = make(map[int]int), make(map[int]bool)
	}
	choices := make([]model.ChunkChoice, 0, len(response.Candidates))
	for i, candidate := range response.Candidates {
		index := max(i, candidate.Index)
		message := geminiCandidateMessage(candidate)
		for j := range message.ToolCalls {
			toolIndex := s.nextTool[index]
			message.ToolCalls[j].Index = &toolIndex
			s.nextTool[index]++
			s.hasTools[index] = true
		}
		choice := model.ChunkChoice{Index: index, Delta: model.ChunkDelta{
			Content: message.GetContentString(), ReasoningContent: message.ReasoningContent, ToolCalls: message.ToolCalls,
			Extra: message.Extra,
		}}
		if candidate.FinishReason != "" {
			reason := geminiFinishReason(candidate.FinishReason, s.hasTools[index])
			choice.FinishReason = &reason
		}
		choices = append(choices, choice)
	}
	if len(choices) == 0 && response.PromptFeedback != nil && response.PromptFeedback.BlockReason != "" {
		reason := "content_filter"
		choices = append(choices, model.ChunkChoice{Index: 0, Delta: model.ChunkDelta{}, FinishReason: &reason})
	}
	return choices
}
