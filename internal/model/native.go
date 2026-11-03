package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
)

const (
	NativeChat               = "chat"
	NativeResponses          = "responses"
	NativeMessages           = "messages"
	NativeGemini             = "gemini"
	NativeGeminiInteractions = "gemini_interactions"
	NativeEmbeddings         = "embeddings"
	NativeGeminiEmbedding    = "gemini_embedding"
	NativeRerank             = "rerank"
)

// NativeRequest retains the ingress wire document beside the canonical request.
// The latter is used only for accounting and for an upstream that needs translation.
type NativeRequest struct {
	ShouldRecord func() bool
	AfterRecord  func()
	BillingID    string
	Protocol     string
	Body         json.RawMessage
	Headers      map[string]string
	// TranslationError prevents silently dropping a capability without an
	// equivalent in another protocol. Native upstreams still receive it intact.
	TranslationError error
	// TranslateOnly is used for a continuation whose history lives in the
	// gateway rather than in a native upstream's response store.
	TranslateOnly  bool
	PinnedIdentity string
	PriorTokens    int
}

// UpstreamIdentity groups duplicate channels without retaining a plaintext key.
func UpstreamIdentity(ch *ChannelConfig) string {
	endpoint := strings.TrimRight(ch.BaseURL, "/")
	if u, err := url.Parse(endpoint); err == nil {
		u.Scheme, u.Host = strings.ToLower(u.Scheme), strings.ToLower(u.Host)
		endpoint = u.String()
	}
	sum := sha256.Sum256([]byte(endpoint + "\x00" + ch.APIKey))
	return hex.EncodeToString(sum[:])
}
