package storage

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/config"
)

// A-15 regression: only the documented STORAGE_S3_ENDPOINT is honoured.
//
// The generic S3_ENDPOINT name is used by every S3 SDK/CLI, so a stray value in
// the environment silently pointed the gateway's artifact storage at a different
// service. Configuration must be explicit.
func TestNewS3StorageIgnoresGenericS3Endpoint(t *testing.T) {
	t.Setenv("STORAGE_S3_ENDPOINT", "")
	t.Setenv("S3_ENDPOINT", "http://generic-s3.invalid:9000")

	if _, err := NewS3Storage(config.S3Config{}); err == nil {
		t.Fatal("S3_ENDPOINT must not be used as a fallback endpoint")
	}
}

// The documented variable is still accepted when no explicit endpoint is set.
func TestNewS3StorageAcceptsDocumentedEndpoint(t *testing.T) {
	t.Setenv("S3_ENDPOINT", "http://generic-s3.invalid:9000")
	t.Setenv("STORAGE_S3_ENDPOINT", "http://127.0.0.1:1")

	s, err := NewS3Storage(config.S3Config{})
	if err != nil {
		t.Fatalf("STORAGE_S3_ENDPOINT should be honoured: %v", err)
	}
	if s == nil || s.endpoint == nil || s.endpoint.Host != "127.0.0.1:1" {
		t.Fatalf("endpoint resolved to %+v, want 127.0.0.1:1", s)
	}
}
