package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTempConfig writes a YAML config fixture and returns its path.
func writeTempConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

// clearStorageEnv removes every STORAGE_* override so the fixture decides.
func clearStorageEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"STORAGE_DRIVER", "STORAGE_LOCAL_PATH", "STORAGE_S3_ENDPOINT", "STORAGE_S3_BUCKET",
		"STORAGE_S3_ACCESS_KEY", "STORAGE_S3_SECRET_KEY", "STORAGE_S3_REGION",
		"STORAGE_S3_USE_SSL", "STORAGE_S3_PATH_STYLE", "STORAGE_S3_PUBLIC_URL_PREFIX",
	} {
		t.Setenv(name, "")
	}
}

// A partial `storage:` block (here only s3.bucket, no driver) must not be thrown
// away. The old resolver replaced the whole configuration only when `driver` was
// set, so this fixture silently kept the default bucket.
func TestPartialStorageBlockKeepsConfiguredFields(t *testing.T) {
	clearStorageEnv(t)
	path := writeTempConfig(t, `
server:
  port: 8080
storage:
  s3:
    bucket: my-custom-bucket
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	sc := cfg.GetStorageConfig()
	if sc.Driver != "local" {
		t.Fatalf("driver = %q, want local (default when omitted)", sc.Driver)
	}
	if sc.S3.Bucket != "my-custom-bucket" {
		t.Fatalf("bucket = %q, want my-custom-bucket: a partial storage block was discarded", sc.S3.Bucket)
	}
	if !sc.S3.PathStyle {
		t.Fatal("path_style defaulted to false; the omitted key must keep its default")
	}
	if err := sc.Validate(); err != nil {
		t.Fatalf("valid local config rejected: %v", err)
	}
}

// An explicit driver with credentials validates; a remote driver without
// credentials must fail loudly instead of silently using a local directory or a
// repository-published default secret.
func TestStorageValidationRequiresExplicitRemoteCredentials(t *testing.T) {
	clearStorageEnv(t)

	withCreds := writeTempConfig(t, `
storage:
  driver: s3
  s3:
    endpoint: http://storage:9000
    access_key: explicit-access
    secret_key: explicit-secret
`)
	cfg, err := LoadConfig(withCreds)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.GetStorageConfig().Validate(); err != nil {
		t.Fatalf("complete s3 config rejected: %v", err)
	}

	withoutCreds := writeTempConfig(t, `
storage:
  driver: s3
  s3:
    endpoint: http://storage:9000
`)
	cfg, err = LoadConfig(withoutCreds)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	err = cfg.GetStorageConfig().Validate()
	if err == nil {
		t.Fatal("s3 storage without credentials validated; it must fail instead of using a default secret")
	}
	if !strings.Contains(err.Error(), "STORAGE_S3_ACCESS_KEY") || !strings.Contains(err.Error(), "STORAGE_S3_SECRET_KEY") {
		t.Fatalf("validation error does not name the missing variables: %v", err)
	}
}

func TestStorageValidationRejectsUnknownDriver(t *testing.T) {
	clearStorageEnv(t)
	path := writeTempConfig(t, `
storage:
  driver: ftp
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.GetStorageConfig().Validate(); err == nil {
		t.Fatal("unknown storage driver validated")
	}
}

// Environment overrides still win over the YAML block.
func TestStorageEnvOverridesYAML(t *testing.T) {
	clearStorageEnv(t)
	t.Setenv("STORAGE_DRIVER", "s3")
	t.Setenv("STORAGE_S3_ENDPOINT", "http://env-storage:9000")
	t.Setenv("STORAGE_S3_ACCESS_KEY", "env-access")
	t.Setenv("STORAGE_S3_SECRET_KEY", "env-secret")

	path := writeTempConfig(t, `
storage:
  driver: local
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	sc := cfg.GetStorageConfig()
	if sc.Driver != "s3" || sc.S3.Endpoint != "http://env-storage:9000" {
		t.Fatalf("env overrides not applied: %+v", sc)
	}
	if sc.S3.AccessKey != "env-access" || sc.S3.SecretKey != "env-secret" {
		t.Fatalf("env credentials not applied: %+v", sc.S3)
	}
	if err := sc.Validate(); err != nil {
		t.Fatalf("env-configured s3 rejected: %v", err)
	}
}

// A programmatically built Config with a partial storage block must keep the
// fields it does set. The old resolver replaced the whole resolved config only
// when `driver` was non-empty, so this Config fell back to the defaults and the
// configured bucket was silently ignored.
func TestProgrammaticPartialStorageIsMerged(t *testing.T) {
	clearStorageEnv(t)
	cfg := &Config{Storage: StorageConfig{S3: S3Config{Bucket: "programmatic-bucket", Region: "eu-central-1"}}}
	sc := cfg.GetStorageConfig()
	if sc.S3.Bucket != "programmatic-bucket" {
		t.Fatalf("bucket = %q, want programmatic-bucket: a partial storage value was discarded", sc.S3.Bucket)
	}
	if sc.S3.Region != "eu-central-1" {
		t.Fatalf("region = %q, want eu-central-1", sc.S3.Region)
	}
	if sc.Driver != "local" {
		t.Fatalf("driver = %q, want local default", sc.Driver)
	}
	if err := sc.Validate(); err != nil {
		t.Fatalf("merged local config rejected: %v", err)
	}
}

// No repository-published credential may be used when nothing is configured.
func TestNoImplicitS3Credentials(t *testing.T) {
	clearStorageEnv(t)
	sc := DefaultConfig().GetStorageConfig()
	if sc.S3.AccessKey != "" || sc.S3.SecretKey != "" {
		t.Fatalf("default storage config carries credentials (access=%q, secret set=%t); "+
			"they must come from the deployment, never from the repository",
			sc.S3.AccessKey, sc.S3.SecretKey != "")
	}
	if err := sc.Validate(); err != nil {
		t.Fatalf("default local storage rejected: %v", err)
	}
}
