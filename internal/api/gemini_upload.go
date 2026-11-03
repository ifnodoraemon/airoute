package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/nativestate"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func (h *Handler) HandleGeminiUpload(c *gin.Context) {
	if callerOwner(c) == "" {
		RespondGeminiError(c, 401, "Authenticated owner required", "UNAUTHENTICATED")
		return
	}
	r := &googleResource{namespace: "gemini-file", collection: "files", path: "/upload/v1beta/files", model: c.Query("model")}
	if id := c.Param("upload"); id != "" {
		state, err := nativestate.Load(c.Request.Context(), "gemini-upload", id, callerOwner(c))
		if err != nil {
			RespondGeminiError(c, 404, "Upload session not found", "NOT_FOUND")
			return
		}
		var saved struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(state.Data, &saved) != nil {
			RespondGeminiError(c, 503, "Invalid stored upload session", "UNAVAILABLE")
			return
		}
		r.path, r.model, r.state = saved.Path, state.Model, state
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		RespondGeminiError(c, 400, "Failed to read upload body", "INVALID_ARGUMENT")
		return
	}
	r.body = body
	h.forwardGoogleResource(c, r)
}

func (h *Handler) saveGoogleUpload(c *gin.Context, r *googleResource, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	base, err := url.Parse(provider.GeminiBaseURL(r.channel))
	if err != nil {
		return err
	}
	if u.Host == "" {
		u = base.ResolveReference(u)
	}
	if u.Scheme != base.Scheme || u.Host != base.Host || u.User != nil {
		return fmt.Errorf("upload URL changed upstream origin")
	}
	prefix := strings.TrimSuffix(strings.TrimSuffix(strings.TrimRight(base.Path, "/"), "/v1beta"), "/v1")
	path := strings.TrimPrefix(u.Path, prefix)
	if !strings.HasPrefix(path, "/upload/") && !strings.HasPrefix(path, "/resumable/") {
		return fmt.Errorf("invalid upload continuation path")
	}
	query := u.Query()
	for _, key := range []string{"key", "api_key", "access_token"} {
		query.Del(key)
	}
	if query.Encode() != "" {
		path += "?" + query.Encode()
	}
	id := nativestate.NewID()
	data, _ := json.Marshal(map[string]string{"path": path})
	err = nativestate.Save(c.Request.Context(), storage.NativeState{Namespace: "gemini-upload", ID: id, Owner: callerOwner(c),
		Identity: model.UpstreamIdentity(r.channel), Model: r.model, Data: data, ExpiresAt: time.Now().Add(24 * time.Hour)})
	if err != nil {
		return err
	}
	origin := strings.TrimRight(os.Getenv("PUBLIC_URL"), "/")
	if origin == "" {
		scheme := "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
		origin = scheme + "://" + c.Request.Host
	}
	c.Header("X-Goog-Upload-URL", origin+"/upload/v1beta/sessions/"+id)
	return nil
}

func registerGeminiUpload(r *gin.Engine, h *Handler, auth, rate gin.HandlerFunc) {
	for _, prefix := range []string{"/upload/v1beta", "/resumable/upload/v1beta"} {
		group := r.Group(prefix, auth, rate)
		group.POST("/files", h.HandleGeminiUpload)
		group.POST("/sessions/:upload", h.HandleGeminiUpload)
		group.PUT("/sessions/:upload", h.HandleGeminiUpload)
	}
	group := r.Group("/download/v1beta", auth, rate)
	for _, collection := range []string{"files", "generatedFiles"} {
		group.GET("/"+collection+"/*resource", func(c *gin.Context) {
			c.Request.URL.Path = strings.TrimPrefix(c.Request.URL.Path, "/download")
			r, err := h.parseGoogleResource(c)
			if err != nil {
				RespondGeminiError(c, http.StatusNotFound, "File not found", "NOT_FOUND")
				return
			}
			r.path = "/download" + r.path
			h.forwardGoogleResource(c, r)
		})
	}
}
