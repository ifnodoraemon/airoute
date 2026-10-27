package controlplane

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/costing"
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
	if req.InputCost < 0 || req.OutputCost < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "成本单价不能为负"})
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
	if engine := costing.Global(); engine != nil {
		engine.Invalidate()
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": toSupplierCostView(rec)})
}

// DeleteSupplierCost removes a price and invalidates the lookup cache.
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
	if engine := costing.Global(); engine != nil {
		engine.Invalidate()
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": id}})
}

type costOverviewView struct {
	Requests            int64   `json:"requests"`
	Revenue             float64 `json:"revenue"`
	Cost                float64 `json:"cost"`
	Margin              float64 `json:"margin"`
	MarginPercent       float64 `json:"margin_percent"`
	BilledUnits         int64   `json:"billed_units"`
	CostBasisUnits      int64   `json:"cost_basis_units"`
	KnownCostRequests   int64   `json:"known_cost_requests"`
	UnknownCostRequests int64   `json:"unknown_cost_requests"`
}

// GetCostOverview returns revenue, supplier cost and margin for a range.
func (h *AdminHandler) GetCostOverview(c *gin.Context) {
	summary, err := h.repo.CostSummary(c.Query("start"), c.Query("end"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "读取成本概览失败: " + err.Error()})
		return
	}
	revenue := storage.FromUnits(summary.BilledUnits)
	cost := storage.FromUnits(summary.CostBasisUnits)
	margin := revenue - cost
	percent := 0.0
	if revenue != 0 {
		percent = margin / revenue * 100
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": costOverviewView{
		Requests: summary.Requests, Revenue: revenue, Cost: cost, Margin: margin, MarginPercent: percent,
		BilledUnits: summary.BilledUnits, CostBasisUnits: summary.CostBasisUnits,
		KnownCostRequests: summary.KnownCostRequests, UnknownCostRequests: summary.UnknownCostRequests,
	}})
}

type costGroupView struct {
	Key               string  `json:"key"`
	Label             string  `json:"label,omitempty"`
	Requests          int64   `json:"requests"`
	Revenue           float64 `json:"revenue"`
	Cost              float64 `json:"cost"`
	Margin            float64 `json:"margin"`
	MarginPercent     float64 `json:"margin_percent"`
	KnownCostRequests int64   `json:"known_cost_requests"`
}

// GetCostBreakdown groups margin by org, channel or model.
func (h *AdminHandler) GetCostBreakdown(c *gin.Context) {
	start, end := c.Query("start"), c.Query("end")
	var groups []storage.CostGroup
	var err error
	switch strings.ToLower(strings.TrimSpace(c.DefaultQuery("group", "org"))) {
	case "channel":
		groups, err = h.repo.CostByChannel(start, end)
	case "model":
		groups, err = h.repo.CostByModel(start, end)
	case "org", "":
		groups, err = h.repo.CostByOrg(start, end)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "group 必须是 org、channel 或 model"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "读取成本分组失败: " + err.Error()})
		return
	}
	views := make([]costGroupView, 0, len(groups))
	for _, group := range groups {
		revenue := storage.FromUnits(group.BilledUnits)
		cost := storage.FromUnits(group.CostBasisUnits)
		margin := revenue - cost
		percent := 0.0
		if revenue != 0 {
			percent = margin / revenue * 100
		}
		views = append(views, costGroupView{
			Key: group.Key, Label: group.Label, Requests: group.Requests,
			Revenue: revenue, Cost: cost, Margin: margin, MarginPercent: percent,
			KnownCostRequests: group.KnownCostRequests,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": views})
}
