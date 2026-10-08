package storage

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/config"
)

// ArtifactStorage defines the pluggable storage interface for skill ZIP packages,
// MCP artifacts, attachments, and binary files.
type ArtifactStorage interface {
	// Driver returns the underlying storage driver name ("local", "s3", "rustfs").
	Driver() string

	// Put stores an artifact with the given key, content stream, size and content type.
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error

	// Get retrieves an artifact content stream and its size.
	Get(ctx context.Context, key string) (io.ReadCloser, int64, error)

	// GetDownloadURL generates a direct or presigned URL for downloading the artifact.
	// If the driver does not support presigned URLs, it returns an empty string.
	GetDownloadURL(ctx context.Context, key string, expire time.Duration) (string, error)

	// Delete removes an artifact by key.
	Delete(ctx context.Context, key string) error

	// Exists checks if an artifact exists.
	Exists(ctx context.Context, key string) (bool, error)
}

// NewArtifactStorage creates an ArtifactStorage instance based on configuration.
func NewArtifactStorage(cfg config.StorageConfig) (ArtifactStorage, error) {
	driver := strings.ToLower(strings.TrimSpace(cfg.Driver))
	switch driver {
	case "s3", "rustfs":
		return NewS3Storage(cfg.S3)
	default:
		return NewLocalStorage(cfg.LocalPath)
	}
}

// FormatSkillKey returns the standard object storage key for a skill ZIP bundle.
func FormatSkillKey(skillID string) string {
	return fmt.Sprintf("skills/%s/%s.zip", skillID, skillID)
}
