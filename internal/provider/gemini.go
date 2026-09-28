package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// GeminiProvider handles Google's specialized Gemini Developer API & Vertex AI protocols.
type GeminiProvider struct {
	client *http.Client
}

// NewGeminiProvider creates a Gemini provider instance.
func NewGeminiProvider(client *http.Client) *GeminiProvider {
	if client == nil {
		client = SharedDefaultHTTPClient
	}
	return &GeminiProvider{client: client}
}

func (p *GeminiProvider) Name() string {
	return "google-gemini"
}

func (p *GeminiProvider) Type() model.ProviderType {
	return model.ProviderGemini
}

type GeminiFunctionDeclaration struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

type GeminiToolDeclarationContainer struct {
	FunctionDeclarations []GeminiFunctionDeclaration `json:"functionDeclarations,omitempty"`
}

type GeminiFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type GeminiFunctionResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

// GeminiPart represents a single part of content (text, multimodal inlineData, or functionCall/functionResponse).
type GeminiPart struct {
	Text             string                  `json:"text,omitempty"`
	InlineData       *GeminiInlineData       `json:"inlineData,omitempty"`
	FunctionCall     *GeminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *GeminiFunctionResponse `json:"functionResponse,omitempty"`
}

type GeminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"` // base64 encoded
}

// GeminiContent represents a role-grouped turn.
type GeminiContent struct {
	Role  string       `json:"role"` // "user" or "model"
	Parts []GeminiPart `json:"parts"`
}

// GeminiSystemInstruction represents the system prompt container.
type GeminiSystemInstruction struct {
	Parts []GeminiPart `json:"parts"`
}

// GeminiGenerationConfig controls temperature, max tokens, etc.
type GeminiGenerationConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
}

// GeminiSafetySetting disables or tunes strictness of safety filters to avoid false positive blocks.
type GeminiSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

// GeminiRequest represents the payload for Gemini generateContent.
type GeminiRequest struct {
	Contents          []GeminiContent                  `json:"contents"`
	SystemInstruction *GeminiSystemInstruction         `json:"systemInstruction,omitempty"`
	GenerationConfig  *GeminiGenerationConfig          `json:"generationConfig,omitempty"`
	SafetySettings    []GeminiSafetySetting            `json:"safetySettings,omitempty"`
	Tools             []GeminiToolDeclarationContainer `json:"tools,omitempty"`
}

// GeminiUsageMetadata reports token counts.
type GeminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// GeminiCandidate represents a candidate generation.
type GeminiCandidate struct {
	Content struct {
		Parts []GeminiPart `json:"parts"`
		Role  string       `json:"role"`
	} `json:"content"`
	FinishReason string `json:"finishReason"`
	Index        int    `json:"index"`
}

// GeminiResponse is the response from generateContent.
type GeminiResponse struct {
	Candidates    []GeminiCandidate    `json:"candidates"`
	UsageMetadata *GeminiUsageMetadata `json:"usageMetadata,omitempty"`
}

// convertOpenAIToGemini translates OpenAI canonical request to Gemini structure.
func convertOpenAIToGemini(req *model.ChatCompletionRequest) *GeminiRequest {
	var contents []GeminiContent
	var systemParts []GeminiPart

	toolCallNames := make(map[string]string)
	for _, m := range req.Messages {
		for _, tc := range m.ToolCalls {
			if tc.ID != "" && tc.Function.Name != "" {
				toolCallNames[tc.ID] = tc.Function.Name
			}
		}
	}

	for _, msg := range req.Messages {
		if strings.ToLower(msg.Role) == "system" {
			parts := model.ParseMessageContent(msg.Content)
			for _, part := range parts {
				if part.Text != "" {
					systemParts = append(systemParts, GeminiPart{Text: part.Text})
				}
			}
			continue
		}

		if strings.ToLower(msg.Role) == "tool" {
			var respMap map[string]any
			if err := json.Unmarshal([]byte(msg.GetContentString()), &respMap); err != nil {
				respMap = map[string]any{"content": msg.GetContentString()}
			}
			fnName := msg.Name
			if fnName == "" && msg.ToolCallID != "" {
				fnName = toolCallNames[msg.ToolCallID]
			}
			if fnName == "" {
				fnName = "function_response"
			}
			contents = append(contents, GeminiContent{
				Role: "user",
				Parts: []GeminiPart{
					{
						FunctionResponse: &GeminiFunctionResponse{
							Name:     fnName,
							Response: respMap,
						},
					},
				},
			})
			continue
		}

		if strings.ToLower(msg.Role) == "assistant" && len(msg.ToolCalls) > 0 {
			var geminiParts []GeminiPart
			if text := msg.GetContentString(); text != "" {
				geminiParts = append(geminiParts, GeminiPart{Text: text})
			}
			for _, tc := range msg.ToolCalls {
				var args map[string]any
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
				geminiParts = append(geminiParts, GeminiPart{
					FunctionCall: &GeminiFunctionCall{
						Name: tc.Function.Name,
						Args: args,
					},
				})
			}
			contents = append(contents, GeminiContent{
				Role:  "model",
				Parts: geminiParts,
			})
			continue
		}

		parts := model.ParseMessageContent(msg.Content)
		if len(parts) == 0 {
			continue
		}

		// Role mapping: "assistant" -> "model", "user" -> "user"
		role := "user"
		if strings.ToLower(msg.Role) == "assistant" {
			role = "model"
		}

		var geminiParts []GeminiPart
		for _, part := range parts {
			switch part.Type {
			case model.ContentPartText:
				if part.Text != "" {
					geminiParts = append(geminiParts, GeminiPart{Text: part.Text})
				}
			case model.ContentPartImageURL:
				if part.ImageURL != nil && part.ImageURL.URL != "" {
					mime, b64 := model.ParseDataURI(part.ImageURL.URL)
					geminiParts = append(geminiParts, GeminiPart{
						InlineData: &GeminiInlineData{
							MimeType: mime,
							Data:     b64,
						},
					})
				}
			case model.ContentPartInputAudio:
				if part.InputAudio != nil && part.InputAudio.Data != "" {
					mime := "audio/wav"
					if part.InputAudio.Format == "mp3" {
						mime = "audio/mp3"
					}
					geminiParts = append(geminiParts, GeminiPart{
						InlineData: &GeminiInlineData{
							MimeType: mime,
							Data:     part.InputAudio.Data,
						},
					})
				}
			}
		}

		if len(geminiParts) > 0 {
			contents = append(contents, GeminiContent{
				Role:  role,
				Parts: geminiParts,
			})
		}
	}

	var toolContainers []GeminiToolDeclarationContainer
	if len(req.Tools) > 0 {
		var funcDecls []GeminiFunctionDeclaration
		for _, t := range req.Tools {
			fnName, _ := t.Function["name"].(string)
			fnDesc, _ := t.Function["description"].(string)
			fnParams := t.Function["parameters"]
			funcDecls = append(funcDecls, GeminiFunctionDeclaration{
				Name:        fnName,
				Description: fnDesc,
				Parameters:  fnParams,
			})
		}
		if len(funcDecls) > 0 {
			toolContainers = append(toolContainers, GeminiToolDeclarationContainer{
				FunctionDeclarations: funcDecls,
			})
		}
	}

	// Merge consecutive same-role contents to strictly adhere to Gemini's alternating turns contract
	var mergedContents []GeminiContent
	for _, c := range contents {
		if len(mergedContents) > 0 && mergedContents[len(mergedContents)-1].Role == c.Role {
			mergedContents[len(mergedContents)-1].Parts = append(mergedContents[len(mergedContents)-1].Parts, c.Parts...)
		} else {
			mergedContents = append(mergedContents, c)
		}
	}

	geminiReq := &GeminiRequest{
		Contents: mergedContents,
		Tools:    toolContainers,
		GenerationConfig: &GeminiGenerationConfig{
			Temperature:     req.Temperature,
			TopP:            req.TopP,
			MaxOutputTokens: req.MaxTokens,
		},
		// Permissive safety settings by default so enterprise and technical code aren't falsely blocked
		SafetySettings: []GeminiSafetySetting{
			{Category: "HARM_CATEGORY_HARASSMENT", Threshold: "BLOCK_NONE"},
			{Category: "HARM_CATEGORY_HATE_SPEECH", Threshold: "BLOCK_NONE"},
			{Category: "HARM_CATEGORY_SEXUALLY_EXPLICIT", Threshold: "BLOCK_NONE"},
			{Category: "HARM_CATEGORY_DANGEROUS_CONTENT", Threshold: "BLOCK_NONE"},
		},
	}

	if len(systemParts) > 0 {
		geminiReq.SystemInstruction = &GeminiSystemInstruction{Parts: systemParts}
	}

	return geminiReq
}

// buildGeminiURL creates Google endpoint URL.
func buildGeminiURL(baseURL, modelName string, stream bool, apiKey string) string {
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	action := "generateContent"
	if stream {
		action = "streamGenerateContent?alt=sse"
	}

	sep := "?"
	if strings.Contains(action, "?") {
		sep = "&"
	}

	return fmt.Sprintf("%s/v1beta/models/%s:%s%skey=%s", baseURL, modelName, action, sep, apiKey)
}

// ChatComplete executes a non-streaming Gemini call and converts to OpenAI format.
func (p *GeminiProvider) ChatComplete(ctx context.Context, req *model.ChatCompletionRequest, channel *model.ChannelConfig) (*model.ChatCompletionResponse, error) {
	geminiReq := convertOpenAIToGemini(req)
	payloadBytes, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal gemini request error: %w", err)
	}

	targetModel := channel.GetUpstreamModel(req.Model)
	targetURL := buildGeminiURL(channel.BaseURL, targetModel, false, channel.APIKey)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("create gemini http request error: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", channel.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do gemini request error: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read gemini response body error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream gemini %s returned %d: %s", channel.Name, resp.StatusCode, string(bodyBytes))
	}

	var geminiResp GeminiResponse
	if err := json.Unmarshal(bodyBytes, &geminiResp); err != nil {
		return nil, fmt.Errorf("unmarshal gemini response error: %w", err)
	}

	replyText := ""
	finishReason := "stop"
	var toolCalls []model.ToolCall
	if len(geminiResp.Candidates) > 0 {
		cand := geminiResp.Candidates[0]
		var sb strings.Builder
		for _, part := range cand.Content.Parts {
			if part.Text != "" {
				sb.WriteString(part.Text)
			}
			if part.FunctionCall != nil {
				argsBytes, _ := json.Marshal(part.FunctionCall.Args)
				toolCalls = append(toolCalls, model.ToolCall{
					ID:   fmt.Sprintf("call_%d_%s", time.Now().UnixNano(), part.FunctionCall.Name),
					Type: "function",
					Function: model.FunctionCall{
						Name:      part.FunctionCall.Name,
						Arguments: string(argsBytes),
					},
				})
			}
		}
		replyText = sb.String()
		if len(toolCalls) > 0 {
			finishReason = "tool_calls"
		} else if cand.FinishReason == "MAX_TOKENS" {
			finishReason = "length"
		}
	}

	var usage *model.Usage
	if geminiResp.UsageMetadata != nil {
		usage = &model.Usage{
			PromptTokens:     geminiResp.UsageMetadata.PromptTokenCount,
			CompletionTokens: geminiResp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      geminiResp.UsageMetadata.TotalTokenCount,
		}
	}

	return &model.ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-gemini-%d", time.Now().Unix()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []model.ChatCompletionChoice{
			{
				Index: 0,
				Message: model.ChatMessage{
					Role:      "assistant",
					Content:   replyText,
					ToolCalls: toolCalls,
				},
				FinishReason: &finishReason,
			},
		},
		Usage: usage,
	}, nil
}

// ChatCompleteStream executes a streaming Gemini call and converts SSE events to OpenAI format.
func (p *GeminiProvider) ChatCompleteStream(ctx context.Context, req *model.ChatCompletionRequest, channel *model.ChannelConfig) (<-chan *model.StreamEvent, error) {
	geminiReq := convertOpenAIToGemini(req)
	payloadBytes, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal gemini request error: %w", err)
	}

	targetModel := channel.GetUpstreamModel(req.Model)
	targetURL := buildGeminiURL(channel.BaseURL, targetModel, true, channel.APIKey)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("create gemini stream request error: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("x-goog-api-key", channel.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("connect to gemini stream error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("upstream gemini %s returned %d: %s", channel.Name, resp.StatusCode, string(bodyBytes))
	}

	eventChan := make(chan *model.StreamEvent, 64)

	go func() {
		defer resp.Body.Close()
		defer close(eventChan)

		reader := bufio.NewReader(resp.Body)
		msgID := fmt.Sprintf("chatcmpl-gemini-%d", time.Now().UnixNano())
		created := time.Now().Unix()

		for {
			select {
			case <-ctx.Done():
				eventChan <- &model.StreamEvent{Err: ctx.Err()}
				return
			default:
			}

			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err != io.EOF {
					eventChan <- &model.StreamEvent{Err: err}
				}
				return
			}

			lineStr := strings.TrimSpace(string(line))
			if lineStr == "" {
				continue
			}

			if !strings.HasPrefix(lineStr, "data:") {
				continue
			}

			dataContent := strings.TrimSpace(strings.TrimPrefix(lineStr, "data:"))
			if dataContent == "[DONE]" {
				eventChan <- &model.StreamEvent{IsDone: true}
				return
			}

			var geminiChunk GeminiResponse
			if err := json.Unmarshal([]byte(dataContent), &geminiChunk); err != nil {
				continue
			}

			textDelta := ""
			var toolCalls []model.ToolCall
			var finishReason *string
			if len(geminiChunk.Candidates) > 0 {
				cand := geminiChunk.Candidates[0]
				for _, part := range cand.Content.Parts {
					if part.Text != "" {
						textDelta += part.Text
					}
					if part.FunctionCall != nil {
						argsBytes, _ := json.Marshal(part.FunctionCall.Args)
						toolCalls = append(toolCalls, model.ToolCall{
							ID:   fmt.Sprintf("call_%d_%s", time.Now().UnixNano(), part.FunctionCall.Name),
							Type: "function",
							Function: model.FunctionCall{
								Name:      part.FunctionCall.Name,
								Arguments: string(argsBytes),
							},
						})
					}
				}
				if len(toolCalls) > 0 {
					r := "tool_calls"
					finishReason = &r
				} else if cand.FinishReason != "" {
					r := strings.ToLower(cand.FinishReason)
					if r == "max_tokens" {
						r = "length"
					} else {
						r = "stop"
					}
					finishReason = &r
				}
			}

			var usage *model.Usage
			if geminiChunk.UsageMetadata != nil {
				usage = &model.Usage{
					PromptTokens:     geminiChunk.UsageMetadata.PromptTokenCount,
					CompletionTokens: geminiChunk.UsageMetadata.CandidatesTokenCount,
					TotalTokens:      geminiChunk.UsageMetadata.TotalTokenCount,
				}
			}

			chunk := &model.ChatCompletionChunk{
				ID:      msgID,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   req.Model,
				Choices: []model.ChunkChoice{
					{
						Index: 0,
						Delta: model.ChunkDelta{
							Content:   textDelta,
							ToolCalls: toolCalls,
						},
						FinishReason: finishReason,
					},
				},
				Usage: usage,
			}

			eventChan <- &model.StreamEvent{Chunk: chunk}
		}
	}()

	return eventChan, nil
}

// Embed executes embedding generation on Google Gemini Developer API.
func (p *GeminiProvider) Embed(ctx context.Context, req *model.EmbeddingRequest, channel *model.ChannelConfig) (*model.EmbeddingResponse, error) {
	targetModel := channel.GetUpstreamModel(req.Model)
	texts := req.GetInputStrings()
	if len(texts) == 0 {
		return nil, fmt.Errorf("input text is required for embedding")
	}

	baseURL := channel.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	var items []model.EmbeddingItem
	totalTokens := 0

	if len(texts) == 1 {
		// Single embedContent call
		targetURL := fmt.Sprintf("%s/v1beta/models/%s:embedContent?key=%s", baseURL, targetModel, channel.APIKey)
		type singleReq struct {
			Model   string `json:"model,omitempty"`
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		}
		var sReq singleReq
		sReq.Model = "models/" + strings.TrimPrefix(targetModel, "models/")
		sReq.Content.Parts = []struct {
			Text string `json:"text"`
		}{{Text: texts[0]}}

		payload, _ := json.Marshal(sReq)
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("x-goog-api-key", channel.APIKey)

		resp, err := p.client.Do(httpReq)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("gemini embed returned %d: %s", resp.StatusCode, string(body))
		}

		var parsed struct {
			Embedding struct {
				Values []float64 `json:"values"`
			} `json:"embedding"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, err
		}
		items = append(items, model.EmbeddingItem{
			Object:    "embedding",
			Index:     0,
			Embedding: parsed.Embedding.Values,
		})
		totalTokens = len(texts[0]) / 4
		if totalTokens == 0 {
			totalTokens = 1
		}
	} else {
		// batchEmbedContents call
		targetURL := fmt.Sprintf("%s/v1beta/models/%s:batchEmbedContents?key=%s", baseURL, targetModel, channel.APIKey)
		type itemReq struct {
			Model   string `json:"model"`
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		}
		type batchReq struct {
			Requests []itemReq `json:"requests"`
		}
		var bReq batchReq
		modelPath := "models/" + strings.TrimPrefix(targetModel, "models/")
		for _, t := range texts {
			toks := len(t) / 4
			if toks == 0 {
				toks = 1
			}
			totalTokens += toks
			bReq.Requests = append(bReq.Requests, itemReq{
				Model: modelPath,
				Content: struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				}{
					Parts: []struct {
						Text string `json:"text"`
					}{{Text: t}},
				},
			})
		}

		payload, _ := json.Marshal(bReq)
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("x-goog-api-key", channel.APIKey)

		resp, err := p.client.Do(httpReq)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("gemini batchEmbed returned %d: %s", resp.StatusCode, string(body))
		}

		var parsed struct {
			Embeddings []struct {
				Values []float64 `json:"values"`
			} `json:"embeddings"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, err
		}
		for idx, emb := range parsed.Embeddings {
			items = append(items, model.EmbeddingItem{
				Object:    "embedding",
				Index:     idx,
				Embedding: emb.Values,
			})
		}
	}

	return &model.EmbeddingResponse{
		Object: "list",
		Data:   items,
		Model:  req.Model,
		Usage: model.EmbeddingUsage{
			PromptTokens: totalTokens,
			TotalTokens:  totalTokens,
		},
	}, nil
}
