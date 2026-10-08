package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LocalStorage implements ArtifactStorage on the local filesystem.
type LocalStorage struct {
	baseDir string
	mu      sync.RWMutex
}

// NewLocalStorage creates a LocalStorage instance rooted at baseDir.
func NewLocalStorage(baseDir string) (*LocalStorage, error) {
	if baseDir == "" {
		baseDir = "data/storage"
	}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		baseDir = filepath.Join(os.TempDir(), "airoute-storage")
		_ = os.MkdirAll(baseDir, 0755)
	}
	return &LocalStorage{baseDir: baseDir}, nil
}

func (s *LocalStorage) Driver() string {
	return "local"
}

func (s *LocalStorage) filePath(key string) string {
	cleanKey := filepath.Clean("/" + key)
	return filepath.Join(s.baseDir, cleanKey)
}

func (s *LocalStorage) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if s == nil {
		return fmt.Errorf("local storage not initialized")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	target := s.filePath(key)
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory for artifact %q: %w", target, err)
	}

	tmpFile := target + ".tmp"
	f, err := os.Create(tmpFile)
	if err != nil {
		return fmt.Errorf("failed to create tmp artifact file: %w", err)
	}

	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to write artifact data: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to close tmp file: %w", err)
	}

	if err := os.Rename(tmpFile, target); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to commit artifact file: %w", err)
	}
	return nil
}

func (s *LocalStorage) Get(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	if s == nil {
		return nil, 0, fmt.Errorf("local storage not initialized")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	target := s.filePath(key)
	stat, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, fmt.Errorf("artifact not found: %s", key)
		}
		return nil, 0, err
	}

	f, err := os.Open(target)
	if err != nil {
		return nil, 0, err
	}
	return f, stat.Size(), nil
}

func (s *LocalStorage) GetDownloadURL(ctx context.Context, key string, expire time.Duration) (string, error) {
	// Local storage does not produce remote presigned URLs; gateway serves content stream.
	return "", nil
}

func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	target := s.filePath(key)
	err := os.Remove(target)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *LocalStorage) Exists(ctx context.Context, key string) (bool, error) {
	if s == nil {
		return false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	target := s.filePath(key)
	_, err := os.Stat(target)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
