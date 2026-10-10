package storage

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"
)

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
