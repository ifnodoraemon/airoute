package provider

import (
	"net/url"
	"strings"
)

// GeminiResourceURL preserves a relay's prefix and upload namespace. The path
// comes only from registered gateway routes or a validated upload response.
func GeminiResourceURL(base, path string) string {
	if base == "" {
		base = "https://generativelanguage.googleapis.com"
	}
	u, err := url.Parse(base)
	if err != nil {
		return ""
	}
	parts := strings.SplitN(path, "?", 2)
	root := strings.TrimRight(u.Path, "/")
	root = strings.TrimSuffix(root, "/v1beta")
	root = strings.TrimSuffix(root, "/v1")
	if strings.Contains(root, "/publishers/google") {
		if i := strings.Index(root, "/publishers/google"); i >= 0 {
			root = root[:i]
		}
		parts[0] = strings.TrimPrefix(strings.TrimPrefix(parts[0], "/v1beta"), "/v1")
	}
	u.Path = root + "/" + strings.TrimLeft(parts[0], "/")
	u.RawPath = ""
	u.RawQuery = ""
	if len(parts) > 1 {
		u.RawQuery = parts[1]
	}
	u.User = nil
	return u.String()
}
