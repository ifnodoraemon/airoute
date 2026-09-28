package web

import (
	"embed"
	"io/fs"
	"net/http"

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

	// Serve static assets under /app, /workspace, and /console
	r.StaticFS("/app", httpFS)
	r.StaticFS("/workspace", httpFS)
	r.StaticFS("/console", httpFS)

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
	} else {
		r.GET("/", func(c *gin.Context) {
			c.Redirect(http.StatusFound, "/app/")
		})
	}

	// Backward compatibility: redirect /ui to /app/
	r.GET("/ui", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/app/")
	})
	r.GET("/ui/*filepath", func(c *gin.Context) {
		p := c.Param("filepath")
		c.Redirect(http.StatusMovedPermanently, "/app"+p)
	})
}
