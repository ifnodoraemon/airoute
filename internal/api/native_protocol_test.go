package api

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
)

func TestLegacyCompletionsIsNotRegisteredAtEitherPrefix(t *testing.T) {
	d := router.NewDispatcher(nil)
	h := NewHandler(d)
	r := gin.New()
	for _, prefix := range []string{"", "/v1"} {
		registerDataPlaneRoutes(r.Group(prefix), h, nil)
	}
	for _, path := range []string{"/completions", "/v1/completions"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"m","prompt":"hi"}`)))
		if w.Code != http.StatusNotFound {
			t.Fatalf("retired endpoint %s returned %d", path, w.Code)
		}
	}
	c, w := newTestContext(http.MethodGet, "/v1")
	h.HandleV1Info(c)
	if strings.Contains(w.Body.String(), `"completions"`) || strings.Contains(w.Body.String(), "/v1/completions") {
		t.Fatal("discovery still advertises the retired endpoint")
	}
}

func TestNativeResponsesContinuationKeepsOwnerAndOriginalUpstream(t *testing.T) {
	newResponsesContextRepo(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Error("native request was translated")
			w.WriteHeader(500)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if calls.Add(1) == 1 {
			if !strings.Contains(string(body), `"web_search"`) || !strings.Contains(string(body), `"reasoning"`) {
				t.Error("native request lost tools/reasoning")
			}
		} else if !strings.Contains(string(body), `"previous_response_id":"resp-native-1"`) {
			t.Error("native continuation id was dropped")
		}
		fmt.Fprint(w, `{"id":"resp-native-1","object":"response","model":"m","status":"completed","output":[],"usage":{"input_tokens":12,"output_tokens":3},"opaque_vendor_field":9007199254740993}`)
	}))
	defer server.Close()
	d := router.NewDispatcher([]model.ChannelConfig{{Name: "native", Type: model.ProviderOpenAI, BaseURL: server.URL, APIKey: "upstream", Models: []string{"m"}, Protocols: []string{"responses"}}})
	h := NewHandler(d)
	call := func(owner int64, body string) *httptest.ResponseRecorder {
		c, w := newTestContext(http.MethodPost, "/v1/responses")
		c.Request.Body = io.NopCloser(strings.NewReader(body))
		responsesContextCaller(c, owner, fmt.Sprint(owner))
		h.HandleResponses(c)
		return w
	}
	first := call(101, `{"model":"m","input":"hi","tools":[{"type":"web_search"}],"reasoning":{"effort":"high"}}`)
	if first.Code != 200 || !strings.Contains(first.Body.String(), `"opaque_vendor_field":9007199254740993`) {
		t.Fatalf("native reply not preserved: %d %s", first.Code, first.Body.String())
	}
	foreign := call(202, `{"model":"m","input":"continue","previous_response_id":"resp-native-1"}`)
	if foreign.Code != 400 || calls.Load() != 1 {
		t.Fatal("another caller replayed a native response id")
	}
	second := call(101, `{"model":"m","input":"continue","previous_response_id":"resp-native-1"}`)
	if second.Code != 200 || calls.Load() != 2 {
		t.Fatalf("native continuation failed: %d %s", second.Code, second.Body.String())
	}
	d.UpdateChannels([]model.ChannelConfig{{Name: "replacement", Type: model.ProviderOpenAI, BaseURL: server.URL, APIKey: "different", Models: []string{"m"}, Protocols: []string{"responses"}}})
	third := call(101, `{"model":"m","input":"continue","previous_response_id":"resp-native-1"}`)
	if third.Code == 200 || calls.Load() != 2 {
		t.Fatal("continuation moved to another credential")
	}
}

func TestNativeResponsesSSEKeepsEventsAndBillsActualUsage(t *testing.T) {
	repo := edgeBillingRepo(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Error("stream translated")
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-stream\",\"object\":\"response\"}}\n\nevent: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\",\"vendor_extra\":9007199254740993}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-stream\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":40,\"output_tokens\":9,\"input_tokens_details\":{\"cached_tokens\":30}}}}\n\n")
	}))
	defer server.Close()
	h := newResponsesContextHandler(t, server.URL)
	h.dispatcher.UpdateChannels([]model.ChannelConfig{{Name: "native", Type: model.ProviderOpenAI, BaseURL: server.URL, Models: []string{"m"}, Protocols: []string{"responses"}}})
	c, w := newTestContext(http.MethodPost, "/v1/responses")
	c.Request.Body = io.NopCloser(strings.NewReader(`{"model":"m","input":"hi","stream":true}`))
	responsesContextCaller(c, 101, "key")
	h.HandleResponses(c)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"vendor_extra":9007199254740993`) || strings.Count(w.Body.String(), "event: response.created") != 1 {
		t.Fatalf("native SSE was rebuilt: %d %s", w.Code, w.Body.String())
	}
	rows := edgeUsageRows(t, repo, 1)
	if rows[0].PromptTokens != 40 || rows[0].CompletionTokens != 9 || rows[0].CachedTokens != 30 {
		t.Fatalf("native usage lost: %+v", rows[0])
	}
}
