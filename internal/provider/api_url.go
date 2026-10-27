package provider

import (
	"net/url"
	"regexp"
	"strings"
)

const (
	// DefaultAPIVersion is the version prefix OpenAI-style endpoints use when a
	// channel configures a bare origin as its base URL.
	DefaultAPIVersion = "v1"
	// GeminiAPIVersion is the version prefix Gemini endpoints use.
	GeminiAPIVersion = "v1beta"
)

// APIURL joins a configured API root with an OpenAI-style endpoint path.
//
// The configured base URL is the API root: it may be a bare origin, may already
// carry the version ("/v1"), or may be a provider-specific prefix ("/v1-openai",
// "/api/paas/v4", "/api/v3"). A bare origin gets DefaultAPIVersion. The version
// is never concatenated twice, and a root that already points at the endpoint is
// used verbatim.
//
// This is the single interpretation of one channel configuration: chat, text
// completion, embeddings, rerank and the multimodal paths all go through it, so
// the same base URL cannot mean different things per endpoint.
func APIURL(baseURL, path string) string {
	return joinAPIURLQuery(baseURL, DefaultAPIVersion, path, nil)
}

// joinAPIURLVersioned joins an API root with an endpoint path, injecting version
// into a prefixed root unless the root already ends with a version segment (for
// example a proxy root "/gemini" gets "/gemini/v1beta/models/...", while
// "/v1beta" is left alone instead of becoming "/v1beta/v1beta").
func joinAPIURLVersioned(baseURL, version, endpointPath string, query url.Values) string {
	return joinAPIURLVersionedWithPolicy(baseURL, version, endpointPath, query, true)
}

func joinAPIURLQuery(baseURL, version, endpointPath string, query url.Values) string {
	return joinAPIURLVersionedWithPolicy(baseURL, version, endpointPath, query, false)
}

// versionSegmentPattern matches the trailing path segment of an API root that
// already selects a version, e.g. "v1", "v1beta", "v4".
var versionSegmentPattern = regexp.MustCompile(`^v[0-9]+[a-z]*[0-9]*$`)

// rootCarriesVersion reports whether the last path segment of root is a version
// selector, so the version must not be concatenated again.
func rootCarriesVersion(rootPath string) bool {
	if rootPath == "" {
		return false
	}
	segment := rootPath[strings.LastIndex(rootPath, "/")+1:]
	return versionSegmentPattern.MatchString(segment)
}

// joinAPIURLVersionedWithPolicy joins an API root with an endpoint path and
// optional query values (merged with any query already present on the root).
// injectIntoPrefix applies the family version to a prefixed root as well.
func joinAPIURLVersionedWithPolicy(baseURL, version, endpointPath string, query url.Values, injectIntoPrefix bool) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	endpoint := strings.TrimLeft(endpointPath, "/")
	if version != "" {
		// The version belongs to the root, so an endpoint that spells it out is
		// reduced to its relative form: the version is written exactly once.
		endpoint = strings.TrimPrefix(endpoint, version+"/")
	}
	if endpoint == "" {
		endpoint = version
	}

	parsed, err := url.Parse(base)
	if err != nil {
		return mergeQuery(base+"/"+endpoint, query)
	}
	rootPath := strings.TrimRight(parsed.Path, "/")

	// Operators do paste a full endpoint URL into the base URL field; keep that
	// working instead of doubling the path.
	if rootPath != "" && endpoint != "" && strings.HasSuffix(rootPath, "/"+endpoint) {
		return mergeQuery(base, query)
	}

	// A bare origin always gets the family version. A prefixed root gets it only
	// for families whose endpoints always carry the version (Gemini) and only when
	// the operator did not already select a version.
	switch {
	case rootPath == "" && version != "":
		parsed.Path = "/" + version
	case rootPath == "":
		parsed.Path = "/"
	case injectIntoPrefix && version != "" && !rootCarriesVersion(rootPath):
		parsed.Path = rootPath + "/" + version
	default:
		parsed.Path = rootPath
	}

	joined, err := url.JoinPath(parsed.String(), endpoint)
	if err != nil {
		return mergeQuery(base+"/"+endpoint, query)
	}
	return mergeQuery(joined, query)
}

// mergeQuery appends query values to a URL string, preserving an existing query.
func mergeQuery(rawURL string, query url.Values) string {
	if len(query) == 0 {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if parsed.RawQuery == "" {
		parsed.RawQuery = query.Encode()
		return parsed.String()
	}
	parsed.RawQuery += "&" + query.Encode()
	return parsed.String()
}
