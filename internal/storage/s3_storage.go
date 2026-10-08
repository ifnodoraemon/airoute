package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
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
		endpointStr = "http://localhost:9000"
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
	if b, ok := r.(*bytes.Buffer); ok {
		bodyBytes = b.Bytes()
	} else {
		bodyBytes, err = io.ReadAll(r)
		if err != nil {
			return fmt.Errorf("failed to read data for S3 upload: %w", err)
		}
	}

	targetURL := s.buildURL(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to build S3 PUT request: %w", err)
	}

	if contentType == "" {
		contentType = "application/octet-stream"
	}
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = int64(len(bodyBytes))

	s.signRequest(req, bodyBytes)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("S3/RustFS PUT failed: %w", err)
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
		return nil, 0, fmt.Errorf("failed to build S3 GET request: %w", err)
	}

	s.signRequest(req, []byte{})

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("S3/RustFS GET failed: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, 0, fmt.Errorf("artifact not found in S3/RustFS: %s", key)
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, 0, fmt.Errorf("S3/RustFS GET error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	return resp.Body, resp.ContentLength, nil
}

// GetDownloadURL generates an AWS SigV4 presigned GET URL for offloading download bandwidth.
func (s *S3Storage) GetDownloadURL(ctx context.Context, key string, expire time.Duration) (string, error) {
	if expire <= 0 {
		expire = 15 * time.Minute
	}
	if expire > 7*24*time.Hour {
		expire = 7 * 24 * time.Hour
	}

	targetURL := s.buildURL(key)
	u, err := url.Parse(targetURL)
	if err != nil {
		return "", err
	}

	now := time.Now().UTC()
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")
	credScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, s.region)
	credParam := fmt.Sprintf("%s/%s", s.accessKey, credScope)

	q := u.Query()
	q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	q.Set("X-Amz-Credential", credParam)
	q.Set("X-Amz-Date", amzDate)
	q.Set("X-Amz-Expires", fmt.Sprintf("%d", int(expire.Seconds())))
	q.Set("X-Amz-SignedHeaders", "host")

	// Canonical Query String
	canonicalQuery := s.buildCanonicalQuery(q)

	// Canonical Request
	canonicalURI := u.Path
	if !strings.HasPrefix(canonicalURI, "/") {
		canonicalURI = "/" + canonicalURI
	}
	canonicalHeaders := fmt.Sprintf("host:%s\n", u.Host)
	signedHeaders := "host"
	payloadHash := "UNSIGNED-PAYLOAD"

	canonicalReq := fmt.Sprintf("GET\n%s\n%s\n%s\n%s\n%s",
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	)

	// String to Sign
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s",
		amzDate,
		credScope,
		sha256Hex([]byte(canonicalReq)),
	)

	// Signature calculation
	signingKey := s.deriveSigningKey(dateStamp)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	finalURL := fmt.Sprintf("%s://%s%s?%s&X-Amz-Signature=%s",
		u.Scheme, u.Host, u.Path, canonicalQuery, signature)

	// If a public external URL prefix is configured, swap host/scheme
	if s.publicURLPrefix != "" {
		finalURL = fmt.Sprintf("%s%s?%s&X-Amz-Signature=%s",
			s.publicURLPrefix, u.Path, canonicalQuery, signature)
	}

	return finalURL, nil
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

// signRequest applies AWS SigV4 Authorization headers to an outgoing HTTP request.
func (s *S3Storage) signRequest(req *http.Request, payload []byte) {
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	payloadHash := sha256Hex(payload)
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)
	if req.Header.Get("Host") == "" {
		req.Header.Set("Host", req.URL.Host)
	}

	// Canonical headers (host, x-amz-content-sha256, x-amz-date)
	var headerKeys []string
	headerMap := make(map[string]string)
	for k, v := range req.Header {
		lowerK := strings.ToLower(k)
		if lowerK == "host" || strings.HasPrefix(lowerK, "x-amz-") || lowerK == "content-type" {
			headerKeys = append(headerKeys, lowerK)
			headerMap[lowerK] = strings.TrimSpace(strings.Join(v, ","))
		}
	}
	sort.Strings(headerKeys)

	var canonicalHeaders strings.Builder
	for _, k := range headerKeys {
		canonicalHeaders.WriteString(fmt.Sprintf("%s:%s\n", k, headerMap[k]))
	}
	signedHeaders := strings.Join(headerKeys, ";")

	canonicalURI := req.URL.Path
	if !strings.HasPrefix(canonicalURI, "/") {
		canonicalURI = "/" + canonicalURI
	}

	canonicalQuery := s.buildCanonicalQuery(req.URL.Query())

	canonicalReq := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		req.Method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders.String(),
		signedHeaders,
		payloadHash,
	)

	credScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, s.region)
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s",
		amzDate,
		credScope,
		sha256Hex([]byte(canonicalReq)),
	)

	signingKey := s.deriveSigningKey(dateStamp)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		s.accessKey, credScope, signedHeaders, signature)
	req.Header.Set("Authorization", authHeader)
}

func (s *S3Storage) deriveSigningKey(dateStamp string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+s.secretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(s.region))
	kService := hmacSHA256(kRegion, []byte("s3"))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	return kSigning
}

func (s *S3Storage) buildCanonicalQuery(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	var keys []string
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf strings.Builder
	for i, k := range keys {
		escapedK := url.QueryEscape(k)
		vals := values[k]
		sort.Strings(vals)
		for j, v := range vals {
			if i > 0 || j > 0 {
				buf.WriteByte('&')
			}
			buf.WriteString(escapedK)
			buf.WriteByte('=')
			buf.WriteString(url.QueryEscape(v))
		}
	}
	return buf.String()
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
