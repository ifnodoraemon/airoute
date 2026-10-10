package provider

// GeminiFunctionDeclaration declares a callable function tool in Gemini schema.
type GeminiFunctionDeclaration struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

// GeminiToolDeclarationContainer holds function tool declarations.
type GeminiToolDeclarationContainer struct {
	FunctionDeclarations []GeminiFunctionDeclaration `json:"functionDeclarations,omitempty"`
}

// GeminiFunctionCall represents a model invocation of a declared tool.
type GeminiFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

// GeminiFunctionResponse represents the user response to a tool invocation.
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

// GeminiInlineData holds base64 encoded multimodal payload.
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
