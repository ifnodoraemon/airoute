package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// The console is embedded so the gateway ships as one self-contained binary.
//
// dist/ is a build artifact and is deliberately not tracked by git, so it has to
// exist before this package compiles. Run `make build-web` (or
// `cd web && npm ci && npm run build`) first. Without it the compiler stops here
// with "pattern dist/*: no matching files found", which does not say what to do.
//
//go:embed dist/*
var distFS embed.FS

// assetCacheControl is safe because every file under /assets carries a
// content hash in its name: a new build produces new names, so a cached copy
// can never be stale.
const assetCacheControl = "public, max-age=31536000, immutable"

// shellCacheControl forces revalidation of the SPA shell. The shell is the
// only file that maps "what the user requested" to "which hashed bundle to
// load"; if a browser reuses a cached copy after a deploy it will reference
// asset names that no longer exist, the module graph fails to load, React never
// mounts and the console appears to be logged out / stuck on the login screen
// until the cache is manually cleared.
const shellCacheControl = "no-cache, must-revalidate"

// staticCacheHeaders applies the correct cache policy per path prefix.
func staticCacheHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/assets/") {
			c.Header("Cache-Control", assetCacheControl)
		} else {
			c.Header("Cache-Control", shellCacheControl)
		}
		c.Next()
	}
}

// RegisterStaticRoutes mounts the embedded SPA on the Gin engine.
func RegisterStaticRoutes(r *gin.Engine) {
	// All static routes share the cache policy middleware.
	static := r.Group("", staticCacheHeaders())

	indexHTML, readErr := fs.ReadFile(distFS, "dist/index.html")
	if readErr != nil {
		r.GET("/", func(c *gin.Context) {
			c.Redirect(http.StatusFound, "/app/")
		})
		return
	}

	// A strong ETag lets the mandatory revalidation above resolve to a cheap 304
	// instead of re-downloading the shell on every navigation.
	sum := sha256.Sum256(indexHTML)
	indexETag := `"` + hex.EncodeToString(sum[:16]) + `"`

	serveIndex := func(c *gin.Context) {
		c.Header("Cache-Control", shellCacheControl)
		c.Header("ETag", indexETag)
		if match := c.GetHeader("If-None-Match"); match != "" && strings.Contains(match, indexETag) {
			c.Status(http.StatusNotModified)
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	}

	// Serve index.html directly on root / for zero-friction access.
	//
	// This is the PUBLIC entrance, not the control plane: the SPA itself resolves a
	// bare "/" to the landing/login page (web/src/lib/consoleRoute.js), and admin
	// data is only reachable through the guarded /api/v1/admin API. Do not add "/"
	// to middleware.isControlPlanePath - that would lock the public login screen out
	// behind the private-network guard.
	r.GET("/", serveIndex)

	// The console entry point MUST go through serveIndex (not StaticFS) so it
	// gets the same no-cache + ETag treatment as "/". StaticFS serves a file
	// straight off the embedded FS without any revalidation policy, and it also
	// registers a catch-all route that conflicts with the explicit /app route.
	r.GET("/app", func(c *gin.Context) { c.Redirect(http.StatusMovedPermanently, "/app/") })
	r.GET("/app/", serveIndex)

	// Hashed bundles. Aliased under both /assets (referenced by index.html) and
	// /app/assets (relative references) so either form resolves.
	if assetsSub, err := fs.Sub(distFS, "dist/assets"); err == nil {
		static.StaticFS("/assets", http.FS(assetsSub))
		static.StaticFS("/app/assets", http.FS(assetsSub))
	}

	// Favicons: tiny and stable, but still cheap to revalidate.
	if favIco, err := fs.ReadFile(distFS, "dist/favicon.ico"); err == nil {
		static.GET("/favicon.ico", func(c *gin.Context) {
			c.Data(http.StatusOK, "image/x-icon", favIco)
		})
	}
	if favSvg, err := fs.ReadFile(distFS, "dist/favicon.svg"); err == nil {
		static.GET("/favicon.svg", func(c *gin.Context) {
			c.Data(http.StatusOK, "image/svg+xml", favSvg)
		})
	}

	// SPA HTML5 History API fallback: return index.html for client-side routing
	// under /app on page refresh.
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/app" || strings.HasPrefix(path, "/app/") {
			serveIndex(c)
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "Endpoint not found"})
	})
}
