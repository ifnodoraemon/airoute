package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

var googleModels = struct {
	sync.Mutex
	entries map[string]googleModelEntry
}{entries: make(map[string]googleModelEntry)}

type googleModelEntry struct {
	body    json.RawMessage
	expires time.Time
}

func GoogleModelInfo(ctx context.Context, ch *model.ChannelConfig, name string) json.RawMessage {
	key := model.UpstreamIdentity(ch) + "\x00" + ch.GetUpstreamModel(name)
	googleModels.Lock()
	entry, ok := googleModels.entries[key]
	googleModels.Unlock()
	if ok && time.Now().Before(entry.expires) {
		return append([]byte(nil), entry.body...)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	target := strings.TrimSuffix(GeminiActionURL(GeminiBaseURL(ch), ch.GetUpstreamModel(name), ""), ":")
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil
	}
	if ConfigureGoogleRequest(ctx, r, ch) != nil {
		return nil
	}
	response, err := SharedDefaultHTTPClient.Do(r)
	var body []byte
	if err == nil {
		defer response.Body.Close()
		if response.StatusCode == 200 {
			body, _ = io.ReadAll(io.LimitReader(response.Body, 1<<20))
			if !json.Valid(body) {
				body = nil
			}
		}
	}
	ttl := 5 * time.Minute
	if len(body) == 0 {
		ttl = 30 * time.Second
	}
	googleModels.Lock()
	defer googleModels.Unlock()
	if len(googleModels.entries) >= 2048 {
		googleModels.entries = make(map[string]googleModelEntry)
	}
	googleModels.entries[key] = googleModelEntry{body: body, expires: time.Now().Add(ttl)}
	return body
}
