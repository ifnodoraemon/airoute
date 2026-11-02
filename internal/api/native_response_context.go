package api

import (
	"context"
	"encoding/json"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

const contextNativeResponse = "native_response_context"

type nativeContinuation struct {
	Native   bool   `json:"native"`
	Identity string `json:"identity"`
	Tokens   int    `json:"tokens"`
}

// A native continuation remains at the upstream. Record ownership before the
// caller sees its id so another tenant cannot replay a response on our shared
// upstream credential by guessing previous_response_id.
func persistNativeResponseContext(c *gin.Context, id, identity string, tokens int) {
	owner := callerOwner(c)
	repo := storage.GetGlobalRepository()
	if id == "" || owner == "" || repo == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), responseContextStoreTimeout)
	defer cancel()
	marker, _ := json.Marshal(nativeContinuation{Native: true, Identity: identity, Tokens: max(0, tokens)})
	if err := repo.SaveResponseContext(ctx, id, owner, marker); err != nil {
		telemetry.ScopedLogger("system").Warn("failed to save native response ownership", "response_id", id, "error", err)
	}
}
