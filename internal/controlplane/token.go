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
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

var (
	adminSecretMu sync.RWMutex
	// Secure shared secret across cluster replicas for stateless HMAC token verification.
	// Reads GATEWAY_ADMIN_SECRET, falling back to a cryptographically secure random secret.
	adminSecret = func() []byte {
		sec := os.Getenv("GATEWAY_ADMIN_SECRET")
		if sec == "" {
			b := make([]byte, 32)
			if _, err := rand.Read(b); err == nil {
				sec = hex.EncodeToString(b)
			} else {
				panic("secure randomness unavailable: " + err.Error())
			}
		}
		return []byte(sec)
	}()
)

func getAdminSecret() []byte {
	adminSecretMu.RLock()
	defer adminSecretMu.RUnlock()
	return adminSecret
}

// SetAdminSecret updates the runtime HMAC signing secret.
func SetAdminSecret(sec []byte) {
	if len(sec) == 0 {
		return
	}
	adminSecretMu.Lock()
	defer adminSecretMu.Unlock()
	adminSecret = sec
}

// EnsureAdminSecret ensures that cluster replicas share a single stable HMAC secret.
// If GATEWAY_ADMIN_SECRET is not provided, it attempts to load or generate a persisted
// secret from the system_settings table.
func EnsureAdminSecret(repo *storage.Repository) {
	if os.Getenv("GATEWAY_ADMIN_SECRET") != "" || repo == nil {
		return
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("secure randomness unavailable: " + err.Error())
	}
	secret, err := repo.GetOrCreateSetting("admin_jwt_secret", hex.EncodeToString(b))
	if err != nil {
		panic("cannot initialize shared session secret: " + err.Error())
	}
	if secret == "" {
		panic("shared session secret is empty")
	}
	SetAdminSecret([]byte(secret))
}

// AdminClaims holds stateless token payload.
type AdminClaims struct {
	Username      string `json:"sub"`
	Role          string `json:"role"`
	ExpiresAt     int64  `json:"exp"`
	CredentialTag string `json:"credentials"`
}

// GenerateAdminToken creates an HMAC-SHA256 signed stateless token.
func GenerateAdminToken(user *storage.UserRecord, duration time.Duration) (string, error) {
	if user == nil || user.ID <= 0 {
		return "", errors.New("必须提供持久化账号")
	}
	claims := AdminClaims{
		Username:      user.Username,
		Role:          user.Role,
		CredentialTag: sessionCredentialTag(user),
		ExpiresAt:     time.Now().Add(duration).Unix(),
	}
	data, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(data)
	mac := hmac.New(sha256.New, getAdminSecret())
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

	mac := hmac.New(sha256.New, getAdminSecret())
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

// sessionCredentialTag is opaque; no password hash is exposed in the token.
func sessionCredentialTag(user *storage.UserRecord) string {
	mac := hmac.New(sha256.New, getAdminSecret())
	fmt.Fprintf(mac, "airoute-session-credentials-v1|%d|%s", user.ID, user.PasswordHash)
	return hex.EncodeToString(mac.Sum(nil))
}
