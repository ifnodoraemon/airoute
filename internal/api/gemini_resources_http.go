package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/nativestate"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/router"
)

func (h *Handler) googleResourceCall(c *gin.Context, r *googleResource) (*router.UpstreamResponse, error) {
	identity := ""
	if r.state != nil {
		identity = r.state.Identity
	}
	native := &model.NativeRequest{Body: r.body, PinnedIdentity: identity}
	if len(r.body) > 0 && strings.Contains(c.GetHeader("Content-Type"), "application/json") {
		if err := bindNativeState(c, native); err != nil {
			return nil, err
		}
	}
	ch, name, err := h.selectGoogleChannel(c, r.model, native.PinnedIdentity)
	if err != nil {
		return nil, err
	}
	r.channel, r.model = ch, name
	q := c.Request.URL.Query()
	for _, key := range []string{"key", "api_key", "access_token", "model"} {
		q.Del(key)
	}
	path := r.path
	if r.action != "" {
		path = ""
	}
	if q.Encode() != "" && !strings.Contains(path, "?") {
		path += "?" + q.Encode()
	}
	body := rewriteGoogleResourceModel(r.body, ch.GetUpstreamModel(name))
	if r.action != "" {
		body = rewriteGoogleResourceModel(r.body, "models/"+ch.GetUpstreamModel(name))
	}
	if r.collection == "cachedContents" && len(body) > 0 {
		resourceName := "models/" + ch.GetUpstreamModel(name)
		if provider.IsVertexChannel(ch) {
			u, _ := url.Parse(provider.GeminiBaseURL(ch))
			resourceName = strings.TrimPrefix(strings.TrimPrefix(u.Path, "/v1/"), "/v1beta/") + "/models/" + ch.GetUpstreamModel(name)
		}
		body = provider.RewriteNativeModel(body, resourceName)
	}
	headers := make(map[string]string)
	for key, values := range c.Request.Header {
		if strings.HasPrefix(strings.ToLower(key), "x-goog-upload-") || key == "Range" || key == "If-Match" {
			headers[key] = strings.Join(values, ",")
		}
	}
	ctx := r.context
	if ctx == nil {
		ctx = c.Request.Context()
	}
	return h.dispatcher.DispatchHTTP(ctx, &router.UpstreamRequest{
		PinnedChannel: ch.Name, PinnedChannelID: ch.ID, PinnedBaseURL: ch.BaseURL, PinnedIdentity: model.UpstreamIdentity(ch),
		Model: name, Method: c.Request.Method, GeminiPath: path, GeminiAction: r.action, Body: body, ContentType: c.GetHeader("Content-Type"), Headers: headers, NoRetry: true, NoRewriteBody: true,
	})
}

func (h *Handler) forwardGoogleResource(c *gin.Context, r *googleResource) {
	response, err := h.googleResourceCall(c, r)
	if err != nil {
		status, _, _, _ := resolveUpstreamFailureStatus(err)
		setUpstreamRetryAfter(c, err)
		RespondGeminiError(c, status, sanitizeErrorMessage(err), geminiErrorStatus(status))
		return
	}
	defer response.Stream.Close()
	if strings.HasPrefix(r.path, "/download/") || c.Query("alt") == "media" {
		copyGoogleResponseHeaders(c, response.Headers)
		c.Status(response.StatusCode)
		_, _ = io.Copy(c.Writer, response.Stream)
		return
	}
	data, err := readGoogleJSON(response.Stream)
	if err != nil {
		RespondGeminiError(c, 502, "Failed to read native response", "UNAVAILABLE")
		return
	}
	if err = h.captureGoogleResource(c, r, response, data); err != nil {
		RespondGeminiError(c, 503, "Failed to retain native resource ownership", "UNAVAILABLE")
		return
	}
	h.recordGoogleBatchResult(c, r, data)
	copyGoogleResponseHeaders(c, response.Headers)
	if c.Request.Method == http.MethodDelete {
		_ = nativestate.Delete(c.Request.Context(), r.namespace, r.id, callerOwner(c))
	}
	c.Data(response.StatusCode, response.Headers.Get("Content-Type"), data)
}

func (h *Handler) captureGoogleResource(c *gin.Context, r *googleResource, response *router.UpstreamResponse, data []byte) error {
	if upload := response.Headers.Get("X-Goog-Upload-URL"); upload != "" {
		if err := h.saveGoogleUpload(c, r, upload); err != nil {
			return err
		}
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) != nil {
		return nil
	}
	if file, ok := object["file"]; ok {
		data = file
		_ = json.Unmarshal(file, &object)
		r.namespace, r.collection = "gemini-file", "files"
	}
	var id string
	_ = json.Unmarshal(object["name"], &id)
	if r.collection == "interactions" {
		_ = json.Unmarshal(object["id"], &id)
	}
	if id == "" {
		id = r.id
	}
	if id != "" {
		if err := saveGoogleResource(c, r, id, data); err != nil {
			return err
		}
	}
	if r.namespace == "gemini-batch" {
		for _, file := range googleBatchOutputFiles(data) {
			entry, _ := json.Marshal(map[string]string{"name": file})
			resource := &googleResource{namespace: "gemini-file", collection: "files", model: r.model, channel: r.channel}
			if err := saveGoogleResource(c, resource, file, entry); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *Handler) listGoogleResources(c *gin.Context, r *googleResource) {
	states, err := nativestate.List(c.Request.Context(), r.namespace, callerOwner(c))
	if err != nil {
		RespondGeminiError(c, 503, "Native resource storage is unavailable", "UNAVAILABLE")
		return
	}
	sort.Slice(states, func(i, j int) bool { return states[i].ID < states[j].ID })
	size, _ := strconv.Atoi(c.DefaultQuery("pageSize", "100"))
	if size <= 0 || size > 1000 {
		size = 100
	}
	cursorBytes, _ := base64.RawURLEncoding.DecodeString(c.Query("pageToken"))
	cursor := string(cursorBytes)
	var items []json.RawMessage
	next := ""
	for _, state := range states {
		if state.ID <= cursor || !h.googleModelAllowed(c, state.Model) {
			continue
		}
		if r.collection != "interactions" && !strings.HasPrefix(state.ID, r.collection+"/") {
			continue
		}
		if len(items) == size {
			next = base64.RawURLEncoding.EncodeToString([]byte(statesID(items[len(items)-1])))
			break
		}
		items = append(items, state.Data)
	}
	if items == nil {
		items = []json.RawMessage{}
	}
	body := gin.H{r.collection: items}
	if next != "" {
		body["nextPageToken"] = next
	}
	c.JSON(200, body)
}

func statesID(raw []byte) string {
	var v struct {
		Name string `json:"name"`
		ID   string `json:"id"`
	}
	_ = json.Unmarshal(raw, &v)
	if v.Name != "" {
		return v.Name
	}
	return v.ID
}

func readGoogleJSON(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, (64<<20)+1))
	if err == nil && len(data) > 64<<20 {
		return nil, fmt.Errorf("native JSON response exceeds 64 MiB")
	}
	return data, err
}

func copyGoogleResponseHeaders(c *gin.Context, headers http.Header) {
	for _, key := range []string{"Content-Type", "Content-Disposition", "Content-Range", "Accept-Ranges", "Retry-After", "X-Goog-Upload-Status", "X-Goog-Upload-Size-Received", "X-Goog-Upload-Chunk-Granularity"} {
		if value := headers.Get(key); value != "" {
			c.Header(key, value)
		}
	}
}

func rewriteGoogleResourceModel(body []byte, name string) []byte {
	var object map[string]json.RawMessage
	if json.Unmarshal(body, &object) != nil {
		return body
	}
	if _, ok := object["model"]; ok {
		object["model"], _ = json.Marshal(name)
	}
	for _, key := range []string{"batch", "request", "generateContentRequest", "inputConfig"} {
		if raw, ok := object[key]; ok {
			object[key] = rewriteGoogleResourceModel(raw, name)
		}
	}
	if raw, ok := object["requests"]; ok {
		var list []json.RawMessage
		if json.Unmarshal(raw, &list) == nil {
			for i := range list {
				list[i] = rewriteGoogleResourceModel(list[i], name)
			}
			object["requests"], _ = json.Marshal(list)
		} else {
			object["requests"] = rewriteGoogleResourceModel(raw, name)
		}
	}
	out, _ := json.Marshal(object)
	return out
}
