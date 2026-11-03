package model

// IsRetiredProtocol is kept only to reject old configuration safely. Removing a
// channel's sole protocol must never turn it into an unrestricted channel.
func IsRetiredProtocol(protocol string) bool {
	return protocol == "openai_text" || protocol == "completion"
}
