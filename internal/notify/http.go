package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultHTTPTimeout = 5 * time.Second

func httpClientOrDefault(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	return &http.Client{Timeout: defaultHTTPTimeout}
}

func postJSON(ctx context.Context, client *http.Client, target string, headers map[string]string, payload any) error {
	if strings.TrimSpace(target) == "" {
		return fmt.Errorf("notification endpoint is empty")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal notification payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build notification request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "airoute-notifier/1")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := httpClientOrDefault(client).Do(req)
	if err != nil {
		return fmt.Errorf("post notification: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("notification endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	return nil
}

// WebhookSender posts a transport-neutral JSON document. It is the integration
// point for a customer's own alerting/automation system.
type WebhookSender struct {
	URL     string
	Headers map[string]string
	Client  *http.Client
}

func (s WebhookSender) Name() string { return "webhook" }

func (s WebhookSender) Send(ctx context.Context, msg Message) error {
	msg = msg.Normalize()
	event := msg.Event
	if event == "" {
		event = "airoute.alert"
	}
	payload := map[string]any{
		"source":    "airoute",
		"event":     event,
		"level":     string(msg.Level),
		"title":     msg.Title,
		"body":      msg.Body,
		"fields":    msg.Fields,
		"timestamp": msg.Timestamp.UTC().Format(time.RFC3339),
	}
	return postJSON(ctx, s.Client, s.URL, s.Headers, payload)
}

// DingTalkSender posts a markdown message to a DingTalk group bot.
type DingTalkSender struct {
	URL    string
	Client *http.Client
}

func (s DingTalkSender) Name() string { return "dingtalk" }

func (s DingTalkSender) Send(ctx context.Context, msg Message) error {
	msg = msg.Normalize()
	text := markdownText(msg)
	payload := map[string]any{
		"msgtype":  "markdown",
		"markdown": map[string]string{"title": msg.Title, "text": text},
	}
	return postJSON(ctx, s.Client, s.URL, nil, payload)
}

// FeishuSender posts a plain-text message to a Feishu (Lark) bot webhook.
type FeishuSender struct {
	URL    string
	Client *http.Client
}

func (s FeishuSender) Name() string { return "feishu" }

func (s FeishuSender) Send(ctx context.Context, msg Message) error {
	msg = msg.Normalize()
	payload := map[string]any{
		"msg_type": "text",
		"content":  map[string]string{"text": plainText(msg)},
	}
	return postJSON(ctx, s.Client, s.URL, nil, payload)
}

// WeComSender posts a markdown message to a WeCom (企业微信) group bot.
type WeComSender struct {
	URL    string
	Client *http.Client
}

func (s WeComSender) Name() string { return "wecom" }

func (s WeComSender) Send(ctx context.Context, msg Message) error {
	msg = msg.Normalize()
	payload := map[string]any{
		"msgtype":  "markdown",
		"markdown": map[string]string{"content": markdownText(msg)},
	}
	return postJSON(ctx, s.Client, s.URL, nil, payload)
}

func plainText(msg Message) string {
	parts := []string{fmt.Sprintf("[%s] %s", msg.Level, msg.Title)}
	if strings.TrimSpace(msg.Body) != "" {
		parts = append(parts, msg.Body)
	}
	if fields := msg.FieldsText(); fields != "" {
		parts = append(parts, fields)
	}
	return strings.Join(parts, "\n")
}

func markdownText(msg Message) string {
	parts := []string{fmt.Sprintf("### [%s] %s", msg.Level, msg.Title)}
	if strings.TrimSpace(msg.Body) != "" {
		parts = append(parts, msg.Body)
	}
	if fields := msg.FieldsText(); fields != "" {
		parts = append(parts, fields)
	}
	return strings.Join(parts, "\n\n")
}
