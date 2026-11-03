package provider

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func TestVertexServiceAccountAndADCRefreshUseBearerCredentials(t *testing.T) {
	var tokens atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokens.Add(1)
		_ = r.ParseForm()
		if r.Form.Get("assertion") == "" {
			t.Error("missing signed service-account assertion")
		}
		fmt.Fprint(w, `{"access_token":"local-test-token","expires_in":3600,"token_type":"Bearer"}`)
	}))
	defer server.Close()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	private := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	credentials, _ := json.Marshal(map[string]string{"type": "service_account", "client_email": "test@example.invalid", "private_key": string(private), "token_uri": server.URL})
	file := filepath.Join(t.TempDir(), "google.json")
	if err := os.WriteFile(file, credentials, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", file)
	for _, secret := range []string{string(credentials), "adc"} {
		ch := &model.ChannelConfig{Type: model.ProviderVertex, APIKey: secret, BaseURL: server.URL + "/v1/projects/test/locations/global/publishers/google"}
		for i := 0; i < 2; i++ {
			request, _ := http.NewRequest(http.MethodPost, server.URL, nil)
			if err := ConfigureGoogleRequest(context.Background(), request, ch); err != nil {
				t.Fatal(err)
			}
			if request.Header.Get("Authorization") != "Bearer local-test-token" || request.Header.Get("x-goog-api-key") != "" {
				t.Fatalf("wrong Vertex auth headers: %v", request.Header)
			}
		}
		if target := buildGeminiURL(GeminiBaseURL(ch), "gemini", false); !strings.HasSuffix(target, "/publishers/google/models/gemini:generateContent") || strings.Contains(target, "v1beta") {
			t.Fatalf("wrong Vertex path: %s", target)
		}
	}
	if tokens.Load() != 2 {
		t.Fatalf("cached tokens refreshed %d times", tokens.Load())
	}
}
