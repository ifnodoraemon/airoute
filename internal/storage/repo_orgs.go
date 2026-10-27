package storage

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// OrgLevel is the fixed three-level hierarchy used by the organization tree.
// Levels are ordered: a department belongs to a unit, a project to a department.
const (
	OrgLevelUnit       = "unit"
	OrgLevelDepartment = "department"
	OrgLevelProject    = "project"
)

var orgLevelRank = map[string]int{
	OrgLevelUnit:       0,
	OrgLevelDepartment: 1,
	OrgLevelProject:    2,
}

// OrgRecord is one node of the organization tree.
type OrgRecord struct {
	ID        int64     `json:"id"`
	ParentID  int64     `json:"parent_id"`
	Level     string    `json:"level"`
	Name      string    `json:"name"`
	Code      string    `json:"code"`
	Path      string    `json:"path"`  // materialized path, e.g. "/1/4/9/"
	Depth     int       `json:"depth"` // 0 for a unit
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OrgMemberRecord is one user's membership in an organization.
type OrgMemberRecord struct {
	OrgID     int64     `json:"org_id"`
	UserID    int64     `json:"user_id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Role      string    `json:"role"` // admin | member
	CreatedAt time.Time `json:"created_at"`
}

func validateOrg(org *OrgRecord) error {
	if org == nil {
		return fmt.Errorf("organization is required")
	}
	org.Level = strings.ToLower(strings.TrimSpace(org.Level))
	org.Name = strings.TrimSpace(org.Name)
	org.Code = strings.TrimSpace(org.Code)
	org.Status = strings.ToLower(strings.TrimSpace(org.Status))
	if _, ok := orgLevelRank[org.Level]; !ok {
		return fmt.Errorf("invalid organization level %q (want unit, department or project)", org.Level)
	}
	if org.Name == "" || org.Code == "" {
		return fmt.Errorf("organization name and code are required")
	}
	if org.Status == "" {
		org.Status = "active"
	}
	if org.Status != "active" && org.Status != "archived" {
		return fmt.Errorf("invalid organization status %q", org.Status)
	}
	return nil
}

func expectedParentLevel(level string) (string, bool) {
	switch level {
	case OrgLevelDepartment:
		return OrgLevelUnit, true
	case OrgLevelProject:
		return OrgLevelDepartment, true
	}
	return "", false
}

// CreateOrg inserts a node and computes its materialized path from its parent.
// A unit is a root (ParentID must be 0); a department needs a unit parent and a
// project needs a department parent.
func (r *Repository) CreateOrg(org *OrgRecord) error {
	if err := validateOrg(org); err != nil {
		return err
	}

	var parent *OrgRecord
	if org.Level == OrgLevelUnit {
		if org.ParentID != 0 {
			return fmt.Errorf("a unit is a root organization and cannot have a parent")
		}
	} else {
		if org.ParentID <= 0 {
			return fmt.Errorf("level %s requires a parent", org.Level)
		}
		found, err := r.GetOrg(org.ParentID)
		if err != nil {
			return fmt.Errorf("load parent organization: %w", err)
		}
		want, _ := expectedParentLevel(org.Level)
		if found.Level != want {
			return fmt.Errorf("a %s must belong to a %s (parent %d is %s)", org.Level, want, found.ID, found.Level)
		}
		if found.Status != "active" {
			return fmt.Errorf("parent organization %d is archived", found.ID)
		}
		parent = found
	}

	depth := 0
	if parent != nil {
		depth = parent.Depth + 1
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var parentID any
	if org.ParentID > 0 {
		parentID = org.ParentID
	}
	id, err := tx.InsertGetID(`INSERT INTO orgs (parent_id, level, name, code, path, depth, status)
		VALUES (?, ?, ?, ?, '', ?, ?)`, parentID, org.Level, org.Name, org.Code, depth, org.Status)
	if err != nil {
		return fmt.Errorf("insert organization: %w", err)
	}
	path := "/" + strconv.FormatInt(id, 10) + "/"
	if parent != nil {
		path = parent.Path + strconv.FormatInt(id, 10) + "/"
	}
	if _, err := tx.Exec(`UPDATE orgs SET path = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, path, id); err != nil {
		return fmt.Errorf("set organization path: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	org.ID = id
	org.Path = path
	org.Depth = depth
	return nil
}

// GetOrg loads one node by id.
func (r *Repository) GetOrg(id int64) (*OrgRecord, error) {
	row := r.db.QueryRow(`SELECT id, COALESCE(parent_id, 0), level, name, code, path, depth, status, created_at, updated_at
		FROM orgs WHERE id = ?`, id)
	var org OrgRecord
	if err := row.Scan(&org.ID, &org.ParentID, &org.Level, &org.Name, &org.Code, &org.Path,
		&org.Depth, &org.Status, &org.CreatedAt, &org.UpdatedAt); err != nil {
		return nil, err
	}
	return &org, nil
}

// GetOrgByCode resolves a stable business code (for example the "default" root).
func (r *Repository) GetOrgByCode(code string) (*OrgRecord, error) {
	var id int64
	if err := r.db.QueryRow(`SELECT id FROM orgs WHERE code = ?`, strings.TrimSpace(code)).Scan(&id); err != nil {
		return nil, err
	}
	return r.GetOrg(id)
}

// ListOrgs returns the whole tree ordered by materialized path (parents first).
func (r *Repository) ListOrgs() ([]*OrgRecord, error) {
	rows, err := r.db.Query(`SELECT id, COALESCE(parent_id, 0), level, name, code, path, depth, status, created_at, updated_at
		FROM orgs ORDER BY path ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]*OrgRecord, 0)
	for rows.Next() {
		var org OrgRecord
		if err := rows.Scan(&org.ID, &org.ParentID, &org.Level, &org.Name, &org.Code, &org.Path,
			&org.Depth, &org.Status, &org.CreatedAt, &org.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, &org)
	}
	return list, rows.Err()
}

// ListOrgChildren returns the direct children of a node (or root units when
// parentID is 0).
func (r *Repository) ListOrgChildren(parentID int64) ([]*OrgRecord, error) {
	query := `SELECT id, COALESCE(parent_id, 0), level, name, code, path, depth, status, created_at, updated_at
		FROM orgs WHERE ` + orgParentPredicate(parentID) + ` ORDER BY path ASC, id ASC`
	rows, err := r.db.Query(query, orgParentArgs(parentID)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]*OrgRecord, 0)
	for rows.Next() {
		var org OrgRecord
		if err := rows.Scan(&org.ID, &org.ParentID, &org.Level, &org.Name, &org.Code, &org.Path,
			&org.Depth, &org.Status, &org.CreatedAt, &org.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, &org)
	}
	return list, rows.Err()
}

func orgParentPredicate(parentID int64) string {
	if parentID <= 0 {
		return "parent_id IS NULL"
	}
	return "parent_id = ?"
}

func orgParentArgs(parentID int64) []any {
	if parentID <= 0 {
		return nil
	}
	return []any{parentID}
}

// UpdateOrg renames a node and/or changes its status. Empty values keep the
// current one.
func (r *Repository) UpdateOrg(id int64, name, status string) error {
	name = strings.TrimSpace(name)
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "" && status != "active" && status != "archived" {
		return fmt.Errorf("invalid organization status %q", status)
	}
	result, err := r.db.Exec(`UPDATE orgs SET
		name = COALESCE(NULLIF(?, ''), name),
		status = COALESCE(NULLIF(?, ''), status),
		updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, name, status, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// MoveOrg re-parents a department/project and rewrites the materialized paths
// and depths of its subtree in one transaction. A move never rewrites audit
// history: only future attribution uses the new tree.
func (r *Repository) MoveOrg(id, newParentID int64) error {
	org, err := r.GetOrg(id)
	if err != nil {
		return err
	}
	if org.Level == OrgLevelUnit {
		return fmt.Errorf("a root unit cannot be moved")
	}
	if newParentID <= 0 {
		return fmt.Errorf("a %s requires a parent", org.Level)
	}
	if org.ParentID == newParentID {
		return nil
	}
	parent, err := r.GetOrg(newParentID)
	if err != nil {
		return err
	}
	want, _ := expectedParentLevel(org.Level)
	if parent.Level != want {
		return fmt.Errorf("a %s must belong to a %s (new parent %d is %s)", org.Level, want, parent.ID, parent.Level)
	}
	if parent.Status != "active" {
		return fmt.Errorf("new parent organization %d is archived", parent.ID)
	}
	if parent.Path == org.Path || strings.HasPrefix(parent.Path, org.Path) {
		return fmt.Errorf("cannot move organization %d under its own descendant %d", id, parent.ID)
	}

	newPrefix := parent.Path + strconv.FormatInt(org.ID, 10) + "/"
	depthDelta := parent.Depth + 1 - org.Depth

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id, path, depth FROM orgs WHERE path LIKE ?`, org.Path+"%")
	if err != nil {
		return err
	}
	type subtreeNode struct {
		id    int64
		path  string
		depth int
	}
	var subtree []subtreeNode
	for rows.Next() {
		var node subtreeNode
		if err := rows.Scan(&node.id, &node.path, &node.depth); err != nil {
			rows.Close()
			return err
		}
		subtree = append(subtree, node)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, node := range subtree {
		remainder := strings.TrimPrefix(node.path, org.Path)
		if _, err := tx.Exec(`UPDATE orgs SET path = ?, depth = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			newPrefix+remainder, node.depth+depthDelta, node.id); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE orgs SET parent_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, newParentID, id); err != nil {
		return err
	}
	return tx.Commit()
}

// OrgAncestors returns the chain root -> self using the materialized path, which
// is what budget inheritance walks later.
func (r *Repository) OrgAncestors(id int64) ([]*OrgRecord, error) {
	org, err := r.GetOrg(id)
	if err != nil {
		return nil, err
	}
	segments := strings.Split(strings.Trim(org.Path, "/"), "/")
	chain := make([]*OrgRecord, 0, len(segments))
	for _, segment := range segments {
		nodeID, err := strconv.ParseInt(segment, 10, 64)
		if err != nil || nodeID <= 0 {
			continue
		}
		node, err := r.GetOrg(nodeID)
		if err != nil {
			return nil, err
		}
		chain = append(chain, node)
	}
	return chain, nil
}

// DefaultOrgID returns the root unit created by migration 30.
func (r *Repository) DefaultOrgID() (int64, error) {
	var id int64
	err := r.db.QueryRow(`SELECT id FROM orgs WHERE code = 'default' LIMIT 1`).Scan(&id)
	return id, err
}

// UserOrgID resolves a user's primary organization, falling back to the root
// unit for accounts created before the organization model existed.
func (r *Repository) UserOrgID(userID int64) (int64, error) {
	var id int64
	err := r.db.QueryRow(`SELECT COALESCE(
		NULLIF((SELECT org_id FROM users WHERE id = ?), 0),
		(SELECT id FROM orgs WHERE code = 'default' LIMIT 1))`, userID).Scan(&id)
	return id, err
}
