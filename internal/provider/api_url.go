package provider

import (
	"net/url"
	"strings"
)

// APIURL treats a configured path as the API root. A bare origin uses /v1.
func APIURL(baseURL, path string) string {
	base := strings.TrimRight(baseURL, "/")
	endpoint := strings.TrimPrefix(path, "/v1/")
	parsed, err := url.Parse(base)
	if err != nil {
		return base + "/" + strings.TrimLeft(endpoint, "/")
	}
	if parsed.Path == "" || parsed.Path == "/" {
		parsed.Path = "/v1"
	}
	joined, err := url.JoinPath(parsed.String(), endpoint)
	if err != nil {
		return base + "/" + strings.TrimLeft(endpoint, "/")
	}
	return joined
}
