package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/nativestate"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func registerGeminiLive(r *gin.Engine, h *Handler) {
	for _, version := range []string{"v1alpha", "v1beta"} {
		r.GET("/ws/google.ai.generativelanguage."+version+".GenerativeService.BidiGenerateContent", middleware.AuthMiddleware(), middleware.RateLimitMiddleware(), h.HandleGeminiLive)
	}
}

func (h *Handler) HandleGeminiLive(c *gin.Context) {
	upgrader := websocket.Upgrader{HandshakeTimeout: 10 * time.Second, CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		u, e := url.Parse(origin)
		return origin == "" || e == nil && u.Host == r.Host
	}}
	client, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer client.Close()
	client.SetReadLimit(16 << 20)
	_ = client.SetReadDeadline(time.Now().Add(10 * time.Second))
	kind, setup, err := client.ReadMessage()
	if err != nil {
		return
	}
	_ = client.SetReadDeadline(time.Time{})
	name, identity, err := h.validateLiveSetup(c, setup)
	if err != nil {
		closeGoogleLive(client, err)
		return
	}
	setup, meter, err := prepareLiveQuota(c, name, setup)
	if err != nil {
		closeGoogleLive(client, err)
		return
	}
	ch, name, err := h.selectGoogleChannel(c, name, identity)
	if err != nil {
		closeGoogleLive(client, err)
		return
	}
	ctx, cancel, err := h.dispatcher.BeginNativeSession(c.Request.Context(), ch, name)
	if err != nil {
		closeGoogleLive(client, err)
		return
	}
	defer cancel()
	setup, err = rewriteLiveSetup(setup, provider.GeminiModelResource(ch, name))
	if err != nil {
		closeGoogleLive(client, err)
		return
	}
	version := "v1beta"
	if strings.Contains(c.Request.URL.Path, "v1alpha") {
		version = "v1alpha"
	}
	target := provider.GeminiLiveURL(ch, version)
	upstream, err := h.dialGoogleLive(ctx, ch, name, target)
	if err != nil {
		closeGoogleLive(client, err)
		return
	}
	defer upstream.Close()
	upstream.SetReadLimit(16 << 20)
	if err = upstream.WriteMessage(kind, setup); err != nil {
		closeGoogleLive(client, err)
		return
	}
	h.bridgeGoogleLive(c, ctx, client, upstream, ch, name, meter)
}

func (h *Handler) validateLiveSetup(c *gin.Context, wire []byte) (string, string, error) {
	var message struct {
		Setup struct {
			Model  string `json:"model"`
			Resume struct {
				Handle string `json:"handle"`
			} `json:"sessionResumption"`
		} `json:"setup"`
	}
	if json.Unmarshal(provider.CanonicalGeminiJSON(wire), &message) != nil || message.Setup.Model == "" {
		return "", "", fmt.Errorf("first Live message must contain setup.model")
	}
	name := strings.TrimPrefix(message.Setup.Model, "models/")
	if !h.googleModelAllowed(c, name) {
		return "", "", fmt.Errorf("live model is not allowed")
	}
	// Once upgraded, policy errors use a WebSocket close instead of HTTP JSON.
	if billing.GlobalEngine != nil {
		group := getKeyGroup(c)
		for _, candidate := range currencyModelsForBilling(name, group) {
			if err := storage.ValidatePriceCurrency(billing.GlobalEngine.GetPriceWithGroup(candidate, group)); err != nil {
				return "", "", err
			}
		}
	}
	identity := ""
	if handle := message.Setup.Resume.Handle; handle != "" {
		state, err := nativestate.Load(c.Request.Context(), "gemini-live", handle, callerOwner(c))
		if err != nil || state.Model != name {
			return "", "", fmt.Errorf("live resume handle is not available for this caller or model")
		}
		identity = state.Identity
	}
	native := &model.NativeRequest{Body: wire, PinnedIdentity: identity}
	if err := bindNativeState(c, native); err != nil {
		return "", "", err
	}
	return name, native.PinnedIdentity, nil
}

func rewriteLiveSetup(wire []byte, name string) ([]byte, error) {
	var outer map[string]json.RawMessage
	var setup map[string]json.RawMessage
	if err := json.Unmarshal(wire, &outer); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(outer["setup"], &setup); err != nil {
		return nil, err
	}
	setup["model"], _ = json.Marshal(name)
	outer["setup"], _ = json.Marshal(setup)
	return json.Marshal(outer)
}

func closeGoogleLive(conn *websocket.Conn, err error) {
	_ = conn.WriteJSON(gin.H{"error": gin.H{"message": sanitizeErrorMessage(err), "status": "UNAVAILABLE"}})
	_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "Live session ended"), time.Now().Add(time.Second))
}

func (h *Handler) dialGoogleLive(ctx context.Context, ch *model.ChannelConfig, name, target string) (*websocket.Conn, error) {
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err := provider.ConfigureGoogleRequest(ctx, request, ch); err != nil {
		return nil, err
	}
	dialer := websocket.Dialer{Proxy: http.ProxyFromEnvironment, HandshakeTimeout: 10 * time.Second}
	upstream, response, err := dialer.DialContext(ctx, target, request.Header)
	if err != nil {
		if response != nil {
			defer response.Body.Close()
			err = provider.NewUpstreamError(ch.Name, response.StatusCode, nil, response.Header)
		}
		h.dispatcher.ObserveNativeSession(ctx, ch, name, err)
	}
	return upstream, err
}
