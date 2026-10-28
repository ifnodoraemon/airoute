package controlplane

import (
	"database/sql"
	"encoding/csv"
	"errors"
	"io"
	"math"
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

// ExportSupplierCosts streams the price list as CSV.
func (h *AdminHandler) ExportSupplierCosts(c *gin.Context) {
	records, err := h.repo.ListSupplierCosts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "导出供应商成本失败: " + err.Error()})
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=supplier-costs.csv")
	_, _ = c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(c.Writer)
	_ = writer.Write([]string{"channel", "model", "input_cost", "output_cost", "status"})
	for _, rec := range records {
		_ = writer.Write([]string{
			rec.Channel, rec.Model,
			strconv.FormatFloat(storage.FromUnits(rec.InputCostUnitsPerMillion), 'f', 4, 64),
			strconv.FormatFloat(storage.FromUnits(rec.OutputCostUnitsPerMillion), 'f', 4, 64),
			rec.Status,
		})
	}
	writer.Flush()
}

// ImportSupplierCosts accepts a CSV body (channel,model,input_cost,output_cost,
// status). dry_run=1 validates every row and reports errors without writing.
func (h *AdminHandler) ImportSupplierCosts(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 2<<20))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "读取 CSV 失败: " + err.Error()})
		return
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\ufeff")))
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "解析 CSV 失败: " + err.Error()})
		return
	}
	if len(rows) > 5001 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "单次最多导入 5000 行"})
		return
	}
	dryRun := c.Query("dry_run") == "1"
	records := make([]*storage.SupplierCostRecord, 0, len(rows))
	type rowError struct {
		Row   int    `json:"row"`
		Error string `json:"error"`
	}
	errorsList := make([]rowError, 0)
	for index, row := range rows {
		line := index + 1
		if len(row) == 0 || (len(row) == 1 && strings.TrimSpace(row[0]) == "") {
			continue
		}
		if index == 0 && strings.EqualFold(strings.TrimSpace(row[0]), "channel") {
			continue
		}
		if len(row) < 4 {
			errorsList = append(errorsList, rowError{Row: line, Error: "需要 channel,model,input_cost,output_cost 四列"})
			continue
		}
		input, err := strconv.ParseFloat(strings.TrimSpace(row[2]), 64)
		if err != nil || math.IsNaN(input) || math.IsInf(input, 0) {
			errorsList = append(errorsList, rowError{Row: line, Error: "input_cost 不是有效数字"})
			continue
		}
		output, err := strconv.ParseFloat(strings.TrimSpace(row[3]), 64)
		if err != nil || math.IsNaN(output) || math.IsInf(output, 0) {
			errorsList = append(errorsList, rowError{Row: line, Error: "output_cost 不是有效数字"})
			continue
		}
		status := "active"
		if len(row) >= 5 && strings.TrimSpace(row[4]) != "" {
			status = strings.TrimSpace(row[4])
		}
		rec := &storage.SupplierCostRecord{
			Channel: strings.TrimSpace(row[0]), Model: strings.TrimSpace(row[1]),
			InputCostUnitsPerMillion: storage.ToUnits(input), OutputCostUnitsPerMillion: storage.ToUnits(output),
			Status: status,
		}
		if err := storage.ValidateSupplierCost(rec); err != nil {
			errorsList = append(errorsList, rowError{Row: line, Error: err.Error()})
			continue
		}
		records = append(records, rec)
	}
	if len(errorsList) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "CSV 存在无效行", "data": gin.H{"errors": errorsList}})
		return
	}
	if dryRun {
		views := make([]supplierCostView, 0, len(records))
		for _, rec := range records {
			views = append(views, toSupplierCostView(rec))
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"dry_run": true, "valid": len(records), "items": views}})
		return
	}
	for _, rec := range records {
		if err := h.repo.UpsertSupplierCost(rec); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "导入失败: " + err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"imported": len(records)}})
}
