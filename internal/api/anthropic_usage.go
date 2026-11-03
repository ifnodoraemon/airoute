package api

// nativeCacheCreationTokens reads Anthropic's cache_creation_input_tokens out of a
// native (protocol-verbatim) usage object.
//
// C-02: the value counts prompt-cache writes, which are invoiced at a premium over
// the plain input rate. The unary Anthropic path used to read only
// cache_read_input_tokens, so writes were billed as ordinary prompt tokens. The
// streaming path already parsed this field; this helper is shared by both so the
// two transports cannot drift apart again.
func nativeCacheCreationTokens(usage any) int {
	fields, ok := usage.(map[string]any)
	if !ok || fields == nil {
		return 0
	}
	return intFromJSONNumber(fields["cache_creation_input_tokens"])
}
