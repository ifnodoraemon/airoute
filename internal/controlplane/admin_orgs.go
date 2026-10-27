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

// orgRequest is the wire shape for creating/updating an organization. It is
// deliberately separate from storage.OrgRecord so id/path/depth/timestamps are
// never client-writable.
type orgRequest struct {
	ParentID int64  `json:"parent_id"`
	Level    string `json:"level"`
	Name     string `json:"name"`
	Code     string `json:"code"`
	Status   string `json:"status"`
}

type orgMoveRequest struct {
	ParentID int64 `json:"parent_id"`
}

type orgMemberRequest struct {
	UserID int64  `json:"user_id"`
	Role   string `json:"role"`
}

func parseOrgID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "无效的组织 ID"})
		return 0, false
	}
	return id, true
}

// GetOrg returns one organization node.
func (h *AdminHandler) GetOrg(c *gin.Context) {
	id, ok := parseOrgID(c)
	if !ok {
		return
	}
	org, err := h.repo.GetOrg(id)
	if err != nil {
		respondOrgError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": org})
}

// ListOrgs returns the whole organization tree as a flat list ordered by
// materialized path; the console renders the hierarchy from parent_id.
func (h *AdminHandler) ListOrgs(c *gin.Context) {
	orgs, err := h.repo.ListOrgs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "读取组织列表失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": orgs})
}

// CreateOrg adds a unit/department/project node.
func (h *AdminHandler) CreateOrg(c *gin.Context) {
	var req orgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "无效的组织请求: " + err.Error()})
		return
	}
	org := &storage.OrgRecord{
		ParentID: req.ParentID,
		Level:    req.Level,
		Name:     req.Name,
		Code:     req.Code,
		Status:   req.Status,
	}
	if err := h.repo.CreateOrg(org); err != nil {
		respondOrgError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": org})
}

// UpdateOrg renames a node and/or archives it.
func (h *AdminHandler) UpdateOrg(c *gin.Context) {
	id, ok := parseOrgID(c)
	if !ok {
		return
	}
	var req orgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "无效的组织请求: " + err.Error()})
		return
	}
	if err := h.repo.UpdateOrg(id, req.Name, req.Status); err != nil {
		respondOrgError(c, err)
		return
	}
	org, err := h.repo.GetOrg(id)
	if err != nil {
		respondOrgError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": org})
}

// MoveOrg re-parents a department/project and rewrites its subtree paths.
func (h *AdminHandler) MoveOrg(c *gin.Context) {
	id, ok := parseOrgID(c)
	if !ok {
		return
	}
	var req orgMoveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "无效的移动请求: " + err.Error()})
		return
	}
	if err := h.repo.MoveOrg(id, req.ParentID); err != nil {
		respondOrgError(c, err)
		return
	}
	org, err := h.repo.GetOrg(id)
	if err != nil {
		respondOrgError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": org})
}

// ListOrgMembers returns the members of one organization.
func (h *AdminHandler) ListOrgMembers(c *gin.Context) {
	id, ok := parseOrgID(c)
	if !ok {
		return
	}
	members, err := h.repo.ListOrgMembers(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "读取组织成员失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": members})
}

// AddOrgMember adds or updates a member role.
func (h *AdminHandler) AddOrgMember(c *gin.Context) {
	id, ok := parseOrgID(c)
	if !ok {
		return
	}
	var req orgMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "无效的成员请求: " + err.Error()})
		return
	}
	if err := h.repo.AddOrgMember(id, req.UserID, req.Role); err != nil {
		respondOrgError(c, err)
		return
	}
	members, err := h.repo.ListOrgMembers(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "读取组织成员失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": members})
}

// RemoveOrgMember removes a membership.
func (h *AdminHandler) RemoveOrgMember(c *gin.Context) {
	id, ok := parseOrgID(c)
	if !ok {
		return
	}
	userID, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil || userID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "无效的用户 ID"})
		return
	}
	if err := h.repo.RemoveOrgMember(id, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "移除组织成员失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"org_id": id, "user_id": userID}})
}

// respondOrgError keeps the console contract consistent: a missing node is 404,
// everything else is a validation/conflict error (400/409).
func respondOrgError(c *gin.Context, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "error": "组织不存在"})
		return
	}
	status := http.StatusBadRequest
	if isOrgConflict(err) {
		status = http.StatusConflict
	}
	c.JSON(status, gin.H{"code": status, "error": err.Error()})
}

// isOrgConflict recognizes the duplicate-code unique violation without depending
// on one driver's error text.
func isOrgConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate") || strings.Contains(msg, "重复")
}
