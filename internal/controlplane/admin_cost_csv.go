package controlplane

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// ExportSupplierCosts emits spreadsheet-safe identifiers and exact stored rates.
func (h *AdminHandler) ExportSupplierCosts(c *gin.Context) {
	records, err := h.repo.ListSupplierCosts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "导出供应商成本失败"})
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=supplier-costs.csv")
	_, _ = c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(c.Writer)
	_ = writer.Write([]string{"channel", "model", "input_cost", "output_cost", "status"})
	for _, rec := range records {
		_ = writer.Write([]string{sanitizeCSVCell(rec.Channel), sanitizeCSVCell(rec.Model),
			supplierRateCSV(rec.InputCostUnitsPerMillion), supplierRateCSV(rec.OutputCostUnitsPerMillion), rec.Status})
	}
	writer.Flush()
}

func supplierRateCSV(units int64) string {
	fraction := fmt.Sprintf("%09d", units%storage.UnitsPerCurrency)
	for len(fraction) > 4 && strings.HasSuffix(fraction, "0") {
		fraction = fraction[:len(fraction)-1]
	}
	return fmt.Sprintf("%d.%s", units/storage.UnitsPerCurrency, fraction)
}

func csvFormulaCell(value string) bool {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	return trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0]))
}

// Escape a leading quote too, making our spreadsheet escaping reversible on
// import. A literal '=text becomes ”=text rather than colliding with =text.
func sanitizeCSVCell(value string) string {
	if strings.HasPrefix(value, "'") || csvFormulaCell(value) {
		return "'" + value
	}
	return value
}

func restoreCSVCell(value string) string {
	if strings.HasPrefix(value, "'") && (strings.HasPrefix(value[1:], "'") || csvFormulaCell(value[1:])) {
		return value[1:]
	}
	return value
}

type supplierCSVError struct {
	Row   int    `json:"row"`
	Error string `json:"error"`
}

const supplierCSVMaxBytes = 2 << 20

// ImportSupplierCosts validates the whole file, then commits it as one unit.
func (h *AdminHandler) ImportSupplierCosts(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, supplierCSVMaxBytes+1))
	if err != nil || len(body) > supplierCSVMaxBytes {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "读取 CSV 失败或文件超过 2 MiB"})
		return
	}
	records, rowErrors, err := parseSupplierCSV(string(body))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": err.Error()})
		return
	}
	if len(rowErrors) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "CSV 存在无效行", "data": gin.H{"errors": rowErrors}})
		return
	}
	if c.Query("dry_run") == "1" {
		views := make([]supplierCostView, 0, len(records))
		for _, rec := range records {
			views = append(views, toSupplierCostView(rec))
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"dry_run": true, "valid": len(records), "items": views}})
		return
	}
	if err := h.repo.BatchUpsertSupplierCosts(c.Request.Context(), records); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "导入失败，本批次未写入，请重试"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"imported": len(records)}})
}

func parseSupplierCSV(body string) ([]*storage.SupplierCostRecord, []supplierCSVError, error) {
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(body, "\ufeff")))
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("解析 CSV 失败: %w", err)
	}
	records := make([]*storage.SupplierCostRecord, 0, len(rows))
	rowErrors := make([]supplierCSVError, 0)
	for index, row := range rows {
		if index == 0 && len(row) > 0 && strings.EqualFold(strings.TrimSpace(row[0]), "channel") {
			continue
		}
		if len(row) == 0 || (len(row) == 1 && strings.TrimSpace(row[0]) == "") {
			continue
		}
		if len(records)+len(rowErrors) >= 5000 {
			return nil, nil, fmt.Errorf("单次最多导入 5000 行")
		}
		rec, err := parseSupplierCSVRow(row)
		if err != nil {
			rowErrors = append(rowErrors, supplierCSVError{Row: index + 1, Error: err.Error()})
			continue
		}
		records = append(records, rec)
	}
	if len(records) == 0 && len(rowErrors) == 0 {
		return nil, nil, fmt.Errorf("CSV 没有可导入的数据行")
	}
	return records, rowErrors, nil
}

func parseSupplierCSVRow(row []string) (*storage.SupplierCostRecord, error) {
	if len(row) < 4 || len(row) > 5 {
		return nil, fmt.Errorf("需要 channel,model,input_cost,output_cost 和可选的 status 列")
	}
	input, err := strconv.ParseFloat(strings.TrimSpace(row[2]), 64)
	if err != nil || !validSupplierRate(input) {
		return nil, fmt.Errorf("input_cost 必须为 0 至 1000000000 之间的有效数字")
	}
	output, err := strconv.ParseFloat(strings.TrimSpace(row[3]), 64)
	if err != nil || !validSupplierRate(output) {
		return nil, fmt.Errorf("output_cost 必须为 0 至 1000000000 之间的有效数字")
	}
	status := "active"
	if len(row) == 5 && strings.TrimSpace(row[4]) != "" {
		status = strings.TrimSpace(row[4])
	}
	rec := &storage.SupplierCostRecord{Channel: restoreCSVCell(strings.TrimSpace(row[0])), Model: restoreCSVCell(strings.TrimSpace(row[1])),
		InputCostUnitsPerMillion: storage.ToUnits(input), OutputCostUnitsPerMillion: storage.ToUnits(output), Status: status}
	if err := storage.ValidateSupplierCost(rec); err != nil {
		return nil, err
	}
	return rec, nil
}

func validSupplierRate(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1_000_000_000
}
