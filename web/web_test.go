package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterStaticRoutes(r)
	return r
}

// TestSPAShellMustNotBeCached is a regression test for "refreshing the console
// logs me out": the SPA shell was served with no cache headers at all, so the
// browser could reuse a stale index.html that referenced asset filenames from a
// previous build. Those chunks no longer exist, the module graph failed to
// load, React never mounted, and the console fell back to the login screen.
func TestSPAShellMustNotBeCached(t *testing.T) {
	r := newTestRouter()

	for _, path := range []string{"/", "/app/", "/app/logs"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d", path, w.Code)
		}
		cc := w.Header().Get("Cache-Control")
		if !strings.Contains(cc, "no-cache") {
			t.Fatalf("%s: shell must require revalidation, got Cache-Control=%q", path, cc)
		}
		if w.Header().Get("ETag") == "" {
			t.Fatalf("%s: shell should carry an ETag for cheap revalidation", path)
		}
	}
}

// TestSPAShellSupportsConditionalRequests verifies the mandatory revalidation
// resolves to a 304 rather than re-sending the shell on every navigation.
func TestSPAShellSupportsConditionalRequests(t *testing.T) {
	r := newTestRouter()

	first := httptest.NewRecorder()
	r.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/", nil))
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}

	second := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("If-None-Match", etag)
	r.ServeHTTP(second, req)

	if second.Code != http.StatusNotModified {
		t.Fatalf("expected 304 for matching ETag, got %d", second.Code)
	}
}

// TestHashedAssetsAreImmutable ensures we keep the long-lived cache for
// content-hashed bundles (they can never collide with a future build).
func TestHashedAssetsAreImmutable(t *testing.T) {
	r := newTestRouter()

	// Discover a real hashed asset from the served shell.
	shell := httptest.NewRecorder()
	r.ServeHTTP(shell, httptest.NewRequest(http.MethodGet, "/", nil))

	idx := strings.Index(shell.Body.String(), "assets/")
	if idx < 0 {
		t.Skip("no hashed asset referenced by the shell")
	}
	rest := shell.Body.String()[idx:]
	name := rest
	if end := strings.IndexAny(rest, "\"'"); end > 0 {
		name = rest[:end]
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/"+name, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("%s: expected 200, got %d", name, w.Code)
	}
	cc := w.Header().Get("Cache-Control")
	if !strings.Contains(cc, "immutable") || !strings.Contains(cc, "max-age=31536000") {
		t.Fatalf("%s: hashed assets should be immutable, got %q", name, cc)
	}
}
