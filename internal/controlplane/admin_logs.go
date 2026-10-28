package controlplane

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// ListLogs returns filtered invocation audit records.
func (h *AdminHandler) ListLogs(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "50")
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 {
		limit = 50
	} else if limit > storage.MaxUsageLogPageSize {
		// Clamp before the repository sees it, so the `limit` echoed below is the
		// size actually applied. The repository used to clamp silently, and the
		// handler replayed the requested size, so a client paging by it skipped
		// rows.
		limit = storage.MaxUsageLogPageSize
	}
	offsetStr := c.DefaultQuery("offset", "0")
	offset, _ := strconv.Atoi(offsetStr)
	if offset < 0 {
		offset = 0
	}

	filter := storage.LogFilter{
		Limit:     limit,
		Offset:    offset,
		StartTime: c.Query("start_time"),
		EndTime:   c.Query("end_time"),
		TraceID:   c.Query("trace_id"),
		SessionID: c.Query("session_id"),
		Model:     c.Query("model"),
		TenantID:  c.Query("tenant_id"),
		ChatID:    c.Query("chat_id"),
		// The console's search box and facet filters must be applied server-side;
		// filtering only the loaded page silently hides matches on other pages.
		Keyword: c.Query("q"),
		Channel: c.Query("channel"),
		Status:  c.Query("status"),
		Cache:   c.Query("cache"),
		Latency: c.Query("latency"),
	}
	if keyID := c.Query("key_id"); keyID != "" {
		id, err := strconv.ParseInt(keyID, 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的密钥 ID"})
			return
		}
		filter.APIKeyID = id
	}

	// Non-admin callers are scoped to their own API keys. Fail-closed: when the
	// user lookup fails, ScopeByAPIKeys stays set with an empty key list, and the
	// repository then returns no rows (never the full admin view).
	if claims, ok := GetClaimsFromContext(c); ok && claims.Role != "admin" {
		filter.ScopeByAPIKeys = true
		if user, _ := h.repo.GetUserByUsername(claims.Username); user != nil {
			userKeys, _ := h.repo.ListHistoricalAPIKeysByUser(user.ID)
			// Scope in SQL so LIMIT/OFFSET pagination applies after filtering.
			filter.APIKeys = make([]string, 0, len(userKeys))
			for _, k := range userKeys {
				filter.APIKeys = append(filter.APIKeys, k.Key)
			}
		}
	}

	if targetKey := strings.TrimSpace(c.Query("key")); targetKey != "" {
		// The console sends back the identifier it displays (ak_…). The audit
		// column stores the digest, so resolve the identifier before filtering.
		if storage.IsAPIKeyPublicID(targetKey) {
			if digest, err := h.repo.APIKeyDigestByPublicID(targetKey); err == nil && digest != "" {
				targetKey = digest
			}
		}
		if claims, ok := GetClaimsFromContext(c); ok && claims.Role != "admin" {
			allowed := false
			for _, k := range filter.APIKeys {
				if k == targetKey {
					allowed = true
					break
				}
			}
			if allowed {
				filter.APIKeys = []string{targetKey}
			} else {
				filter.APIKeys = []string{"__no_access__"}
			}
		} else {
			filter.ScopeByAPIKeys = true
			filter.APIKeys = []string{targetKey}
		}
	}

	// Aggregates are optional: the dashboard's 8s poll and the CSV export only
	// need rows, and the filtered COUNT plus the DISTINCT facets are the expensive
	// half of a page request.
	skipFacets := c.Query("facets") == "0"
	filter.SkipCount = c.Query("count") == "0"

	logs, total, err := h.repo.ListUsageLogsWithFilterAndCount(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if logs == nil {
		logs = make([]*storage.UsageLogRecord, 0)
	}
	h.presentLogKeys(logs, shouldRevealKey(c))
	// `total` is the size of the whole filtered result set, so the console can
	// paginate honestly instead of assuming the current page is everything.
	payload := gin.H{"code": 0, "data": logs, "limit": limit, "offset": offset}
	if !filter.SkipCount {
		payload["total"] = total
	}
	if !skipFacets {
		facets, err := h.repo.ListUsageLogFacets(filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		payload["facets"] = facets
	}
	c.JSON(http.StatusOK, payload)
}

// DeleteLog deletes a single log by ID (admin only).
func (h *AdminHandler) DeleteLog(c *gin.Context) {
	_, ok := RequireAdminClaims(c, "权限不足，仅超级管理员可删除审计日志")
	if !ok {
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的日志 ID"})
		return
	}
	if err := h.repo.DeleteUsageLog(id); err != nil {
		// A charged row is billing evidence: it cannot be deleted, only reversed.
		if errors.Is(err, storage.ErrChargedUsageLogImmutable) {
			c.JSON(http.StatusConflict, gin.H{"code": 409, "error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "已删除该条调用日志"})
}

// BatchDeleteLogs deletes multiple logs by IDs (admin only).
func (h *AdminHandler) BatchDeleteLogs(c *gin.Context) {
	_, ok := RequireAdminClaims(c, "权限不足，仅超级管理员可删除审计日志")
	if !ok {
		return
	}

	ids, ok := h.bindBatchIDs(c, "请提供有效的日志 ID 列表")
	if !ok {
		return
	}
	deleted, skipped, err := h.repo.BatchDeleteUsageLogs(ids)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	message := fmt.Sprintf("已批量删除 %d 条调用日志", deleted)
	if skipped > 0 {
		// The caller must learn why rows survived: they carry a charge.
		message += fmt.Sprintf("，跳过 %d 条已计费记录（不可删除，请走冲正）", skipped)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": message, "deleted_count": deleted, "skipped_charged_count": skipped})
}

// ClearLogs clears all audit logs (admin only).
func (h *AdminHandler) ClearLogs(c *gin.Context) {
	_, ok := RequireAdminClaims(c, "权限不足，仅超级管理员可清空审计日志")
	if !ok {
		return
	}

	deleted, skipped, err := h.repo.ClearAllUsageLogs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	message := fmt.Sprintf("已清空 %d 条未计费调用日志", deleted)
	if skipped > 0 {
		message += fmt.Sprintf("，保留 %d 条已计费记录（账单凭证，不可删除）", skipped)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": message, "deleted_count": deleted, "skipped_charged_count": skipped})
}

// shouldRevealKey reports whether the caller explicitly asked for unmasked keys.
// Only super-admins may do so, and the decision is per request.
func shouldRevealKey(c *gin.Context) bool {
	if c.Query("reveal_key") != "true" && c.Query("reveal_key") != "1" {
		return false
	}
	claims, ok := GetClaimsFromContext(c)
	return ok && claims.Role == "admin"
}

// maskAPIKey renders a credential as prefix + last 4 characters so audit logs
// never expose a complete key to the console or exported CSV. The single
// implementation lives in the storage package, next to the key digest helpers.
func maskAPIKey(key string) string {
	return storage.MaskAPIKey(key)
}

// presentLogKeys names each row's key the way operators identify it.
//
// The audit column holds the key digest (the ledger joins depend on it), but the
// console and the access log speak in public identifiers (ak_…). Resolving the
// digest here keeps the join intact while making the call log comparable with
// the key list. A digest that no longer resolves to a key row (legacy, hard
// deleted) stays hidden behind the mask unless a super-admin asked to reveal it.
func (h *AdminHandler) presentLogKeys(logs []*storage.UsageLogRecord, reveal bool) {
	if len(logs) == 0 {
		return
	}
	digests := make([]string, 0, len(logs))
	for _, l := range logs {
		if l != nil && l.APIKey != "" && !storage.IsAPIKeyPublicID(l.APIKey) {
			digests = append(digests, l.APIKey)
		}
	}
	ids, _ := h.repo.APIKeyPublicIDsByDigest(digests)
	for _, l := range logs {
		if l == nil || l.APIKey == "" || storage.IsAPIKeyPublicID(l.APIKey) {
			continue
		}
		if publicID, ok := ids[l.APIKey]; ok && publicID != "" {
			l.APIKey = publicID
			continue
		}
		if !reveal {
			l.APIKey = maskAPIKey(l.APIKey)
		}
	}
}
