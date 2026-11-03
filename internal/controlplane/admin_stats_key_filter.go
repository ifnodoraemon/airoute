package controlplane

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// Dashboard key drill-down resolution.
//
// Both parameters name the same thing — one API key — in the two shapes the
// console and scripted callers use, so they are resolved next to each other:
// `key_id` is the numeric row id, `key` is the legacy identifier.
//
// statsKeyFilter resolves the dashboard's legacy `key` drill-down parameter to
// the digest usage_logs actually stores.
//
// The parameter used to mean different things per role and per endpoint: a
// non-admin compared it byte-for-byte against its own digests (so only a raw
// digest worked), while the admin path handed it to a repository helper that
// hashed anything non-digest-shaped once — so a plaintext credential matched and
// an `ak_…` public id hashed into a digest that matched no row, silently showing
// zero traffic. Neither behaviour is acceptable, so the value is normalized in
// exactly one place before any handler uses it:
//
//   - `ak_…` (the identifier the console displays) resolves to its stored digest;
//   - a 64-hex digest is accepted as-is for existing scripted callers;
//   - anything else is a 400 instead of a plausible-looking zero.
//
// It returns (digest, present, ok). present reports whether the parameter was
// supplied; ok is false when a response has already been written.
func (h *AdminHandler) statsKeyFilter(c *gin.Context) (string, bool, bool) {
	raw := strings.TrimSpace(c.Query("key"))
	if raw == "" {
		return "", false, true
	}
	if storage.IsAPIKeyPublicID(raw) {
		digest, err := h.repo.APIKeyDigestByPublicID(raw)
		if err != nil || digest == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "无效的密钥引用: 未找到该密钥 ID"})
			return "", true, false
		}
		return digest, true, true
	}
	if storage.IsAPIKeyHash(raw) {
		return raw, true, true
	}
	// No silent re-hash: a plaintext credential or a display mask must not turn
	// into "this key had no traffic".
	c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "无效的密钥引用: 请使用 key_id（数字行 ID）或密钥公开标识（ak_…）"})
	return "", true, false
}

// statsKeyIDFilter resolves the dashboard's key drill-down filter.
//
// The console drills into a key by its numeric id (`key_id`), because the key
// list exposes the public identifier and never the stored digest. Resolving the
// id here, against the authoritative row, is what lets the
// repository be queried with the digest that usage_logs actually stores; the old
// `key=` filter was fed the masked value, which hashed to a digest that matched
// no row, so a drill-down showed zero traffic for a busy key.
//
// It returns (record, present, ok). present reports whether the parameter was
// supplied; ok is false when a response has already been written.
func (h *AdminHandler) statsKeyIDFilter(c *gin.Context) (*storage.APIKeyRecord, bool, bool) {
	raw := strings.TrimSpace(c.Query("key_id"))
	if raw == "" {
		return nil, false, true
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "无效的密钥 ID"})
		return nil, true, false
	}
	record, err := h.repo.GetAPIKey(id)
	if err != nil || record == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "error": "未找到指定 API 密钥"})
		return nil, true, false
	}
	return record, true, true
}
