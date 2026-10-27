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

// budgetRequest is the console wire shape. Money is expressed in CNY and
// converted to the integer 1e-9 ledger internally; amount_units is not exposed
// so callers cannot accidentally send a mixed unit.
type budgetRequest struct {
	ScopeType   string  `json:"scope_type"`
	ScopeID     int64   `json:"scope_id"`
	Period      string  `json:"period"`
	Amount      float64 `json:"amount"`
	Action      string  `json:"action"`
	WarnPercent *int    `json:"warn_percent"`
	Status      string  `json:"status"`
}

type budgetView struct {
	ID          int64   `json:"id"`
	ScopeType   string  `json:"scope_type"`
	ScopeID     int64   `json:"scope_id"`
	Period      string  `json:"period"`
	Amount      float64 `json:"amount"`
	Used        float64 `json:"used"`
	Percent     float64 `json:"percent"`
	Action      string  `json:"action"`
	WarnPercent int     `json:"warn_percent"`
	Status      string  `json:"status"`
}

func toBudgetView(rec *storage.BudgetRecord) budgetView {
	amount := storage.FromUnits(rec.AmountUnits)
	used := storage.FromUnits(rec.UsedUnits)
	percent := 0.0
	if rec.AmountUnits > 0 {
		percent = float64(rec.UsedUnits) / float64(rec.AmountUnits) * 100
	}
	return budgetView{
		ID: rec.ID, ScopeType: rec.ScopeType, ScopeID: rec.ScopeID, Period: rec.Period,
		Amount: amount, Used: used, Percent: percent,
		Action: rec.Action, WarnPercent: rec.WarnPercent, Status: rec.Status,
	}
}

func parseBudgetID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "无效的预算 ID"})
		return 0, false
	}
	return id, true
}

// ListBudgets returns configured budgets; optional scope_type/scope_id filters
// let the console open one organization's tab without loading every rule.
func (h *AdminHandler) ListBudgets(c *gin.Context) {
	scopeType := strings.ToLower(strings.TrimSpace(c.Query("scope_type")))
	scopeID, _ := strconv.ParseInt(strings.TrimSpace(c.Query("scope_id")), 10, 64)
	records, err := h.repo.ListBudgets(scopeType, scopeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "读取预算失败: " + err.Error()})
		return
	}
	views := make([]budgetView, 0, len(records))
	for _, rec := range records {
		views = append(views, toBudgetView(rec))
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": views})
}

// SaveBudget creates or updates the rule for one (scope, period).
func (h *AdminHandler) SaveBudget(c *gin.Context) {
	var req budgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "无效的预算请求: " + err.Error()})
		return
	}
	req.ScopeType = strings.ToLower(strings.TrimSpace(req.ScopeType))
	req.Period = strings.ToLower(strings.TrimSpace(req.Period))
	req.Action = strings.ToLower(strings.TrimSpace(req.Action))
	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	if req.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "预算金额必须大于 0"})
		return
	}
	if req.WarnPercent == nil {
		value := 80
		req.WarnPercent = &value
	}
	if err := h.validateBudgetScope(req.ScopeType, req.ScopeID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": err.Error()})
		return
	}
	rec := &storage.BudgetRecord{
		ScopeType:   req.ScopeType,
		ScopeID:     req.ScopeID,
		Period:      req.Period,
		AmountUnits: storage.ToUnits(req.Amount),
		Action:      req.Action,
		WarnPercent: *req.WarnPercent,
		Status:      req.Status,
	}
	if err := h.repo.UpsertBudget(rec); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "保存预算失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": toBudgetView(rec)})
}

// UpdateBudget changes the amount/action/warn/status of an existing rule; the
// scope and period are immutable so an edit cannot silently move a budget to
// another tenant.
func (h *AdminHandler) UpdateBudget(c *gin.Context) {
	id, ok := parseBudgetID(c)
	if !ok {
		return
	}
	existing, err := h.repo.GetBudget(id)
	if err != nil {
		respondBudgetError(c, err)
		return
	}
	var req budgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "无效的预算请求: " + err.Error()})
		return
	}
	if req.Amount > 0 {
		existing.AmountUnits = storage.ToUnits(req.Amount)
	}
	if action := strings.ToLower(strings.TrimSpace(req.Action)); action != "" {
		existing.Action = action
	}
	if req.WarnPercent != nil {
		existing.WarnPercent = *req.WarnPercent
	}
	if status := strings.ToLower(strings.TrimSpace(req.Status)); status != "" {
		existing.Status = status
	}
	if err := h.repo.UpsertBudget(existing); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "更新预算失败: " + err.Error()})
		return
	}
	stored, err := h.repo.GetBudget(existing.ID)
	if err != nil {
		respondBudgetError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": toBudgetView(stored)})
}

// DeleteBudget removes a rule. Historical events stay in budget_events.
func (h *AdminHandler) DeleteBudget(c *gin.Context) {
	id, ok := parseBudgetID(c)
	if !ok {
		return
	}
	if err := h.repo.DeleteBudget(id); err != nil {
		respondBudgetError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": id}})
}

// ListBudgetEvents returns the newest threshold events for a rule.
func (h *AdminHandler) ListBudgetEvents(c *gin.Context) {
	id, ok := parseBudgetID(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	events, err := h.repo.ListBudgetEvents(id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "读取预算事件失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": events})
}

func (h *AdminHandler) validateBudgetScope(scopeType string, scopeID int64) error {
	if scopeID <= 0 {
		return errors.New("预算作用域 ID 必填")
	}
	switch scopeType {
	case "org":
		if _, err := h.repo.GetOrg(scopeID); err != nil {
			return errors.New("组织不存在")
		}
	case "user":
		if _, err := h.repo.GetUserByID(scopeID); err != nil {
			return errors.New("用户不存在")
		}
	case "key":
		exists, err := h.repo.APIKeyExists(scopeID)
		if err != nil || !exists {
			return errors.New("密钥不存在")
		}
	default:
		return errors.New("预算作用域必须是 org、user 或 key")
	}
	return nil
}

func respondBudgetError(c *gin.Context, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "error": "预算不存在"})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": err.Error()})
}
