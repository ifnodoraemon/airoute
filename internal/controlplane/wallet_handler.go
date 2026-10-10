package controlplane

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
)

// UpdateUserStatusRequest defines lock/unlock payload.
type UpdateUserStatusRequest struct {
	Status string `json:"status" binding:"required"` // "active" or "locked"
}

// UpdateUserStatus locks or unlocks a user account.
func (h *AdminHandler) UpdateUserStatus(c *gin.Context) {
	_, ok := RequireAdminClaims(c, "权限不足，仅管理员可锁定或解锁用户")
	if !ok {
		return
	}

	targetUsername := c.Param("username")
	rootAdmin := config.GetGlobalConfig().GetAdminUsername()
	if targetUsername == "admin" || targetUsername == rootAdmin {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "系统初始管理员账号不允许被锁定"})
		return
	}

	var req UpdateUserStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请提供有效状态: active 或 locked"})
		return
	}

	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status != "active" && status != "locked" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "状态仅支持 active 或 locked"})
		return
	}

	if err := h.repo.UpdateUserStatus(targetUsername, status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "更新用户状态失败: " + err.Error()})
		return
	}

	msg := fmt.Sprintf("已成功将用户 [%s] 设置为 %s", targetUsername, status)
	if status == "locked" {
		msg = fmt.Sprintf("已成功锁定用户 [%s]，该用户无法再调用 API 或登录", targetUsername)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": msg})
}

// UpdateUserBalanceRequest defines balance adjustment payload.
type UpdateUserBalanceRequest struct {
	Balance *float64 `json:"balance"` // exact balance
	Delta   *float64 `json:"delta"`   // delta balance
}

// UpdateUserBalance allows administrator to adjust a user's wallet quota/balance.
func (h *AdminHandler) UpdateUserBalance(c *gin.Context) {
	_, ok := RequireAdminClaims(c, "权限不足，仅管理员可调整用户额度")
	if !ok {
		return
	}

	targetUsername := c.Param("username")
	var req UpdateUserBalanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请提供调整额度参数 (balance 或 delta)"})
		return
	}

	if req.Balance != nil {
		if err := h.repo.SetUserBalance(targetUsername, *req.Balance); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "更新额度失败: " + err.Error()})
			return
		}
	} else if req.Delta != nil {
		if err := h.repo.UpdateUserBalance(targetUsername, *req.Delta); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "调整额度失败: " + err.Error()})
			return
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请提供 balance 或 delta 参数"})
		return
	}

	u, _ := h.repo.GetUserByUsername(targetUsername)
	var newBal float64
	if u != nil {
		newBal = u.Balance
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": fmt.Sprintf("已成功调整用户 [%s] 额度，当前余额: ¥%.2f", targetUsername, newBal),
		"data":    gin.H{"username": targetUsername, "balance": newBal},
	})
}

// UpdateUserGroupRequest defines pricing group change.
type UpdateUserGroupRequest struct {
	GroupName string `json:"group_name" binding:"required"`
}

// UpdateUserGroup switches user pricing group tier (default, vip, enterprise).
func (h *AdminHandler) UpdateUserGroup(c *gin.Context) {
	_, ok := RequireAdminClaims(c, "权限不足，仅管理员可设置用户分组")
	if !ok {
		return
	}

	targetUsername := c.Param("username")
	var req UpdateUserGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.GroupName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请指定有效的分组名称"})
		return
	}

	groupName := strings.ToLower(strings.TrimSpace(req.GroupName))
	if err := h.repo.UpdateUserGroup(targetUsername, groupName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "更新分组失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": fmt.Sprintf("已成功将用户 [%s] 分组切换为 [%s]", targetUsername, groupName),
	})
}

// UpdateUserRoleRequest defines role change payload.
type UpdateUserRoleRequest struct {
	Role string `json:"role" binding:"required"` // "admin" or "user"
}

// UpdateUserRole updates a user's system role ('admin' or 'user').
func (h *AdminHandler) UpdateUserRole(c *gin.Context) {
	claims, ok := RequireAdminClaims(c, "权限不足，仅超级管理员可修改角色")
	if !ok {
		return
	}

	targetUsername := c.Param("username")
	rootAdmin := config.GetGlobalConfig().GetAdminUsername()
	if (targetUsername == "admin" || targetUsername == rootAdmin) && claims.Username != targetUsername {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "error": "禁止修改内置超级管理员的角色"})
		return
	}

	var req UpdateUserRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请提供有效角色 (admin 或 user)"})
		return
	}

	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role != "admin" && role != "user" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "无效角色类型，仅支持 admin 或 user"})
		return
	}

	if targetUsername == claims.Username && role != "admin" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "不能将自己的管理员角色降级为普通用户"})
		return
	}

	targetUser, err := h.repo.GetUserByUsername(targetUsername)
	if err != nil || targetUser == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "error": "目标用户不存在"})
		return
	}

	if err := h.repo.UpdateUserRole(targetUsername, role); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "修改角色失败: " + err.Error()})
		return
	}

	roleLabel := "超级管理员"
	if role == "user" {
		roleLabel = "普通用户"
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": fmt.Sprintf("已成功将用户 [%s] 的角色变更为 [%s]", targetUsername, roleLabel),
	})
}

// GetUserWallet returns balance, status, group, role, and recent orders for current user.
func (h *AdminHandler) GetUserWallet(c *gin.Context) {
	claims, ok := RequireAuthClaims(c)
	if !ok {
		return
	}

	user, err := h.repo.GetUserByUsername(claims.Username)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "error": "用户不存在"})
		return
	}

	orders, _ := h.repo.ListRechargeOrders(user.Username)

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"username":   user.Username,
			"email":      user.Email,
			"role":       user.Role,
			"status":     user.Status,
			"balance":    user.Balance,
			"is_admin":   strings.EqualFold(user.Role, "admin"),
			"group_name": user.GroupName,
			"orders":     orders,
		},
	})
}
