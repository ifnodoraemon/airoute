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
	FileData         *GeminiFileData         `json:"fileData,omitempty"`
	FunctionCall     *GeminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *GeminiFunctionResponse `json:"functionResponse,omitempty"`
}

// GeminiInlineData holds base64 encoded multimodal payload.
type GeminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"` // base64 encoded
}

// GeminiFileData references a remote resource instead of inlining it. It is the
// only Gemini representation that accepts a URI, so an OpenAI image_url carrying
// an https URL must be forwarded here rather than in inlineData.data (F-07).
type GeminiFileData struct {
	MimeType string `json:"mimeType,omitempty"`
	FileURI  string `json:"fileUri"`
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
	ToolConfig        *GeminiToolConfig                `json:"toolConfig,omitempty"`
}

// GeminiToolConfig constrains how the model may call the declared functions.
type GeminiToolConfig struct {
	FunctionCallingConfig *GeminiFunctionCallingConfig `json:"functionCallingConfig,omitempty"`
}

// GeminiFunctionCallingConfig is Gemini's tool_choice equivalent; Mode is one of
// AUTO / ANY / NONE, and AllowedFunctionNames narrows ANY to specific tools.
type GeminiFunctionCallingConfig struct {
	Mode                 string   `json:"mode,omitempty"`
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

// GeminiUsageMetadata reports token counts.
//
// Thinking models (gemini-2.5-flash/pro and later) report the tokens spent on
// reasoning separately in thoughtsTokenCount: candidatesTokenCount holds only the
// visible answer, while totalTokenCount already includes the thoughts and Google
// bills them as output tokens. Dropping the field under-counted completion by an
// order of magnitude on long reasoning turns (BUG-GEMINI-THOUGHTS).
type GeminiUsageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount,omitempty"`
	TotalTokenCount         int `json:"totalTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount,omitempty"`
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

// GeminiError is an error payload Gemini (or a Gemini-compatible proxy) can put
// inside an HTTP 200 response or an SSE data frame.
type GeminiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// GeminiResponse is the response from generateContent.
type GeminiResponse struct {
	Candidates    []GeminiCandidate    `json:"candidates"`
	UsageMetadata *GeminiUsageMetadata `json:"usageMetadata,omitempty"`
	// Error is populated when the upstream reports a failure inside a 200 body
	// (mid-stream overload, quota, safety) instead of an HTTP error status.
	Error *GeminiError `json:"error,omitempty"`
}
