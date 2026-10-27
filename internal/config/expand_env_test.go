package config

import (
	"os"
	"testing"
)

// Regression: expandConfigEnv delegated to os.ExpandEnv, which treats every '$'
// as a variable reference. A literal dollar sign in a secret was therefore eaten
// (`pa$$word` -> `paword`, `pass$2026` -> `pass2026`) with no warning.
func TestExpandConfigEnvPreservesLiteralDollars(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"double dollar escape", "password: pa$$word", "password: pa$word"},
		{"digit after dollar", "secret_key: pass$2026", "secret_key: pass$2026"},
		{"trailing dollar", "api_key: abc$", "api_key: abc$"},
		{"escaped reference stays literal", "value: $${NOT_A_VAR}", "value: ${NOT_A_VAR}"},
		{"unterminated reference", "value: ${OOPS", "value: ${OOPS"},
		{"multiple escapes", "a: $$$$", "a: $$"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := expandConfigEnv(tc.in); got != tc.want {
				t.Fatalf("expandConfigEnv(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// The documented forms must keep working, and a substituted value must be
// inserted verbatim even when it contains a dollar sign.
func TestExpandConfigEnvSupportsDocumentedForms(t *testing.T) {
	t.Setenv("AIROUTE_TEST_VALUE", "from-env")
	t.Setenv("AIROUTE_TEST_DOLLAR", "pa$word")

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"braced", "port: ${AIROUTE_TEST_VALUE}", "port: from-env"},
		{"braced default used", "port: ${AIROUTE_TEST_MISSING:-8080}", "port: 8080"},
		{"braced env wins over default", "port: ${AIROUTE_TEST_VALUE:-8080}", "port: from-env"},
		{"braced missing without default", "port: ${AIROUTE_TEST_MISSING}", "port: "},
		{"bare", "port: $AIROUTE_TEST_VALUE", "port: from-env"},
		{"env value containing dollar is not re-expanded", "password: ${AIROUTE_TEST_DOLLAR}", "password: pa$word"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := expandConfigEnv(tc.in); got != tc.want {
				t.Fatalf("expandConfigEnv(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// End-to-end: a YAML document whose password contains `$$` must load with a
// single literal dollar, and an unrelated `$1`-looking value must survive.
func TestLoadConfigKeepsLiteralDollarsInSecrets(t *testing.T) {
	path := t.TempDir() + "/config.yaml"
	doc := "admin:\n  password: pa$$word\nserver:\n  port: ${AIROUTE_TEST_PORT:-9091}\n"
	if err := os.WriteFile(path, []byte(doc), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AIROUTE_TEST_PORT", "")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Admin.Password != "pa$word" {
		t.Fatalf("password mangled: got %q want %q", cfg.Admin.Password, "pa$word")
	}
	if cfg.Server.Port != 9091 {
		t.Fatalf("documented default not applied: got %d", cfg.Server.Port)
	}
}
