package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/stretchr/testify/require"
)

// A fresh-install acceptance journey, using the public/admin HTTP routes for
// every business operation and a controlled upstream for reproducible charges.
func TestOperationalClosureAccountToBillingAndRevocation(t *testing.T) {
	app := newClosureApplication(t)
	var upstreamCalls atomic.Int64
	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer failed.Close()
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Model != "remote-chat" {
			w.WriteHeader(400)
			return
		}
		upstreamCalls.Add(1)
		if body.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"id\":\"closure-chat\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"closed-loop-answer\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":1000,\"completion_tokens\":500,\"total_tokens\":1500}}\n\ndata: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"closure-chat","object":"chat.completion","model":"remote-chat","choices":[{"index":0,"message":{"role":"assistant","content":"closed-loop-answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1000,"completion_tokens":500,"total_tokens":1500}}`)
	}))
	defer healthy.Close()
	primary := app.createChannel("closure-primary", failed.URL, 1)
	backup := app.createChannel("closure-backup", healthy.URL, 2)
	app.call("POST", "/api/v1/admin/models/routes", app.admin, fmt.Sprintf(`{"model":"org/closure-chat","provider_updates":[{"channel_id":%d,"priority":1,"weight":10,"mapped_model":"remote-chat"},{"channel_id":%d,"priority":2,"weight":10,"mapped_model":"remote-chat"}],"prompt_price":2,"completion_price":4,"currency":"CNY","off_peak_enabled":false}`, primary, backup), 200)

	created := app.call("POST", "/api/v1/admin/users", app.admin, `{"username":"closure-customer","password":"customer-password","balance":0}`, 200)
	credential := closureData(t, created)["api_key"].(string)
	require.NotEmpty(t, credential)
	login := app.call("POST", "/api/v1/auth/login", "", `{"username":"CLOSURE-CUSTOMER","password":"customer-password"}`, 200)
	userToken := closureData(t, login)["token"].(string)
	chat := `{"model":"org/closure-chat","messages":[{"role":"user","content":"hello"}],"max_tokens":1000}`
	app.call("POST", "/v1/chat/completions", credential, chat, 402)
	require.Zero(t, upstreamCalls.Load(), "empty wallet must reject before reaching the upstream")
	app.call("GET", "/api/v1/admin/channels", userToken, "", 403)
	keyList := app.call("GET", "/api/v1/user/keys", userToken, "", 200)
	require.NotContains(t, keyList.Body.String(), credential)
	var keys struct {
		Data []struct {
			ID    int64  `json:"id"`
			KeyID string `json:"key_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(keyList.Body.Bytes(), &keys))
	require.Len(t, keys.Data, 1)
	require.NotEmpty(t, keys.Data[0].KeyID)

	cards := app.call("POST", "/api/v1/admin/redemptions/generate", app.admin, `{"count":1,"amount":10,"name":"Closure credit"}`, 200)
	var cardEnvelope struct {
		Data []string `json:"data"`
	}
	require.NoError(t, json.Unmarshal(cards.Body.Bytes(), &cardEnvelope))
	redeem := fmt.Sprintf(`{"code":%q}`, cardEnvelope.Data[0])
	app.call("POST", "/api/v1/user/wallet/redeem", userToken, redeem, 200)
	app.call("POST", "/api/v1/user/wallet/redeem", userToken, redeem, 400)
	app.call("GET", "/v1/models", credential, "", 200)

	for _, body := range []string{chat, strings.TrimSuffix(chat, "}") + `,"stream":true}`} {
		w := app.call("POST", "/v1/chat/completions", credential, body, 200)
		require.Contains(t, w.Body.String(), "closed-loop-answer")
		require.NotEmpty(t, w.Header().Get("X-Airoute-Trace-Id"))
	}
	require.Equal(t, int64(2), upstreamCalls.Load())
	require.Eventually(t, func() bool {
		rows, err := app.repo.ListUsageLogsWithFilter(storage.LogFilter{Model: "org/closure-chat", Limit: 10})
		return err == nil && len(rows) == 2
	}, 3*time.Second, 10*time.Millisecond)
	logs := app.call("GET", fmt.Sprintf("/api/v1/admin/logs?key_id=%d&model=org%%2Fclosure-chat", keys.Data[0].ID), userToken, "", 200)
	var logEnvelope struct {
		Data []storage.UsageLogRecord `json:"data"`
	}
	require.NoError(t, json.Unmarshal(logs.Body.Bytes(), &logEnvelope))
	require.Len(t, logEnvelope.Data, 2)
	for _, record := range logEnvelope.Data {
		require.Equal(t, "closure-primary→closure-backup", record.Channel)
		require.InDelta(t, 0.004, record.Cost, 1e-9)
		require.NotNil(t, record.PricingSnapshot)
	}
	wallet := app.call("GET", "/api/v1/user/wallet", userToken, "", 200)
	require.InDelta(t, 9.992, closureData(t, wallet)["balance"], 1e-9)

	keyPath := fmt.Sprintf("/api/v1/admin/keys/%d", keys.Data[0].ID)
	app.call("PUT", keyPath, app.admin, `{"status":"disabled"}`, 200)
	app.call("PUT", keyPath, userToken, `{"status":"active"}`, 403)
	app.call("POST", "/v1/chat/completions", credential, chat, 401)
	app.call("DELETE", "/api/v1/admin/users/closure-customer", app.admin, "", 200)
	app.call("GET", "/api/v1/auth/me", userToken, "", 401)
	history := app.call("GET", "/api/v1/admin/logs?model="+url.QueryEscape("org/closure-chat"), app.admin, "", 200)
	require.Contains(t, history.Body.String(), "closure-backup", "closing the account must preserve historical charges")
	app.call("DELETE", "/api/v1/admin/models/routes?model="+url.QueryEscape("org/closure-chat"), app.admin, "", 200)
	app.call("DELETE", fmt.Sprintf("/api/v1/admin/channels/%d", backup), app.admin, "", 200)
}
