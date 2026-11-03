package model

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"sync"
)

// ExtraFields holds JSON object fields this gateway does not model explicitly.
//
// They are captured on decode and re-emitted on encode, so a field the gateway
// has never heard of (response_format, seed, logprobs, system_fingerprint,
// service_tier, ...) is not silently dropped on the way to a same-protocol
// upstream. Cross-protocol conversion still uses the explicit field mapping in
// the adapters: there is no place to put a field the target protocol cannot
// express, and pretending otherwise would send garbage upstream.
type ExtraFields map[string]json.RawMessage

var knownJSONFieldsCache sync.Map // reflect.Type -> map[string]struct{}

// knownJSONFields returns the JSON object keys a struct type models itself.
// It is derived from the struct tags once per type, so adding a field to the
// struct automatically stops that key from being treated as an unknown extra.
func knownJSONFields(t reflect.Type) map[string]struct{} {
	if cached, ok := knownJSONFieldsCache.Load(t); ok {
		return cached.(map[string]struct{})
	}
	known := make(map[string]struct{}, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" { // unexported: encoding/json ignores it
			continue
		}
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name := tag
		if idx := strings.IndexByte(tag, ','); idx >= 0 {
			name = tag[:idx]
		}
		if name == "" {
			name = field.Name
		}
		known[name] = struct{}{}
	}
	knownJSONFieldsCache.Store(t, known)
	return known
}

// captureExtraFields returns the unknown members of a JSON object, or nil when
// every member is modeled by the struct type. Callers use the alias struct type
// (for example "type alias Usage") so the result does not recurse.
func captureExtraFields(data []byte, t reflect.Type) ExtraFields {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil || len(object) == 0 {
		return nil
	}
	for name := range knownJSONFields(t) {
		delete(object, name)
	}
	if len(object) == 0 {
		return nil
	}
	return ExtraFields(object)
}

// appendExtraFields appends unknown members to an already-marshaled JSON
// object. Known fields keep their struct order; unknown fields are appended in
// sorted key order, so the wire form is deterministic without reordering the
// fields the gateway does understand. Values are raw JSON produced by the
// decoder, so they are re-emitted byte-for-byte.
func appendExtraFields(base []byte, extra ExtraFields) ([]byte, error) {
	if len(extra) == 0 {
		return base, nil
	}
	trimmed := bytes.TrimSpace(base)
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return base, nil
	}
	body := trimmed[:len(trimmed)-1]
	keys := make([]string, 0, len(extra))
	for key := range extra {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	buf.Write(body)
	hasField := len(bytes.TrimSpace(body)) > 1 // body still contains "{" only for {}
	for _, key := range keys {
		if hasField {
			buf.WriteByte(',')
		}
		keyJSON, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		buf.Write(keyJSON)
		buf.WriteByte(':')
		buf.Write(extra[key])
		hasField = true
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// marshalWithExtra marshals the alias struct (which has no MarshalJSON method)
// and appends the preserved unknown fields.
func marshalWithExtra(base any, extra ExtraFields) ([]byte, error) {
	encoded, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	return appendExtraFields(encoded, extra)
}

// ChatCompletionRequest.

func (r *ChatCompletionRequest) UnmarshalJSON(data []byte) error {
	type alias ChatCompletionRequest
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = ChatCompletionRequest(decoded)
	r.Extra = captureExtraFields(data, reflect.TypeOf(decoded))
	return nil
}

func (r ChatCompletionRequest) MarshalJSON() ([]byte, error) {
	type alias ChatCompletionRequest
	return marshalWithExtra(alias(r), r.Extra)
}

// ChatMessage.

func (m *ChatMessage) UnmarshalJSON(data []byte) error {
	type alias ChatMessage
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*m = ChatMessage(decoded)
	m.Extra = captureExtraFields(data, reflect.TypeOf(decoded))
	return nil
}

func (m ChatMessage) MarshalJSON() ([]byte, error) {
	type alias ChatMessage
	return marshalWithExtra(alias(m), m.Extra)
}

// ChatCompletionChoice.

func (c *ChatCompletionChoice) UnmarshalJSON(data []byte) error {
	type alias ChatCompletionChoice
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*c = ChatCompletionChoice(decoded)
	c.Extra = captureExtraFields(data, reflect.TypeOf(decoded))
	return nil
}

func (c ChatCompletionChoice) MarshalJSON() ([]byte, error) {
	type alias ChatCompletionChoice
	return marshalWithExtra(alias(c), c.Extra)
}

// ChatCompletionResponse.

func (r *ChatCompletionResponse) UnmarshalJSON(data []byte) error {
	type alias ChatCompletionResponse
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = ChatCompletionResponse(decoded)
	r.Extra = captureExtraFields(data, reflect.TypeOf(decoded))
	return nil
}

func (r ChatCompletionResponse) MarshalJSON() ([]byte, error) {
	type alias ChatCompletionResponse
	return marshalWithExtra(alias(r), r.Extra)
}

// RerankResponse keeps unknown members (id/created/meta/...) so a same-protocol
// upstream's rerank answer survives the canonical round trip.

func (r *RerankResponse) UnmarshalJSON(data []byte) error {
	type alias RerankResponse
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = RerankResponse(decoded)
	r.Extra = captureExtraFields(data, reflect.TypeOf(decoded))
	return nil
}

func (r RerankResponse) MarshalJSON() ([]byte, error) {
	type alias RerankResponse
	return marshalWithExtra(alias(r), r.Extra)
}

// ChunkDelta.

func (d *ChunkDelta) UnmarshalJSON(data []byte) error {
	type alias ChunkDelta
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*d = ChunkDelta(decoded)
	d.Extra = captureExtraFields(data, reflect.TypeOf(decoded))
	return nil
}

func (d ChunkDelta) MarshalJSON() ([]byte, error) {
	type alias ChunkDelta
	return marshalWithExtra(alias(d), d.Extra)
}

// ChunkChoice.

func (c *ChunkChoice) UnmarshalJSON(data []byte) error {
	type alias ChunkChoice
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*c = ChunkChoice(decoded)
	c.Extra = captureExtraFields(data, reflect.TypeOf(decoded))
	return nil
}

func (c ChunkChoice) MarshalJSON() ([]byte, error) {
	type alias ChunkChoice
	return marshalWithExtra(alias(c), c.Extra)
}

// ChatCompletionChunk.

func (c *ChatCompletionChunk) UnmarshalJSON(data []byte) error {
	type alias ChatCompletionChunk
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*c = ChatCompletionChunk(decoded)
	c.Extra = captureExtraFields(data, reflect.TypeOf(decoded))
	return nil
}

func (c ChatCompletionChunk) MarshalJSON() ([]byte, error) {
	type alias ChatCompletionChunk
	return marshalWithExtra(alias(c), c.Extra)
}

// PromptTokensDetails.

func (d *PromptTokensDetails) UnmarshalJSON(data []byte) error {
	type alias PromptTokensDetails
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*d = PromptTokensDetails(decoded)
	d.Extra = captureExtraFields(data, reflect.TypeOf(decoded))
	return nil
}

func (d PromptTokensDetails) MarshalJSON() ([]byte, error) {
	type alias PromptTokensDetails
	return marshalWithExtra(alias(d), d.Extra)
}

// Usage keeps its normalization in usage.go; only the unknown-field capture and
// the encode side live here.

func (u Usage) MarshalJSON() ([]byte, error) {
	type alias Usage
	return marshalWithExtra(alias(u), u.Extra)
}
