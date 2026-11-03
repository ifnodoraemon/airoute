package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// GeminiProvider handles the Gemini Developer API and compatible REST endpoints.
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

// buildGeminiURL creates the Google endpoint URL using the shared API root
// semantics: a bare origin becomes /v1beta, a root that already carries /v1beta
// is not doubled, and custom prefixes are preserved.
func buildGeminiURL(baseURL, modelName string, stream bool) string {
	if strings.Contains(baseURL, "/publishers/google") {
		action := "generateContent"
		if stream {
			action = "streamGenerateContent"
		}
		target := GeminiActionURL(baseURL, modelName, action)
		if stream {
			target += "?alt=sse"
		}
		return target
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	// Accept the "models/<id>" form Google documents (F-03).
	modelPath := geminiModelPath(modelName)
	endpoint := "/v1beta/models/" + modelPath + ":generateContent"
	if stream {
		return joinAPIURLVersioned(baseURL, GeminiAPIVersion, "/v1beta/models/"+modelPath+":streamGenerateContent", url.Values{"alt": {"sse"}})
	}
	return joinAPIURLVersioned(baseURL, GeminiAPIVersion, endpoint, nil)
}

// ChatComplete executes a non-streaming Gemini call and converts to OpenAI format.
func (p *GeminiProvider) ChatComplete(ctx context.Context, req *model.ChatCompletionRequest, channel *model.ChannelConfig) (*model.ChatCompletionResponse, error) {
	if response, done, err := maybeNativeUnary(ctx, p.client, req, channel, model.NativeGemini); done {
		return response, err
	}
	var historyErr error
	req, historyErr = restoreGeminiHistory(ctx, req, channel)
	if historyErr != nil {
		return nil, historyErr
	}
	if err := validateGeminiChatOptions(req); err != nil {
		return nil, err
	}
	geminiReq := convertOpenAIToGemini(req)
	targetModel := channel.GetUpstreamModel(req.Model)
	if err := applyGeminiThinking(geminiReq.GenerationConfig, req, targetModel); err != nil {
		return nil, err
	}
	payloadBytes, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal gemini request error: %w", err)
	}

	targetURL := buildGeminiURL(GeminiBaseURL(channel), targetModel, false)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("create gemini http request error: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if err := ConfigureGoogleRequest(ctx, httpReq, channel); err != nil {
		return nil, err
	}

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
		return nil, NewUpstreamError(channel.Name, resp.StatusCode, bodyBytes, resp.Header)
	}

	var geminiResp GeminiResponse
	if err := json.Unmarshal(bodyBytes, &geminiResp); err != nil {
		return nil, fmt.Errorf("unmarshal gemini response error: %w", err)
	}
	// A 200 body can still carry an error object (overload/quota/model failure);
	// returning an empty completion for it hid the failure from the client, the
	// audit row and the circuit breaker.
	if geminiResp.Error != nil {
		return nil, NewUpstreamError(channel.Name, streamErrorStatus(geminiResp.Error.Code, geminiResp.Error.Status), bodyBytes, resp.Header)
	}

	choices := geminiResponseChoices(&geminiResp)
	for i := range choices {
		if i < len(geminiResp.Candidates) {
			if err := preserveGeminiMessage(ctx, channel, req.Model, geminiResp.Candidates[i].Content.Parts, &choices[i].Message); err != nil {
				return nil, err
			}
		}
	}
	if len(choices) == 0 {
		return nil, fmt.Errorf("Gemini response contains neither candidates nor prompt feedback")
	}

	var usage *model.Usage
	if geminiResp.UsageMetadata != nil {
		usage = geminiUsageToModel(geminiResp.UsageMetadata)
	}

	return &model.ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-gemini-%d", time.Now().Unix()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: choices,
		Usage:   usage,
	}, nil
}
