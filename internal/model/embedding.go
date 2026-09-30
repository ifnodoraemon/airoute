package model

// =============================================================================
// Embeddings Models (Vector / Embedding APIs)
// =============================================================================

// EmbeddingRequest represents the standard OpenAI embedding request payload.
type EmbeddingRequest struct {
	Input          any    `json:"input"`                     // string, []string, []int, or [][]int
	Model          string `json:"model"`                     // model identifier
	EncodingFormat string `json:"encoding_format,omitempty"` // "float" or "base64"
	Dimensions     *int   `json:"dimensions,omitempty"`      // number of dimensions for output embedding
	User           string `json:"user,omitempty"`
}

// GetInputStrings returns input as a slice of strings.
func (r *EmbeddingRequest) GetInputStrings() []string {
	if r.Input == nil {
		return nil
	}
	if s, ok := r.Input.(string); ok {
		return []string{s}
	}
	if list, ok := r.Input.([]any); ok {
		var res []string
		for _, item := range list {
			if s, ok := item.(string); ok {
				res = append(res, s)
			}
		}
		return res
	}
	if list, ok := r.Input.([]string); ok {
		return list
	}
	return nil
}

// EmbeddingItem holds an individual embedding vector.
type EmbeddingItem struct {
	Object    string    `json:"object"`    // always "embedding"
	Index     int       `json:"index"`     // index of input item
	Embedding []float64 `json:"embedding"` // floating point vector
}

// EmbeddingUsage reports token consumption for embedding generation.
type EmbeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// EmbeddingResponse represents the standard response containing generated embeddings.
type EmbeddingResponse struct {
	Object string          `json:"object"` // "list"
	Data   []EmbeddingItem `json:"data"`
	Model  string          `json:"model"`
	Usage  EmbeddingUsage  `json:"usage"`

	// Channel is the routing chain that served this request (e.g. "A" or
	// "A→B" after fallback). Internal-only metadata for audit logging.
	Channel string `json:"-"`
}
