package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/nativestate"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

type googleResource struct {
	namespace, collection, id, path, model, action string
	body                                           []byte
	context                                        context.Context
	state                                          *storage.NativeState
	channel                                        *model.ChannelConfig
}

func registerGeminiResources(group *gin.RouterGroup, h *Handler) {
	for _, name := range []string{"files", "generatedFiles", "cachedContents", "batches", "operations", "interactions"} {
		group.GET("/"+name, h.HandleGeminiResource)
		group.POST("/"+name, h.HandleGeminiResource)
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete} {
			group.Handle(method, "/"+name+"/*resource", h.HandleGeminiResource)
		}
	}
}

func (h *Handler) HandleGeminiResource(c *gin.Context) {
	resource, err := h.parseGoogleResource(c)
	if err != nil {
		RespondGeminiError(c, 404, err.Error(), "NOT_FOUND")
		return
	}
	if resource.id == "" && c.Request.Method == http.MethodGet {
		h.listGoogleResources(c, resource)
		return
	}
	if strings.Contains(resource.path, "/interactions") {
		h.handleGeminiInteraction(c, resource)
		return
	}
	h.forwardGoogleResource(c, resource)
}

func (h *Handler) parseGoogleResource(c *gin.Context) (*googleResource, error) {
	if callerOwner(c) == "" {
		return nil, fmt.Errorf("authenticated resource owner is required")
	}
	path := strings.TrimPrefix(c.Request.URL.Path, "/v1beta/")
	if path == c.Request.URL.Path {
		path = strings.TrimPrefix(path, "/v1/")
	}
	collection, tail, _ := strings.Cut(path, "/")
	namespace := map[string]string{"files": "gemini-file", "generatedFiles": "gemini-file", "cachedContents": "gemini-cache", "batches": "gemini-batch", "operations": "gemini-batch", "interactions": "gemini-interaction"}[collection]
	if namespace == "" {
		return nil, fmt.Errorf("unknown native resource")
	}
	r := &googleResource{namespace: namespace, collection: collection, path: "/v1beta/" + path, model: strings.TrimPrefix(c.Query("model"), "models/")}
	if tail != "" {
		id := strings.SplitN(strings.SplitN(tail, "/", 2)[0], ":", 2)[0]
		if !validGoogleID(id) || !validGoogleResourceTail(collection, id, tail) {
			return nil, fmt.Errorf("invalid native resource identifier")
		}
		r.id = collection + "/" + id
		if collection == "interactions" {
			r.id = id
		}
		state, err := nativestate.Load(c.Request.Context(), namespace, r.id, callerOwner(c))
		if err != nil {
			return nil, fmt.Errorf("native resource not found")
		}
		r.state, r.model = state, state.Model
		if !h.googleModelAllowed(c, r.model) {
			return nil, fmt.Errorf("native resource not found")
		}
	}
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodDelete {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			return nil, err
		}
		r.body = body
		var payload struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(body, &payload)
		if r.model == "" {
			r.model = strings.TrimPrefix(payload.Model, "models/")
		}
	}
	return r, nil
}

func validGoogleID(id string) bool {
	if id == "" || id == "." || id == ".." {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("._-", c)) {
			return false
		}
	}
	return true
}

func validGoogleResourceTail(collection, id, tail string) bool {
	if tail == id {
		return true
	}
	switch collection {
	case "files", "generatedFiles":
		return tail == id+":download"
	case "batches", "operations":
		return tail == id+":cancel" || tail == id+":updateGenerateContentBatch" || tail == id+":updateEmbedContentBatch"
	case "interactions":
		return tail == id+"/cancel"
	}
	return false
}

func (h *Handler) googleModelAllowed(c *gin.Context, name string) bool {
	if name == "" {
		return true
	}
	return middleware.ValidateModelAllowed(c, name)
}

func (h *Handler) selectGoogleChannel(c *gin.Context, name, identity string) (*model.ChannelConfig, string, error) {
	models := []string{name}
	if name == "" {
		models = h.dispatcher.GetAllSupportedModels()
		sort.Strings(models)
	}
	for _, m := range models {
		if !h.googleModelAllowed(c, m) {
			continue
		}
		for _, ch := range h.dispatcher.GetChannelsForModelAndProtocolWithContext(c.Request.Context(), m, "") {
			if model.IsGeminiProvider(ch.Type) && (identity == "" || identity == model.UpstreamIdentity(ch)) {
				return ch, m, nil
			}
		}
	}
	return nil, "", fmt.Errorf("no eligible native Google upstream")
}

func saveGoogleResource(c *gin.Context, r *googleResource, id string, data []byte) error {
	expires := time.Now().Add(30 * 24 * time.Hour)
	var payload struct {
		Expiration string `json:"expirationTime"`
		Expire     string `json:"expireTime"`
	}
	_ = json.Unmarshal(data, &payload)
	for _, raw := range []string{payload.Expiration, payload.Expire} {
		if parsed, e := time.Parse(time.RFC3339Nano, raw); e == nil {
			expires = parsed
		}
	}
	var billing []byte
	if r.namespace == "gemini-batch" || r.namespace == "gemini-interaction" {
		if key := getAPIKeyConfig(c); key != nil {
			copy := *key
			if !storage.IsAPIKeyHash(copy.Key) {
				copy.Key = storage.HashAPIKey(copy.Key)
			}
			billing, _ = json.Marshal(copy)
			var snapshot map[string]json.RawMessage
			_ = json.Unmarshal(billing, &snapshot)
			snapshot["_airoute_estimated_prompt_tokens"], _ = json.Marshal(estimateTokens(len(r.body)))
			billing, _ = json.Marshal(snapshot)
		}
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 2*time.Second)
	defer cancel()
	return nativestate.Save(ctx, storage.NativeState{Namespace: r.namespace, ID: id, Owner: callerOwner(c),
		Identity: model.UpstreamIdentity(r.channel), Model: r.model, Data: data, Billing: billing, ExpiresAt: expires})
}
