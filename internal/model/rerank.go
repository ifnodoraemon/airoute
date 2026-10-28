package model

import "encoding/json"

// =============================================================================
// Reranking Models (RAG Document Re-scoring APIs)
// =============================================================================

// RerankRequest represents the standard cross-encoder rerank payload.
type RerankRequest struct {
	Model           string `json:"model"`
	Query           string `json:"query"`
	Documents       []any  `json:"documents"` // []string or []map[string]any
	TopN            *int   `json:"top_n,omitempty"`
	ReturnDocuments *bool  `json:"return_documents,omitempty"`
}

// GetDocumentTexts returns all documents as a slice of strings.
func (r *RerankRequest) GetDocumentTexts() []string {
	var texts []string
	for _, doc := range r.Documents {
		if s, ok := doc.(string); ok {
			texts = append(texts, s)
		} else if m, ok := doc.(map[string]any); ok {
			if t, ok := m["text"].(string); ok {
				texts = append(texts, t)
			}
		}
	}
	return texts
}

// RerankResultDocument holds the document text when return_documents is true.
type RerankResultDocument struct {
	Text string `json:"text"`
}

// UnmarshalJSON accepts both shapes seen in the wild: {"text": "..."} (Cohere,
// Jina) and a bare string. Rejecting the string form made the gateway fail on
// upstreams whose rerank payload is otherwise valid.
func (d *RerankResultDocument) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		d.Text = text
		return nil
	}
	type alias RerankResultDocument
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*d = RerankResultDocument(decoded)
	return nil
}

// RerankItem represents an individual re-scored document.
type RerankItem struct {
	Index          int                   `json:"index"`
	RelevanceScore float64               `json:"relevance_score"`
	Document       *RerankResultDocument `json:"document,omitempty"`
}

// RerankUsage reports token consumption for rerank scoring. Upstreams disagree on
// the shape (Jina/SiliconFlow report prompt_tokens, Cohere reports billed_units),
// so prompt_tokens is preserved when present instead of being dropped.
type RerankUsage struct {
	PromptTokens int `json:"prompt_tokens,omitempty"`
	TotalTokens  int `json:"total_tokens"`
}

// RerankResponse represents the standard response containing re-ranked documents.
type RerankResponse struct {
	Model   string       `json:"model"`
	Results []RerankItem `json:"results"`
	Usage   RerankUsage  `json:"usage"`

	// Extra preserves response members this struct does not model (id, created,
	// meta, ...) so a same-protocol upstream's answer is not silently rewritten.
	Extra ExtraFields `json:"-"`

	// Channel is the routing chain that served this request (e.g. "A" or
	// "A→B" after fallback). Internal-only metadata for audit logging.
	Channel string `json:"-"`
}
