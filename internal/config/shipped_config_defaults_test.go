package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The shipped configs/config.yaml must be safe by default: no open
// self-registration, no implicit trial credit, and no repository-published S3
// credential. Operators opt into each of those explicitly.
func TestShippedConfigHasSafeDefaults(t *testing.T) {
	for _, name := range []string{
		"ALLOW_REGISTRATION", "INITIAL_USER_BALANCE", "DEFAULT_TRIAL_BALANCE",
		"REQUIRE_EMAIL_VERIFICATION",
		"STORAGE_DRIVER", "STORAGE_LOCAL_PATH", "STORAGE_S3_ENDPOINT", "STORAGE_S3_BUCKET",
		"STORAGE_S3_ACCESS_KEY", "STORAGE_S3_SECRET_KEY",
	} {
		t.Setenv(name, "")
	}

	path := filepath.Join("..", "..", "configs", "config.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	// Only active configuration lines matter; the comments intentionally mention
	// admin123 to explain why it is rejected at startup.
	for lineNo, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		for _, forbidden := range []string{"airoute_cluster_secret_pass", "admin123"} {
			if strings.Contains(trimmed, forbidden) {
				t.Fatalf("%s:%d still contains the repository-published credential %q: %s", path, lineNo+1, forbidden, trimmed)
			}
		}
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig(%s): %v", path, err)
	}
	if cfg.IsRegistrationAllowed() {
		t.Fatal("shipped config opens public self-registration by default")
	}
	if got := cfg.GetInitialUserBalance(); got != 0 {
		t.Fatalf("shipped config grants %.2f trial credit by default, want 0", got)
	}
	sc := cfg.GetStorageConfig()
	if sc.S3.AccessKey != "" || sc.S3.SecretKey != "" {
		t.Fatal("shipped config falls back to repository-published S3 credentials")
	}
}
