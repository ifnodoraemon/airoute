package controlplane

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// These handlers are HTTP adapters only: they bind and validate transport input,
// delegate to ChannelService for the business rules, and map the result back to
// a status code. Anything about what a channel *is* lives in channel_service.go.

// writeChannelError maps a ChannelService error onto an HTTP response.
func writeChannelError(c *gin.Context, err error, fallback string) {
	var verr *ValidationError
	switch {
	case errors.As(err, &verr):
		c.JSON(http.StatusBadRequest, gin.H{"error": verr.Message})
	case errors.Is(err, ErrChannelNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到指定渠道"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": fallback + ": " + err.Error()})
	}
}

// parseChannelID reads the :id path parameter, writing a 400 when it is not a
// number.
func parseChannelID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的渠道 ID"})
		return 0, false
	}
	return id, true
}

// ListChannels returns all configured channels.
func (h *AdminHandler) ListChannels(c *gin.Context) {
	channels, err := h.channels.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取渠道列表失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": channels})
}

// ChannelRequest is the wire shape of a channel create or update.
//
// It is deliberately separate from storage.ChannelRecord. Binding the row
// straight onto the request body made every storage change an API change, and it
// let a client post id, breaker_status, created_at and updated_at: columns this
// path never reads, but which the API appeared to accept.
//
// The accepted set is exactly the columns CreateChannel and UpdateChannel write.
// TestSavePricingRateRequestAcceptsExactlyTheWritableColumns covers the same
// property for pricing; TestChannelRequestAcceptsExactlyTheWritableColumns
// covers this one.
type ChannelRequest struct {
	Name           string             `json:"name"`
	Type           model.ProviderType `json:"type"`
	BaseURL        string             `json:"base_url"`
	APIKey         string             `json:"api_key"`
	Models         []string           `json:"models"`
	ModelMapping   map[string]string  `json:"model_mapping"`
	Protocols      []string           `json:"protocols"`
	Priority       int                `json:"priority"`
	Weight         int                `json:"weight"`
	TimeoutSeconds int                `json:"timeout_seconds"`
	Status         string             `json:"status"`
}

// toRecord converts the request into the row the repository persists.
func (r ChannelRequest) toRecord() *storage.ChannelRecord {
	return &storage.ChannelRecord{
		Name:           r.Name,
		Type:           r.Type,
		BaseURL:        r.BaseURL,
		APIKey:         r.APIKey,
		Models:         r.Models,
		ModelMapping:   r.ModelMapping,
		Protocols:      r.Protocols,
		Priority:       r.Priority,
		Weight:         r.Weight,
		TimeoutSeconds: r.TimeoutSeconds,
		Status:         r.Status,
	}
}

// CreateChannel creates a new channel.
func (h *AdminHandler) CreateChannel(c *gin.Context) {
	var req ChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	created, err := h.channels.Create(c.Request.Context(), req.toRecord())
	if err != nil {
		writeChannelError(c, err, "创建渠道失败")
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": created, "message": "渠道已创建并成功同步至数据面内存"})
}

// UpdateChannel updates an existing channel.
func (h *AdminHandler) UpdateChannel(c *gin.Context) {
	id, ok := parseChannelID(c)
	if !ok {
		return
	}

	var req ChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updated, err := h.channels.Update(c.Request.Context(), id, req.toRecord())
	if err != nil {
		writeChannelError(c, err, "更新渠道失败")
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": updated, "message": "渠道配置已成功更新"})
}

// DeleteChannel removes a channel.
func (h *AdminHandler) DeleteChannel(c *gin.Context) {
	id, ok := parseChannelID(c)
	if !ok {
		return
	}

	if err := h.channels.Delete(c.Request.Context(), id); err != nil {
		writeChannelError(c, err, "删除渠道失败")
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "渠道已成功删除"})
}

// BatchDeleteChannels deletes multiple channels.
func (h *AdminHandler) BatchDeleteChannels(c *gin.Context) {
	ids, ok := h.bindBatchIDs(c, "请提供有效渠道 ID 列表")
	if !ok {
		return
	}
	n, err := h.channels.BatchDelete(c.Request.Context(), ids)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": fmt.Sprintf("已批量删除 %d 个服务商渠道", n), "deleted_count": n})
}

// BatchStatusChannels toggles status for multiple channels.
func (h *AdminHandler) BatchStatusChannels(c *gin.Context) {
	req, ok := h.bindBatchStatus(c)
	if !ok {
		return
	}
	n, err := h.channels.BatchStatus(c.Request.Context(), req.IDs, req.Status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	action := "启用"
	if req.Status == "disabled" {
		action = "停用"
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": fmt.Sprintf("已批量%s %d 个服务商渠道", action, n)})
}
