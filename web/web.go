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

	// Serve static assets under /workspace and /console
	r.StaticFS("/workspace", httpFS)
	r.StaticFS("/console", httpFS)

	// Backward compatibility: redirect /ui to /workspace
	r.GET("/ui", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/workspace/")
	})
	r.GET("/ui/*filepath", func(c *gin.Context) {
		p := c.Param("filepath")
		c.Redirect(http.StatusMovedPermanently, "/workspace"+p)
	})

	// Redirect root / to /workspace/
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/workspace/")
	})
}
