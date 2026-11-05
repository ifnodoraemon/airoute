package storage

import "strings"

// channelIdentity separates the terminal upstream from a legacy routing path.
// The path remains intact for diagnosis, including repeated names (retries).
func channelIdentity(channel, trail string) (string, string) {
	channel = strings.TrimSpace(channel)
	if !strings.Contains(channel, "→") {
		return channel, trail
	}
	if trail == "" {
		trail = channel
	}
	parts := strings.Split(channel, "→")
	for i := len(parts) - 1; i >= 0; i-- {
		if name := strings.TrimSpace(parts[i]); name != "" {
			return name, trail
		}
	}
	return "", trail
}

func hasChannelFailover(trail string) bool {
	var previous string
	for _, part := range strings.Split(trail, "→") {
		name := strings.TrimSpace(part)
		if name == "" || name == "…" {
			continue
		}
		if previous != "" && name != previous {
			return true
		}
		previous = name
	}
	return false
}
