package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed dist/*
var distFS embed.FS

// RegisterStaticRoutes mounts the embedded SPA on the Gin engine.
func RegisterStaticRoutes(r *gin.Engine) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return
	}

	httpFS := http.FS(sub)

	// Serve static assets under /app
	r.StaticFS("/app", httpFS)

	// Serve assets subdirectory at /assets so relative ./assets works from root /
	if assetsSub, err := fs.Sub(distFS, "dist/assets"); err == nil {
		r.StaticFS("/assets", http.FS(assetsSub))
	}

	// Serve index.html directly on root / for zero-friction access
	indexHTML, err := fs.ReadFile(distFS, "dist/index.html")
	if err == nil {
		r.GET("/", func(c *gin.Context) {
			c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
		})

		// Serve favicons directly from root /favicon.ico and /favicon.svg
		if favIco, err := fs.ReadFile(distFS, "dist/favicon.ico"); err == nil {
			r.GET("/favicon.ico", func(c *gin.Context) {
				c.Data(http.StatusOK, "image/x-icon", favIco)
			})
		}
		if favSvg, err := fs.ReadFile(distFS, "dist/favicon.svg"); err == nil {
			r.GET("/favicon.svg", func(c *gin.Context) {
				c.Data(http.StatusOK, "image/svg+xml", favSvg)
			})
		}

		// SPA HTML5 History API fallback: return index.html for client-side routing under /app on page refresh
		r.NoRoute(func(c *gin.Context) {
			path := c.Request.URL.Path
			if path == "/app" || strings.HasPrefix(path, "/app/") {
				c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
				return
			}
			c.JSON(http.StatusNotFound, gin.H{"error": "Endpoint not found"})
		})
	} else {
		r.GET("/", func(c *gin.Context) {
			c.Redirect(http.StatusFound, "/app/")
		})
	}
}
