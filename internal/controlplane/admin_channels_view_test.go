package controlplane

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// The upstream provider credential must never leave the process.
//
// channels.api_key holds the real key (unlike api_keys.key, which holds a digest),
// so serializing the stored row shipped a long-lived secret to the browser on
// every list/create/update. These tests pin the wire contract: the response
// carries api_key_configured plus a mask, and no handler can regress to the row.
const channelSecret = "upstream-provider-secret-value"

func newFakeChannelHandler() (*AdminHandler, *fakeChannelStore) {
	store := newFakeChannelStore()
	return &AdminHandler{channels: NewChannelService(store, nil, nil)}, store
}

// assertNoChannelSecret fails when the response body contains the stored upstream
// credential or an api_key field carrying it.
func assertNoChannelSecret(t *testing.T, where, body string) {
	t.Helper()
	if strings.Contains(body, channelSecret) {
		t.Fatalf("%s leaked the upstream credential: %s", where, body)
	}
	var payload any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("%s response is not JSON: %s", where, body)
	}
	if raw, ok := findJSONKey(payload, "api_key"); ok {
		t.Fatalf("%s response still exposes an api_key field (%v): %s", where, raw, body)
	}
}

// findJSONKey walks decoded JSON looking for a key with the given name.
func findJSONKey(value any, name string) (any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		if found, ok := typed[name]; ok {
			return found, true
		}
		for _, child := range typed {
			if found, ok := findJSONKey(child, name); ok {
				return found, true
			}
		}
	case []any:
		for _, child := range typed {
			if found, ok := findJSONKey(child, name); ok {
				return found, ok
			}
		}
	}
	return nil, false
}

func TestChannelResponsesNeverCarryTheUpstreamCredential(t *testing.T) {
	handler, store := newFakeChannelHandler()
	store.seed(&storage.ChannelRecord{
		Name: "chan-a", Type: "openai", BaseURL: "http://a.invalid", APIKey: channelSecret,
		Models: []string{"m1"}, Priority: 3, Weight: 7, TimeoutSeconds: 60, Status: "active",
	})

	t.Run("list", func(t *testing.T) {
		w := invokeChannel(t, handler.ListChannels, http.MethodGet, "", "")
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", w.Code, w.Body.String())
		}
		assertNoChannelSecret(t, "list", w.Body.String())
		envelope := decodeEnvelope(t, w)
		data, _ := envelope["data"].([]any)
		if len(data) != 1 {
			t.Fatalf("data = %v, want one channel", envelope["data"])
		}
		channel, _ := data[0].(map[string]any)
		if channel["api_key_configured"] != true {
			t.Errorf("api_key_configured = %v, want true", channel["api_key_configured"])
		}
		if masked, _ := channel["api_key_masked"].(string); masked == "" || strings.Contains(masked, channelSecret) {
			t.Errorf("api_key_masked = %q, want a mask that hides the credential", masked)
		}
	})

	t.Run("create", func(t *testing.T) {
		body := `{"name":"chan-b","type":"openai","base_url":"http://b.invalid","api_key":"` + channelSecret + `","models":["m2"],"priority":2,"weight":5}`
		w := invokeChannel(t, handler.CreateChannel, http.MethodPost, body, "")
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", w.Code, w.Body.String())
		}
		assertNoChannelSecret(t, "create", w.Body.String())
		envelope := decodeEnvelope(t, w)
		created, _ := envelope["data"].(map[string]any)
		if created["api_key_configured"] != true {
			t.Errorf("api_key_configured = %v, want true", created["api_key_configured"])
		}
	})

	t.Run("update", func(t *testing.T) {
		w := invokeChannel(t, handler.UpdateChannel, http.MethodPut, `{"name":"renamed"}`, "1")
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", w.Code, w.Body.String())
		}
		assertNoChannelSecret(t, "update", w.Body.String())
		envelope := decodeEnvelope(t, w)
		updated, _ := envelope["data"].(map[string]any)
		if updated["api_key_configured"] != true {
			t.Errorf("api_key_configured = %v, want true: the stored key must survive an update that omits it", updated["api_key_configured"])
		}
	})
}

// A channel without a credential reports api_key_configured=false and no mask.
func TestChannelViewReportsUnconfiguredCredential(t *testing.T) {
	handler, store := newFakeChannelHandler()
	store.seed(&storage.ChannelRecord{
		Name: "local", Type: "openai", BaseURL: "http://local.invalid",
		Models: []string{"m1"}, Status: "active",
	})

	w := invokeChannel(t, handler.ListChannels, http.MethodGet, "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	envelope := decodeEnvelope(t, w)
	data, _ := envelope["data"].([]any)
	channel, _ := data[0].(map[string]any)
	if channel["api_key_configured"] != false {
		t.Errorf("api_key_configured = %v, want false", channel["api_key_configured"])
	}
	if masked, ok := channel["api_key_masked"]; ok && masked != "" {
		t.Errorf("api_key_masked = %v, want it omitted for an unconfigured channel", masked)
	}
}

// The committed-then-hot-reload-failed path (202) goes through the same view.
func TestChannelReloadWarningPayloadUsesTheView(t *testing.T) {
	store := newFakeChannelStore()
	handler := &AdminHandler{channels: NewChannelService(store, &fakeReloader{err: errNoRows}, nil)}
	store.seed(&storage.ChannelRecord{Name: "chan-a", Type: "openai", BaseURL: "http://a.invalid", APIKey: channelSecret})

	w := invokeChannel(t, handler.UpdateChannel, http.MethodPut, `{"name":"renamed"}`, "1")
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", w.Code, w.Body.String())
	}
	assertNoChannelSecret(t, "reload-warning", w.Body.String())
}

// An explicit 0 for the numeric routing fields must reach storage instead of being
// read as "the client omitted it".
func TestUpdateChannelAppliesExplicitZeroRoutingFields(t *testing.T) {
	handler, store := newFakeChannelHandler()
	store.seed(&storage.ChannelRecord{
		Name: "chan-a", Type: "openai", BaseURL: "http://a.invalid",
		Priority: 3, Weight: 7, TimeoutSeconds: 60, Status: "active",
	})

	w := invokeChannel(t, handler.UpdateChannel, http.MethodPut, `{"priority":0,"weight":0}`, "1")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	stored, err := store.GetChannel(1)
	if err != nil {
		t.Fatalf("GetChannel: %v", err)
	}
	if stored.Priority != 0 || stored.Weight != 0 {
		t.Errorf("explicit zeros were dropped: priority=%d weight=%d, want 0/0", stored.Priority, stored.Weight)
	}
}

// An omitted numeric field keeps its stored value.
func TestUpdateChannelKeepsOmittedRoutingFields(t *testing.T) {
	handler, store := newFakeChannelHandler()
	store.seed(&storage.ChannelRecord{
		Name: "chan-a", Type: "openai", BaseURL: "http://a.invalid",
		Priority: 3, Weight: 7, TimeoutSeconds: 60, Status: "active",
	})

	w := invokeChannel(t, handler.UpdateChannel, http.MethodPut, `{"name":"renamed"}`, "1")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	stored, err := store.GetChannel(1)
	if err != nil {
		t.Fatalf("GetChannel: %v", err)
	}
	if stored.Priority != 3 || stored.Weight != 7 || stored.TimeoutSeconds != 60 {
		t.Errorf("omitted routing fields were reset: priority=%d weight=%d timeout=%d",
			stored.Priority, stored.Weight, stored.TimeoutSeconds)
	}
}
