package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/nativestate"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

func bindNativeState(c *gin.Context, native *model.NativeRequest) error {
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(native.Body)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	return walkNativeReferences(c.Request.Context(), callerOwner(c), value, &native.PinnedIdentity)
}

func walkNativeReferences(ctx context.Context, owner string, value any, identity *string) error {
	switch object := value.(type) {
	case []any:
		for _, v := range object {
			if err := walkNativeReferences(ctx, owner, v, identity); err != nil {
				return err
			}
		}
	case map[string]any:
		for key, v := range object {
			text, _ := v.(string)
			namespace, id := "", ""
			switch key {
			case "id", "tool_call_id", "call_id", "tool_use_id":
				if strings.HasPrefix(text, model.GeminiCallPrefix) {
					id, _, _ = strings.Cut(strings.TrimPrefix(text, model.GeminiCallPrefix), "_")
					namespace = provider.GeminiHistoryNamespace
				}
			case "state_ref":
				namespace, id = provider.GeminiHistoryNamespace, text
			case "cachedContent", "cached_content":
				namespace, id = "gemini-cache", text
			case "previous_interaction_id":
				namespace, id = "gemini-interaction", text
			case "fileUri", "file_uri", "fileName", "file_name":
				id = geminiFileName(text)
				if id != "" {
					namespace = "gemini-file"
				}
			}
			if namespace != "" && id != "" {
				state, err := nativestate.Load(ctx, namespace, id, owner)
				if err != nil || owner == "" {
					return fmt.Errorf("native resource or signature is not available for this caller")
				}
				if *identity != "" && *identity != state.Identity {
					return fmt.Errorf("native resources belong to different upstream credentials")
				}
				*identity = state.Identity
			}
			if key != "args" && key != "arguments" && key != "parameters" && key != "response" && key != "responseJsonSchema" {
				if err := walkNativeReferences(ctx, owner, v, identity); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func geminiFileName(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	for _, collection := range []string{"files", "generatedFiles"} {
		path := strings.TrimPrefix(u.Path, "/")
		if _, tail, ok := strings.Cut(u.Path, "/"+collection+"/"); ok {
			if u.Hostname() != "generativelanguage.googleapis.com" && !strings.HasPrefix(u.Path, "/v1beta/") && !strings.HasPrefix(u.Path, "/download/v1beta/") {
				continue
			}
			path = collection + "/" + tail
		}
		if !strings.HasPrefix(path, collection+"/") {
			continue
		}
		id := strings.SplitN(strings.SplitN(strings.TrimPrefix(path, collection+"/"), "/", 2)[0], ":", 2)[0]
		if validGoogleID(id) {
			return collection + "/" + id
		}
	}
	return ""
}
