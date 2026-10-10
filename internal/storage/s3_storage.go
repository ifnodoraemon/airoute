package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/config"
)

// S3Storage implements ArtifactStorage for AWS S3, RustFS, MinIO, and Cloudflare R2
// using native Go AWS Signature Version 4 (SigV4) with zero external dependencies.
type S3Storage struct {
	endpoint        *url.URL
	bucket          string
	accessKey       string
	secretKey       string
	region          string
	useSSL          bool
	pathStyle       bool
	publicURLPrefix string
	client          *http.Client
}

// NewS3Storage creates an S3/RustFS storage driver.
func NewS3Storage(cfg config.S3Config) (*S3Storage, error) {
	endpointStr := cfg.Endpoint
	if endpointStr == "" {
		endpointStr = os.Getenv("STORAGE_S3_ENDPOINT")
		if endpointStr == "" {
			endpointStr = os.Getenv("S3_ENDPOINT")
		}
	}
	if endpointStr == "" {
		return nil, fmt.Errorf("S3/RustFS storage endpoint is not configured (STORAGE_S3_ENDPOINT is required)")
	}
	if !strings.HasPrefix(endpointStr, "http://") && !strings.HasPrefix(endpointStr, "https://") {
		if cfg.UseSSL {
			endpointStr = "https://" + endpointStr
		} else {
			endpointStr = "http://" + endpointStr
		}
	}

	u, err := url.Parse(endpointStr)
	if err != nil {
		return nil, fmt.Errorf("invalid S3/RustFS endpoint %q: %w", cfg.Endpoint, err)
	}

	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	bucket := cfg.Bucket
	if bucket == "" {
		bucket = "airoute-skills"
	}

	s := &S3Storage{
		endpoint:        u,
		bucket:          bucket,
		accessKey:       cfg.AccessKey,
		secretKey:       cfg.SecretKey,
		region:          region,
		useSSL:          cfg.UseSSL || u.Scheme == "https",
		pathStyle:       cfg.PathStyle,
		publicURLPrefix: strings.TrimRight(cfg.PublicURLPrefix, "/"),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	// Try ensuring bucket exists asynchronously or on startup (best effort)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.ensureBucket(ctx)
	}()

	return s, nil
}

func (s *S3Storage) Driver() string {
	return "s3"
}

// buildURL constructs the target URL for a given object key.
func (s *S3Storage) buildURL(key string) string {
	cleanKey := strings.TrimLeft(key, "/")
	if s.pathStyle {
		return fmt.Sprintf("%s://%s/%s/%s", s.endpoint.Scheme, s.endpoint.Host, s.bucket, cleanKey)
	}
	return fmt.Sprintf("%s://%s.%s/%s", s.endpoint.Scheme, s.bucket, s.endpoint.Host, cleanKey)
}

// ensureBucket checks if bucket exists, creates it if missing.
func (s *S3Storage) ensureBucket(ctx context.Context) error {
	bucketURL := fmt.Sprintf("%s://%s/%s", s.endpoint.Scheme, s.endpoint.Host, s.bucket)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, bucketURL, nil)
	if err != nil {
		return err
	}
	s.signRequest(req, []byte{})
	resp, err := s.client.Do(req)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return nil
		}
	}

	// If head failed or returned 404, try creating bucket
	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, bucketURL, nil)
	if err != nil {
		return err
	}
	s.signRequest(putReq, []byte{})
	putResp, err := s.client.Do(putReq)
	if err != nil {
		return err
	}
	defer putResp.Body.Close()
	return nil
}

// Put uploads an artifact stream to S3/RustFS.
func (s *S3Storage) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	var bodyBytes []byte
	var err error
	if size > 0 && size <= 64*1024*1024 {
		bodyBytes, err = io.ReadAll(r)
		if err != nil {
			return fmt.Errorf("read artifact stream error: %w", err)
		}
	} else {
		bodyBytes, err = io.ReadAll(r)
		if err != nil {
			return fmt.Errorf("read artifact stream error: %w", err)
		}
	}

	targetURL := s.buildURL(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}

	if contentType == "" {
		contentType = "application/octet-stream"
	}
	req.Header.Set("Content-Type", contentType)

	s.signRequest(req, bodyBytes)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("S3/RustFS PUT error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// Get downloads an artifact stream from S3/RustFS.
func (s *S3Storage) Get(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	targetURL := s.buildURL(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, 0, err
	}

	s.signRequest(req, []byte{})

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return nil, 0, os.ErrNotExist
		}
		return nil, 0, fmt.Errorf("S3/RustFS GET error (HTTP %d)", resp.StatusCode)
	}

	return resp.Body, resp.ContentLength, nil
}



// Delete removes an artifact from S3/RustFS.
func (s *S3Storage) Delete(ctx context.Context, key string) error {
	targetURL := s.buildURL(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, targetURL, nil)
	if err != nil {
		return err
	}

	s.signRequest(req, []byte{})

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("S3/RustFS DELETE error (HTTP %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// Exists checks if an artifact exists in S3/RustFS.
func (s *S3Storage) Exists(ctx context.Context, key string) (bool, error) {
	targetURL := s.buildURL(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, targetURL, nil)
	if err != nil {
		return false, err
	}

	s.signRequest(req, []byte{})

	resp, err := s.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, fmt.Errorf("S3/RustFS HEAD returned HTTP %d", resp.StatusCode)
}
