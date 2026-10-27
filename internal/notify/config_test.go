package notify

import (
	"strings"
	"testing"
)

func envMap(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func senderNames(multi MultiSender) []string {
	names := make([]string, 0, len(multi.Senders))
	for _, sender := range multi.Senders {
		names = append(names, sender.Name())
	}
	return names
}

func containsName(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

func TestFromEnvBuildsEveryTransport(t *testing.T) {
	multi, warnings := FromEnv(envMap(map[string]string{
		EnvWebhookURL:  "https://hook.invalid/webhook",
		EnvDingTalkURL: "https://oapi.dingtalk.invalid/robot",
		EnvFeishuURL:   "https://open.feishu.invalid/bot",
		EnvWeComURL:    "https://qyapi.weixin.invalid/bot",
		EnvSMTPHost:    "smtp.invalid",
		EnvSMTPPort:    "2525",
		EnvSMTPUser:    "airoute",
		EnvSMTPPass:    "secret",
		EnvSMTPFrom:    "airoute@invalid",
		EnvEmailTo:     "ops@invalid, oncall@invalid",
	}))
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if !multi.Configured() || len(multi.Senders) != 5 {
		t.Fatalf("senders = %v", senderNames(multi))
	}
	for _, want := range []string{"webhook", "dingtalk", "feishu", "wecom", "email"} {
		if !containsName(senderNames(multi), want) {
			t.Fatalf("missing %s in %v", want, senderNames(multi))
		}
	}
	var email EmailSender
	for _, sender := range multi.Senders {
		if candidate, ok := sender.(EmailSender); ok {
			email = candidate
		}
	}
	if email.Port != 2525 || len(email.To) != 2 || email.To[1] != "oncall@invalid" {
		t.Fatalf("email config = %+v", email)
	}
}

func TestFromEnvSkipsIncompleteEmailAndWarns(t *testing.T) {
	multi, warnings := FromEnv(envMap(map[string]string{
		EnvEmailTo: "ops@invalid",
	}))
	if len(multi.Senders) != 0 {
		t.Fatalf("senders = %v", senderNames(multi))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "email alerts disabled") {
		t.Fatalf("warnings = %v", warnings)
	}

	multi, warnings = FromEnv(envMap(map[string]string{
		EnvSMTPHost: "smtp.invalid",
		EnvSMTPFrom: "airoute@invalid",
		EnvEmailTo:  "ops@invalid",
		EnvSMTPPort: "not-a-port",
	}))
	if len(warnings) != 1 || !strings.Contains(warnings[0], "using 587") {
		t.Fatalf("port warning = %v", warnings)
	}
	if len(multi.Senders) != 1 {
		t.Fatalf("senders = %v", senderNames(multi))
	}
}

func TestFromEnvEmpty(t *testing.T) {
	multi, warnings := FromEnv(envMap(nil))
	if multi.Configured() || len(warnings) != 0 {
		t.Fatalf("empty env produced %v / %v", senderNames(multi), warnings)
	}
}
