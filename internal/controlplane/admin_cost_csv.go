package controlplane

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"math/big"
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
	_ = writer.Write(supplierCSVColumns)
	for _, rec := range records {
		_ = writer.Write([]string{sanitizeCSVCell(rec.Channel), sanitizeCSVCell(rec.Model),
			supplierTokenRateCSV(rec, true), supplierTokenRateCSV(rec, false), rec.Status,
			rec.Currency, rec.BillingMode, optionalSupplierRateCSV(rec.CacheReadCostUnitsPerMillion), optionalSupplierRateCSV(rec.CacheWriteCostUnitsPerMillion), optionalSupplierRateCSV(rec.FixedCostUnits)})
	}
	writer.Flush()
}

var supplierCSVColumns = []string{"channel", "model", "input_cost", "output_cost", "status", "currency", "billing_mode", "cache_read_cost", "cache_write_cost", "fixed_cost"}

func optionalSupplierRateCSV(units *int64) string {
	if units == nil {
		return ""
	}
	return supplierRateCSV(*units)
}
func supplierTokenRateCSV(rec *storage.SupplierCostRecord, input bool) string {
	if rec.BillingMode == "request" {
		return ""
	}
	if input {
		return supplierRateCSV(rec.InputCostUnitsPerMillion)
	}
	return supplierRateCSV(rec.OutputCostUnitsPerMillion)
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
	var header []string
	for index, row := range rows {
		if index == 0 && len(row) > 0 && strings.EqualFold(strings.TrimSpace(row[0]), "channel") {
			header = make([]string, len(row))
			seen := map[string]bool{}
			for i, name := range row {
				name = strings.ToLower(strings.TrimSpace(name))
				valid := false
				for _, known := range supplierCSVColumns {
					if name == known {
						valid = true
						break
					}
				}
				if !valid || seen[name] {
					return nil, nil, fmt.Errorf("未知或重复的 CSV 列: %s", name)
				}
				seen[name], header[i] = true, name
			}
			if !seen["channel"] || !seen["model"] {
				return nil, nil, fmt.Errorf("CSV 必须包含 channel 和 model")
			}
			continue
		}
		if len(row) == 0 || (len(row) == 1 && strings.TrimSpace(row[0]) == "") {
			continue
		}
		if len(records)+len(rowErrors) >= 5000 {
			return nil, nil, fmt.Errorf("单次最多导入 5000 行")
		}
		var rec *storage.SupplierCostRecord
		var err error
		if header != nil {
			if len(row) != len(header) {
				err = fmt.Errorf("数据列数与表头不一致")
			} else {
				ordered := make([]string, len(supplierCSVColumns))
				for i, name := range supplierCSVColumns {
					for j, column := range header {
						if name == column {
							ordered[i] = row[j]
							break
						}
					}
				}
				rec, err = parseSupplierCSVRow(ordered)
			}
		} else {
			rec, err = parseSupplierCSVRow(row)
		}
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
	if len(row) != 4 && len(row) != 5 && len(row) != len(supplierCSVColumns) {
		return nil, fmt.Errorf("需要旧版 4/5 列，或含币种、计费方式、缓存及按次报价的完整表头")
	}
	values := make([]string, len(supplierCSVColumns))
	copy(values, row)
	mode := strings.ToLower(strings.TrimSpace(values[6]))
	if mode == "" {
		mode = "token"
	}
	rec := &storage.SupplierCostRecord{Channel: restoreCSVCell(strings.TrimSpace(values[0])), Model: restoreCSVCell(strings.TrimSpace(values[1])), Status: values[4], Currency: values[5], BillingMode: mode}
	for _, field := range []struct {
		index  int
		target **int64
	}{
		{7, &rec.CacheReadCostUnitsPerMillion}, {8, &rec.CacheWriteCostUnitsPerMillion}, {9, &rec.FixedCostUnits},
	} {
		value, err := parseSupplierCSVRate(values[field.index], supplierCSVColumns[field.index], mode == "request" && field.index == 9)
		if err != nil {
			return nil, err
		}
		*field.target = value
	}
	input, err := parseSupplierCSVRate(values[2], "input_cost", mode == "token")
	if err != nil {
		return nil, err
	}
	output, err := parseSupplierCSVRate(values[3], "output_cost", mode == "token")
	if err != nil {
		return nil, err
	}
	if input != nil {
		rec.InputCostUnitsPerMillion = *input
	}
	if output != nil {
		rec.OutputCostUnitsPerMillion = *output
	}
	if err := storage.ValidateSupplierCost(rec); err != nil {
		return nil, err
	}
	return rec, nil
}
func parseSupplierCSVRate(raw, name string, required bool) (*int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" && !required {
		return nil, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || !validSupplierRate(value) {
		return nil, fmt.Errorf("%s 必须为 0 至 1000000000 之间的有效数字；未知报价请留空，免费请明确填写 0", name)
	}
	// CSV exports contain exact decimal units. Parsing through float64 loses
	// the last digits of larger rates, so scale the original decimal instead.
	if len(raw) > 128 {
		return nil, fmt.Errorf("%s 数字过长", name)
	}
	if value < 0.5/float64(storage.UnitsPerCurrency) {
		zero := int64(0)
		return &zero, nil
	}
	decimal, ok := new(big.Rat).SetString(raw)
	if !ok || decimal.Sign() < 0 || decimal.Cmp(big.NewRat(1_000_000_000, 1)) > 0 {
		return nil, fmt.Errorf("%s 超出报价范围", name)
	}
	decimal.Mul(decimal, big.NewRat(storage.UnitsPerCurrency, 1))
	whole, remainder := new(big.Int), new(big.Int)
	whole.QuoRem(decimal.Num(), decimal.Denom(), remainder)
	if remainder.Lsh(remainder, 1).Cmp(decimal.Denom()) >= 0 {
		whole.Add(whole, big.NewInt(1))
	}
	units := whole.Int64()
	return &units, nil
}

func validSupplierRate(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1_000_000_000
}
