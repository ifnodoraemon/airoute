package controlplane

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

type supplierCostRequest struct {
	Channel    string  `json:"channel"`
	Model      string  `json:"model"`
	InputCost  float64 `json:"input_cost"`  // CNY per 1M prompt tokens
	OutputCost float64 `json:"output_cost"` // CNY per 1M completion tokens
	Status     string  `json:"status"`
}

type supplierCostView struct {
	ID         int64   `json:"id"`
	Channel    string  `json:"channel"`
	Model      string  `json:"model"`
	InputCost  float64 `json:"input_cost"`
	OutputCost float64 `json:"output_cost"`
	Status     string  `json:"status"`
}

func toSupplierCostView(rec *storage.SupplierCostRecord) supplierCostView {
	return supplierCostView{
		ID: rec.ID, Channel: rec.Channel, Model: rec.Model,
		InputCost:  storage.FromUnits(rec.InputCostUnitsPerMillion),
		OutputCost: storage.FromUnits(rec.OutputCostUnitsPerMillion),
		Status:     rec.Status,
	}
}

// ListSupplierCosts returns the configured supplier prices.
func (h *AdminHandler) ListSupplierCosts(c *gin.Context) {
	records, err := h.repo.ListSupplierCosts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "读取供应商成本失败: " + err.Error()})
		return
	}
	views := make([]supplierCostView, 0, len(records))
	for _, rec := range records {
		views = append(views, toSupplierCostView(rec))
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": views})
}

// SaveSupplierCost upserts one (channel, model) price.
func (h *AdminHandler) SaveSupplierCost(c *gin.Context) {
	var req supplierCostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "无效的成本请求: " + err.Error()})
		return
	}
	if !validSupplierRate(req.InputCost) || !validSupplierRate(req.OutputCost) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "成本单价必须在 0 至 1000000000 之间"})
		return
	}
	rec := &storage.SupplierCostRecord{
		Channel:                   strings.TrimSpace(req.Channel),
		Model:                     strings.TrimSpace(req.Model),
		InputCostUnitsPerMillion:  storage.ToUnits(req.InputCost),
		OutputCostUnitsPerMillion: storage.ToUnits(req.OutputCost),
		Status:                    strings.TrimSpace(req.Status),
	}
	if err := h.repo.UpsertSupplierCost(rec); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "保存供应商成本失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": toSupplierCostView(rec)})
}

// DeleteSupplierCost removes a supplier price.
func (h *AdminHandler) DeleteSupplierCost(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "无效的成本 ID"})
		return
	}
	if err := h.repo.DeleteSupplierCost(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "error": "供应商成本不存在"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": id}})
}
