package model

import (
	"reflect"
	"testing"
)

func TestMCPCommandAndEnvironmentValidation(t *testing.T) {
	parts, err := ParseMCPCommand(`python "folder with spaces/server.py" '' '$UNEXPANDED'`)
	if err != nil || !reflect.DeepEqual(parts, []string{"python", "folder with spaces/server.py", "", "$UNEXPANDED"}) {
		t.Fatalf("command: %v %v", parts, err)
	}
	for _, raw := range []string{"", `python "unterminated`, `python trailing\`} {
		if _, err := ParseMCPCommand(raw); err == nil {
			t.Fatalf("accepted malformed command: %q", raw)
		}
	}
	for _, raw := range []string{`null`, `[]`, `{"TOKEN":null}`, `{"TOKEN":12}`, `{"BAD=NAME":"x"}`} {
		if _, err := ParseMCPEnv(raw); err == nil {
			t.Fatalf("accepted malformed environment: %q", raw)
		}
	}
	env, err := ParseMCPEnv(`{"TOKEN":"secret","EMPTY":""}`)
	if err != nil || env["TOKEN"] != "secret" || env["EMPTY"] != "" {
		t.Fatalf("environment: %v %v", env, err)
	}
}
