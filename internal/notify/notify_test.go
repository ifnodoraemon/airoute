package notify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func sampleMessage() Message {
	return Message{
		Event: "budget.alert",
		Level: LevelWarning,
		Title: "部门预算达到 80%",
		Body:  "算力运营部本月预算已使用 80%",
		Fields: map[string]string{
			"org":    "算力运营部",
			"period": "monthly",
			"used":   "800",
			"budget": "1000",
		},
		Timestamp: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC),
	}
}

func captureServer(t *testing.T, handler func(body map[string]any)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		if handler != nil {
			handler(body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestWebhookSenderPayload(t *testing.T) {
	var captured map[string]any
	server := captureServer(t, func(body map[string]any) { captured = body })
	sender := WebhookSender{URL: server.URL, Headers: map[string]string{"X-Token": "s3cret"}}
	if err := sender.Send(context.Background(), sampleMessage()); err != nil {
		t.Fatalf("send: %v", err)
	}
	if captured["source"] != "airoute" || captured["event"] != "budget.alert" || captured["level"] != "warning" {
		t.Fatalf("payload = %+v", captured)
	}
	if captured["title"] != "部门预算达到 80%" {
		t.Fatalf("title = %v", captured["title"])
	}
	if fields, ok := captured["fields"].(map[string]any); !ok || fields["org"] != "算力运营部" {
		t.Fatalf("fields = %+v", captured["fields"])
	}
}

func TestIMSenderPayloadShapes(t *testing.T) {
	cases := []struct {
		name   string
		sender func(url string) Sender
		check  func(t *testing.T, body map[string]any)
	}{
		{
			name:   "dingtalk",
			sender: func(url string) Sender { return DingTalkSender{URL: url} },
			check: func(t *testing.T, body map[string]any) {
				if body["msgtype"] != "markdown" {
					t.Fatalf("msgtype = %v", body["msgtype"])
				}
				markdown, _ := body["markdown"].(map[string]any)
				if markdown["title"] != "部门预算达到 80%" || !strings.Contains(markdown["text"].(string), "算力运营部") {
					t.Fatalf("markdown = %+v", markdown)
				}
			},
		},
		{
			name:   "feishu",
			sender: func(url string) Sender { return FeishuSender{URL: url} },
			check: func(t *testing.T, body map[string]any) {
				if body["msg_type"] != "text" {
					t.Fatalf("msg_type = %v", body["msg_type"])
				}
				content, _ := body["content"].(map[string]any)
				if !strings.Contains(content["text"].(string), "budget") && !strings.Contains(content["text"].(string), "预算") {
					t.Fatalf("content = %+v", content)
				}
			},
		},
		{
			name:   "wecom",
			sender: func(url string) Sender { return WeComSender{URL: url} },
			check: func(t *testing.T, body map[string]any) {
				if body["msgtype"] != "markdown" {
					t.Fatalf("msgtype = %v", body["msgtype"])
				}
				markdown, _ := body["markdown"].(map[string]any)
				if !strings.Contains(markdown["content"].(string), "算力运营部") {
					t.Fatalf("markdown = %+v", markdown)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var captured map[string]any
			server := captureServer(t, func(body map[string]any) { captured = body })
			if err := tc.sender(server.URL).Send(context.Background(), sampleMessage()); err != nil {
				t.Fatalf("send: %v", err)
			}
			tc.check(t, captured)
		})
	}
}

func TestSenderReportsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer server.Close()
	err := (WebhookSender{URL: server.URL}).Send(context.Background(), sampleMessage())
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("expected a 502 error, got %v", err)
	}
}

type fakeSender struct {
	name string
	err  error
	seen *int
}

func (f fakeSender) Name() string { return f.name }
func (f fakeSender) Send(context.Context, Message) error {
	if f.seen != nil {
		*f.seen++
	}
	return f.err
}

func TestMultiSenderIsolatesFailures(t *testing.T) {
	calls := 0
	multi := MultiSender{Senders: []Sender{
		fakeSender{name: "ok", seen: &calls},
		fakeSender{name: "broken", err: errors.New("down")},
		fakeSender{name: "ok2", seen: &calls},
	}}
	err := multi.Send(context.Background(), sampleMessage())
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("expected the broken sender in the error, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("healthy senders called %d times, want 2", calls)
	}
}

func TestEmailValidationAndBody(t *testing.T) {
	if err := (EmailSender{}).Validate(); err == nil {
		t.Fatal("empty SMTP config must not validate")
	}
	if err := (EmailSender{Host: "smtp.local", From: "a@local"}).Validate(); err == nil {
		t.Fatal("missing recipient must not validate")
	}
	sender := EmailSender{Host: "smtp.local", From: "a@local", To: []string{"b@local"}}
	if err := sender.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	body := string(buildEmail("a@local", []string{"b@local"}, sampleMessage()))
	for _, want := range []string{"From: a@local", "To: b@local", "MIME-Version: 1.0", "算力运营部", "budget"} {
		if !strings.Contains(body, want) {
			t.Fatalf("email body lost %q:\n%s", want, body)
		}
	}
	if !strings.Contains(body, "Subject: ") || !strings.Contains(body, "=?utf-8?") {
		t.Fatalf("subject was not Q-encoded:\n%s", body)
	}
}

func TestMessageFieldsAreSorted(t *testing.T) {
	msg := sampleMessage()
	text := msg.FieldsText()
	lines := strings.Split(text, "\n")
	if len(lines) != 4 {
		t.Fatalf("lines = %v", lines)
	}
	if !strings.HasPrefix(lines[0], "budget:") || !strings.HasPrefix(lines[1], "org:") {
		t.Fatalf("fields not sorted: %v", lines)
	}
}
