package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

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
