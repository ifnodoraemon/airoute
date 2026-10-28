package api

import (
	"bytes"
	"encoding/json"
)

// maxResponseModelRestoreBytes bounds the body this helper is willing to decode.
// Multimodal JSON responses (image metadata, video task status) are small; a
// larger body is passed through untouched instead of being buffered a second
// time on the request path.
const maxResponseModelRestoreBytes = 4 << 20

// restoreResponseModelJSON rewrites only the top-level "model" field of a JSON
// response body to requested, the model the caller asked for and the one the
// audit row records.
//
// A provider may echo its own upstream name — or omit the field — so an
// OpenAI-compatible client that keys on response.model observed a different
// model than it requested, and with a channel model mapping that name was an
// internal alias the client never configured. Only the top level is touched;
// nested values are carried through byte-for-byte as json.RawMessage (key order
// may change, values do not).
//
// The body is returned unchanged when it is not a JSON object, when it has no
// "model" field, or when it exceeds the cap. Binary and multipart payloads
// (audio streams, generated images) must never be decoded here, so the leading
// '{' check rejects them before any allocation.
func restoreResponseModelJSON(body []byte, requested string) []byte {
	if requested == "" || len(body) == 0 || len(body) > maxResponseModelRestoreBytes {
		return body
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return body
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return body
	}
	if _, ok := payload["model"]; !ok {
		return body
	}
	requestedJSON, err := json.Marshal(requested)
	if err != nil {
		return body
	}
	// Already correct: most providers echo the requested name, so this keeps the
	// helper a cheap no-op when several response paths run it in sequence.
	if bytes.Equal(bytes.TrimSpace(payload["model"]), requestedJSON) {
		return body
	}
	payload["model"] = requestedJSON
	restored, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return restored
}
