package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var googleSources = struct {
	sync.Mutex
	values map[string]oauth2.TokenSource
}{values: make(map[string]oauth2.TokenSource)}

func IsVertexChannel(ch *model.ChannelConfig) bool {
	u, _ := url.Parse(ch.BaseURL)
	return ch.Type == model.ProviderVertex || (u != nil && (strings.HasSuffix(u.Hostname(), "aiplatform.googleapis.com") || strings.Contains(u.Path, "/publishers/google")))
}

func GeminiBaseURL(ch *model.ChannelConfig) string {
	if ch.BaseURL != "" {
		return ch.BaseURL
	}
	if ch.Type != model.ProviderVertex {
		return "https://generativelanguage.googleapis.com"
	}
	project, location := os.Getenv("GOOGLE_CLOUD_PROJECT"), os.Getenv("GOOGLE_CLOUD_LOCATION")
	if location == "" {
		location = "global"
	}
	host := "aiplatform.googleapis.com"
	if location != "global" {
		host = location + "-" + host
	}
	return "https://" + host + "/v1/projects/" + url.PathEscape(project) + "/locations/" + url.PathEscape(location) + "/publishers/google"
}

func ConfigureGoogleRequest(ctx context.Context, r *http.Request, ch *model.ChannelConfig) error {
	SetGeminiHeaders(r, "")
	if !IsVertexChannel(ch) {
		SetGeminiHeaders(r, ch.APIKey)
		return nil
	}
	if strings.HasPrefix(ch.APIKey, "bearer:") {
		r.Header.Set("Authorization", "Bearer "+strings.TrimPrefix(ch.APIKey, "bearer:"))
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	source, err := googleTokenSource(ch.APIKey)
	if err != nil {
		return fmt.Errorf("load Google credentials: %w", err)
	}
	token, err := source.Token()
	if err != nil {
		return fmt.Errorf("refresh Google credentials: %w", err)
	}
	token.SetAuthHeader(r)
	r.Header.Del("x-goog-api-key")
	return nil
}

func googleTokenSource(credential string) (oauth2.TokenSource, error) {
	sum := sha256.Sum256([]byte(credential + "\x00" + os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")))
	key := hex.EncodeToString(sum[:])
	googleSources.Lock()
	defer googleSources.Unlock()
	if source := googleSources.values[key]; source != nil {
		return source, nil
	}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Timeout: 10 * time.Second})
	var source oauth2.TokenSource
	if strings.HasPrefix(strings.TrimSpace(credential), "{") {
		config, err := google.JWTConfigFromJSON([]byte(credential), "https://www.googleapis.com/auth/cloud-platform")
		if err != nil {
			return nil, err
		}
		source = config.TokenSource(ctx)
	} else {
		if credential != "" && credential != "adc" && credential != "none" {
			return nil, fmt.Errorf("Vertex requires adc, service-account JSON or bearer:<access-token>")
		}
		config, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
		if err != nil {
			return nil, err
		}
		source = config.TokenSource
	}
	if len(googleSources.values) >= 128 {
		googleSources.values = make(map[string]oauth2.TokenSource)
	}
	source = oauth2.ReuseTokenSource(nil, source)
	googleSources.values[key] = source
	return source, nil
}
