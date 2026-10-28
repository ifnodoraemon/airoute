package controlplane

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func int64String(v int64) string { return strconv.FormatInt(v, 10) }

// These tests pin the HTTP contract of the channel endpoints: the status codes
// and the {code, data} envelope the console depends on. They deliberately run
// through the real handler, the real service and a real (in-memory) repository,
// so a change to any of the three that alters the contract is caught here.
//
// The handler is built with a nil synchronizer and a nil dispatcher on purpose:
// that is the degradation path a single-node deployment without a cluster uses.

func newChannelTestHandler(t *testing.T) (*AdminHandler, *storage.Repository) {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := storage.NewRepository(db)
	return NewAdminHandler(repo, nil, nil), repo
}

func invokeChannel(t *testing.T, handler gin.HandlerFunc, method, body, id string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(method, "/api/v1/admin/channels", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	if id != "" {
		c.Params = gin.Params{{Key: "id", Value: id}}
	}
	handler(c)
	return w
}

func decodeEnvelope(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not JSON (%d): %s", w.Code, w.Body.String())
	}
	return out
}

// storedChannel reads one channel back by id. The repository no longer exposes a
// single-row read (only ListChannels and PatchChannel), so tests look it up in the
// list — which is also how the console sees channels.
func storedChannel(t *testing.T, repo *storage.Repository, id int64) *storage.ChannelRecord {
	t.Helper()
	channels, err := repo.ListChannels()
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	for _, ch := range channels {
		if ch.ID == id {
			return ch
		}
	}
	t.Fatalf("channel %d not found", id)
	return nil
}

const validChannelBody = `{"name":"chan-a","type":"openai","base_url":"http://a.invalid","api_key":"secret","models":["m1"],"priority":3,"weight":7,"status":"active"}`

func TestCreateChannelCommittedReloadWarning(t *testing.T) {
	store := newFakeChannelStore()
	handler := &AdminHandler{channels: NewChannelService(store, &fakeReloader{err: errors.New("reload offline")}, nil)}
	w := invokeChannel(t, handler.CreateChannel, http.MethodPost, validChannelBody, "")
	envelope := decodeEnvelope(t, w)
	if w.Code != http.StatusAccepted || envelope["code"] != float64(0) || envelope["warning"] == nil || envelope["data"] == nil || store.createCalls != 1 {
		t.Fatalf("want committed record and 202 warning, got %d %s", w.Code, w.Body.String())
	}
}

func TestCreateChannelHTTPContract(t *testing.T) {
	handler, repo := newChannelTestHandler(t)

	t.Run("creates and returns the envelope", func(t *testing.T) {
		w := invokeChannel(t, handler.CreateChannel, http.MethodPost, validChannelBody, "")

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
		}
		env := decodeEnvelope(t, w)
		if code, _ := env["code"].(float64); code != 0 {
			t.Errorf("envelope code = %v, want 0", env["code"])
		}
		data, ok := env["data"].(map[string]interface{})
		if !ok {
			t.Fatalf("envelope has no data object: %v", env)
		}
		if data["name"] != "chan-a" {
			t.Errorf("data.name = %v, want chan-a", data["name"])
		}
		if _, hasID := data["id"]; !hasID {
			t.Error("response does not expose the assigned id")
		}

		// And it really is persisted.
		list, err := repo.ListChannels()
		if err != nil {
			t.Fatalf("ListChannels: %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("stored channels = %d, want 1", len(list))
		}
	})

	t.Run("rejects a missing required field with 400", func(t *testing.T) {
		w := invokeChannel(t, handler.CreateChannel, http.MethodPost,
			`{"name":"","type":"openai","base_url":"http://a.invalid"}`, "")

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", w.Code, w.Body.String())
		}
		if msg, _ := decodeEnvelope(t, w)["error"].(string); msg == "" {
			t.Error("400 response carries no error message")
		}
	})

	t.Run("rejects a malformed body with 400", func(t *testing.T) {
		w := invokeChannel(t, handler.CreateChannel, http.MethodPost, `{not json`, "")

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})
}

func TestUpdateChannelHTTPContract(t *testing.T) {
	handler, repo := newChannelTestHandler(t)

	seeded := &storage.ChannelRecord{
		Name: "chan-a", Type: "openai", BaseURL: "http://a.invalid",
		APIKey: "secret", Models: []string{"m1"}, Priority: 3, Weight: 7, Status: "active",
	}
	if err := repo.CreateChannel(seeded); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	id := int64String(seeded.ID)

	t.Run("merges a partial update without wiping omitted fields", func(t *testing.T) {
		w := invokeChannel(t, handler.UpdateChannel, http.MethodPut, `{"name":"renamed"}`, id)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
		}

		stored := storedChannel(t, repo, seeded.ID)
		if stored.Name != "renamed" {
			t.Errorf("name = %q, want renamed", stored.Name)
		}
		if stored.APIKey != "secret" {
			t.Errorf("omitted api_key was wiped: %q", stored.APIKey)
		}
		if stored.BaseURL != "http://a.invalid" {
			t.Errorf("omitted base_url was wiped: %q", stored.BaseURL)
		}
	})

	t.Run("returns 404 for an unknown channel", func(t *testing.T) {
		w := invokeChannel(t, handler.UpdateChannel, http.MethodPut, `{"name":"x"}`, "999999")

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body: %s)", w.Code, w.Body.String())
		}
	})

	t.Run("returns 400 for a non-numeric id", func(t *testing.T) {
		w := invokeChannel(t, handler.UpdateChannel, http.MethodPut, `{"name":"x"}`, "not-a-number")

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})
}

func TestDeleteChannelHTTPContract(t *testing.T) {
	handler, repo := newChannelTestHandler(t)

	seeded := &storage.ChannelRecord{Name: "chan-a", Type: "openai", BaseURL: "http://a.invalid", Status: "active"}
	if err := repo.CreateChannel(seeded); err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	w := invokeChannel(t, handler.DeleteChannel, http.MethodDelete, "", int64String(seeded.ID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}

	list, err := repo.ListChannels()
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("channel survived the delete: %d remain", len(list))
	}

	t.Run("returns 400 for a non-numeric id", func(t *testing.T) {
		w := invokeChannel(t, handler.DeleteChannel, http.MethodDelete, "", "abc")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})
}

func TestListChannelsHTTPContract(t *testing.T) {
	handler, repo := newChannelTestHandler(t)

	if err := repo.CreateChannel(&storage.ChannelRecord{
		Name: "chan-a", Type: "openai", BaseURL: "http://a.invalid", Status: "active",
	}); err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	w := invokeChannel(t, handler.ListChannels, http.MethodGet, "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	env := decodeEnvelope(t, w)
	if code, _ := env["code"].(float64); code != 0 {
		t.Errorf("envelope code = %v, want 0", env["code"])
	}
	data, ok := env["data"].([]interface{})
	if !ok || len(data) != 1 {
		t.Fatalf("data = %v, want a one-element array", env["data"])
	}
}

func TestBatchChannelHTTPContract(t *testing.T) {
	handler, repo := newChannelTestHandler(t)

	a := &storage.ChannelRecord{Name: "a", Type: "openai", BaseURL: "http://a.invalid", Status: "active"}
	b := &storage.ChannelRecord{Name: "b", Type: "openai", BaseURL: "http://b.invalid", Status: "active"}
	for _, ch := range []*storage.ChannelRecord{a, b} {
		if err := repo.CreateChannel(ch); err != nil {
			t.Fatalf("seed channel %s: %v", ch.Name, err)
		}
	}

	t.Run("rejects an empty id list with 400", func(t *testing.T) {
		w := invokeChannel(t, handler.BatchDeleteChannels, http.MethodPost, `{"ids":[]}`, "")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("disables the requested channels", func(t *testing.T) {
		body := `{"ids":[` + int64String(a.ID) + `,` + int64String(b.ID) + `],"status":"disabled"}`
		w := invokeChannel(t, handler.BatchStatusChannels, http.MethodPost, body, "")
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
		}
		for _, id := range []int64{a.ID, b.ID} {
			stored := storedChannel(t, repo, id)
			if stored.Status != "disabled" {
				t.Errorf("channel %d status = %q, want disabled", id, stored.Status)
			}
		}
	})

	t.Run("coerces an unknown status to active", func(t *testing.T) {
		body := `{"ids":[` + int64String(a.ID) + `],"status":"nonsense"}`
		w := invokeChannel(t, handler.BatchStatusChannels, http.MethodPost, body, "")
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		stored := storedChannel(t, repo, a.ID)
		if stored.Status != "active" {
			t.Errorf("status = %q, want active", stored.Status)
		}
	})
}
