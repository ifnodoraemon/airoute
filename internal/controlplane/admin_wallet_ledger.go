package controlplane

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

type walletLedgerView struct {
	ID                int64     `json:"id"`
	UserID            int64     `json:"user_id"`
	Username          string    `json:"username,omitempty"`
	Type              string    `json:"type"`
	Delta             float64   `json:"delta"`
	DeltaUnits        int64     `json:"delta_units"`
	BalanceAfter      float64   `json:"balance_after"`
	BalanceAfterUnits int64     `json:"balance_after_units"`
	Operator          string    `json:"operator,omitempty"`
	Reason            string    `json:"reason,omitempty"`
	RefID             string    `json:"ref_id,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

func toWalletLedgerView(rec *storage.WalletTransactionWithUser) walletLedgerView {
	return walletLedgerView{
		ID: rec.ID, UserID: rec.UserID, Username: rec.Username, Type: rec.Type,
		Delta: storage.FromUnits(rec.DeltaUnits), DeltaUnits: rec.DeltaUnits,
		BalanceAfter: storage.FromUnits(rec.BalanceAfterUnits), BalanceAfterUnits: rec.BalanceAfterUnits,
		Operator: rec.Operator, Reason: rec.Reason, RefID: rec.RefID, CreatedAt: rec.CreatedAt,
	}
}

func walletLedgerLimit(c *gin.Context) (int, int) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	return limit, offset
}

// ListUserWalletTransactions returns the caller's own ledger rows.
func (h *AdminHandler) ListUserWalletTransactions(c *gin.Context) {
	claims, ok := RequireAuthClaims(c)
	if !ok {
		return
	}
	user, err := h.repo.GetUserByUsername(claims.Username)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "error": "用户不存在"})
		return
	}
	limit, offset := walletLedgerLimit(c)
	rows, err := h.repo.ListWalletTransactionsAdmin(user.ID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "读取钱包流水失败: " + err.Error()})
		return
	}
	views := make([]walletLedgerView, 0, len(rows))
	for _, row := range rows {
		views = append(views, toWalletLedgerView(row))
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": views})
}

// ListWalletTransactions returns ledger rows for the admin console, filtered by
// an optional user_id.
func (h *AdminHandler) ListWalletTransactions(c *gin.Context) {
	userID, _ := strconv.ParseInt(strings.TrimSpace(c.Query("user_id")), 10, 64)
	limit, offset := walletLedgerLimit(c)
	rows, err := h.repo.ListWalletTransactionsAdmin(userID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "读取钱包流水失败: " + err.Error()})
		return
	}
	views := make([]walletLedgerView, 0, len(rows))
	for _, row := range rows {
		views = append(views, toWalletLedgerView(row))
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": views})
}

// ExportWalletTransactions streams the filtered ledger as CSV.
func (h *AdminHandler) ExportWalletTransactions(c *gin.Context) {
	userID, _ := strconv.ParseInt(strings.TrimSpace(c.Query("user_id")), 10, 64)
	rows, err := h.repo.ListWalletTransactionsForExport(userID, 10000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "导出钱包流水失败: " + err.Error()})
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=wallet-transactions-%s.csv", time.Now().Format("20060102-150405")))
	writer := csv.NewWriter(c.Writer)
	// BOM so Excel opens UTF-8 Chinese reasons correctly.
	_, _ = c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	_ = writer.Write([]string{"id", "user_id", "username", "type", "delta_cny", "balance_after_cny", "operator", "reason", "ref_id", "created_at"})
	for _, row := range rows {
		_ = writer.Write([]string{
			strconv.FormatInt(row.ID, 10),
			strconv.FormatInt(row.UserID, 10),
			row.Username,
			row.Type,
			fmt.Sprintf("%.4f", storage.FromUnits(row.DeltaUnits)),
			fmt.Sprintf("%.4f", storage.FromUnits(row.BalanceAfterUnits)),
			row.Operator,
			row.Reason,
			row.RefID,
			row.CreatedAt.Format(time.RFC3339),
		})
	}
	writer.Flush()
}
