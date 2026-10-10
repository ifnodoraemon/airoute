package controlplane

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

var (
	// Secure shared secret across cluster replicas for stateless HMAC token verification.
	// Reads GATEWAY_ADMIN_SECRET or NANO_SECRET_KEY, falling back to a cryptographically secure random secret.
	adminSecret = func() []byte {
		sec := os.Getenv("GATEWAY_ADMIN_SECRET")
		if sec == "" {
			sec = os.Getenv("NANO_SECRET_KEY")
		}
		if sec == "" {
			b := make([]byte, 32)
			if _, err := rand.Read(b); err == nil {
				sec = hex.EncodeToString(b)
			} else {
				sec = fmt.Sprintf("airoute-rnd-%d", time.Now().UnixNano())
			}
		}
		return []byte(sec)
	}()
)

// AdminClaims holds stateless token payload.
type AdminClaims struct {
	Username  string `json:"sub"`
	Role      string `json:"role"`
	ExpiresAt int64  `json:"exp"`
}

// GenerateAdminToken creates an HMAC-SHA256 signed stateless token.
func GenerateAdminToken(username, role string, duration time.Duration) (string, error) {
	claims := AdminClaims{
		Username:  username,
		Role:      role,
		ExpiresAt: time.Now().Add(duration).Unix(),
	}
	data, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(data)
	mac := hmac.New(sha256.New, adminSecret)
	mac.Write([]byte(payloadB64))
	sigHex := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%s.%s", payloadB64, sigHex), nil
}

// VerifyAdminToken parses and validates the token signature and expiration.
func VerifyAdminToken(token string) (*AdminClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, errors.New("无效的 Token 格式")
	}
	payloadB64, sigHex := parts[0], parts[1]

	mac := hmac.New(sha256.New, adminSecret)
	mac.Write([]byte(payloadB64))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sigHex), []byte(expectedSig)) {
		return nil, errors.New("Token 签名验证失败")
	}

	data, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, errors.New("Token 载荷损坏")
	}

	var claims AdminClaims
	if err := json.Unmarshal(data, &claims); err != nil {
		return nil, errors.New("Token 格式解析错误")
	}

	if time.Now().Unix() > claims.ExpiresAt {
		return nil, errors.New("登录凭证已过期")
	}

	return &claims, nil
}
