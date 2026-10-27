package notify

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Environment variables read by FromEnv. They are intentionally alert-specific
// (GATEWAY_ALERT_*) except for the SMTP relay, which is shared with account
// verification so operators configure it once.
const (
	EnvWebhookURL  = "GATEWAY_ALERT_WEBHOOK_URL"
	EnvDingTalkURL = "GATEWAY_ALERT_DINGTALK_URL"
	EnvFeishuURL   = "GATEWAY_ALERT_FEISHU_URL"
	EnvWeComURL    = "GATEWAY_ALERT_WECOM_URL"
	EnvEmailTo     = "GATEWAY_ALERT_EMAIL_TO"

	EnvSMTPHost = "SMTP_HOST"
	EnvSMTPPort = "SMTP_PORT"
	EnvSMTPUser = "SMTP_USER"
	EnvSMTPPass = "SMTP_PASS"
	EnvSMTPFrom = "SMTP_FROM"
)

// FromEnv builds a MultiSender from the process environment. The second return
// value carries configuration warnings (for example an e-mail recipient without
// an SMTP relay); incomplete transports are skipped rather than made to fail on
// every alert. Callers should log the warnings at startup.
func FromEnv(getenv func(string) string) (MultiSender, []string) {
	if getenv == nil {
		getenv = os.Getenv
	}
	var senders []Sender
	var warnings []string

	add := func(name, value string, build func(string) Sender) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		senders = append(senders, build(value))
	}
	add("webhook", getenv(EnvWebhookURL), func(url string) Sender { return WebhookSender{URL: url} })
	add("dingtalk", getenv(EnvDingTalkURL), func(url string) Sender { return DingTalkSender{URL: url} })
	add("feishu", getenv(EnvFeishuURL), func(url string) Sender { return FeishuSender{URL: url} })
	add("wecom", getenv(EnvWeComURL), func(url string) Sender { return WeComSender{URL: url} })

	host := strings.TrimSpace(getenv(EnvSMTPHost))
	to := splitRecipients(getenv(EnvEmailTo))
	if host != "" || len(to) > 0 {
		port := 587
		if raw := strings.TrimSpace(getenv(EnvSMTPPort)); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
				port = parsed
			} else {
				warnings = append(warnings, fmt.Sprintf("%s=%q is invalid; using 587", EnvSMTPPort, raw))
			}
		}
		sender := EmailSender{
			Host:     host,
			Port:     port,
			Username: strings.TrimSpace(getenv(EnvSMTPUser)),
			Password: getenv(EnvSMTPPass),
			From:     strings.TrimSpace(getenv(EnvSMTPFrom)),
			To:       to,
		}
		if err := sender.Validate(); err != nil {
			warnings = append(warnings, "email alerts disabled: "+err.Error())
		} else {
			senders = append(senders, sender)
		}
	}

	return MultiSender{Senders: senders}, warnings
}

func splitRecipients(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n'
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if recipient := strings.TrimSpace(field); recipient != "" {
			out = append(out, recipient)
		}
	}
	return out
}
