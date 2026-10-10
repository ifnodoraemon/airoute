package controlplane

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// ListLogs returns filtered invocation audit records.
func (h *AdminHandler) ListLogs(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "50")
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 {
		limit = 50
	} else if limit > 1000 {
		limit = 1000
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
	}

	if claims, ok := GetClaimsFromContext(c); ok && claims.Role != "admin" {
		// Fail-closed scoping: even if the user lookup fails, an empty
		// key list must never widen to full (admin) visibility — the
		// repository returns no rows when scope is set and keys are empty.
		filter.ScopeByAPIKeys = true
		user, _ := h.repo.GetUserByUsername(claims.Username)
		if user != nil {
			userKeys, _ := h.repo.ListAPIKeysByUser(user.ID)
			// Scope to the user's own keys in SQL so that LIMIT/OFFSET
			// pagination applies after filtering, not before.
			filter.APIKeys = make([]string, 0, len(userKeys))
			for _, k := range userKeys {
				filter.APIKeys = append(filter.APIKeys, k.Key)
			}
		}
		logs, err := h.repo.ListUsageLogsWithFilter(filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if logs == nil {
			logs = make([]*storage.UsageLogRecord, 0)
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": logs})
		return
	}

	logs, err := h.repo.ListUsageLogsWithFilter(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if logs == nil {
		logs = make([]*storage.UsageLogRecord, 0)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": logs})
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
	n, err := h.repo.BatchDeleteUsageLogs(ids)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": fmt.Sprintf("已批量删除 %d 条调用日志", n), "deleted_count": n})
}

// ClearLogs clears all audit logs (admin only).
func (h *AdminHandler) ClearLogs(c *gin.Context) {
	_, ok := RequireAdminClaims(c, "权限不足，仅超级管理员可清空审计日志")
	if !ok {
		return
	}

	if err := h.repo.ClearAllUsageLogs(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "已清空所有调用日志"})
}
