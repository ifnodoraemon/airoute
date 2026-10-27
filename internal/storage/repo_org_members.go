package storage

import (
	"database/sql"
	"fmt"
	"strings"
)

// AddOrgMember adds or updates a user's role inside an organization.
func (r *Repository) AddOrgMember(orgID, userID int64, role string) error {
	if orgID <= 0 || userID <= 0 {
		return fmt.Errorf("organization and user are required")
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		role = "member"
	}
	if role != "admin" && role != "member" {
		return fmt.Errorf("invalid organization member role %q", role)
	}
	if _, err := r.GetOrg(orgID); err != nil {
		return fmt.Errorf("load organization: %w", err)
	}
	var exists int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ? AND status <> 'deleted'`, userID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return fmt.Errorf("user %d not found", userID)
	}
	_, err := r.db.Exec(`INSERT INTO org_memberships (org_id, user_id, role) VALUES (?, ?, ?)
		ON CONFLICT (org_id, user_id) DO UPDATE SET role = excluded.role`, orgID, userID, role)
	return err
}

// RemoveOrgMember removes a membership; a missing row is not an error.
func (r *Repository) RemoveOrgMember(orgID, userID int64) error {
	_, err := r.db.Exec(`DELETE FROM org_memberships WHERE org_id = ? AND user_id = ?`, orgID, userID)
	return err
}

// ListOrgMembers returns the members of one organization with display fields.
func (r *Repository) ListOrgMembers(orgID int64) ([]*OrgMemberRecord, error) {
	rows, err := r.db.Query(`SELECT m.org_id, m.user_id, COALESCE(u.username, ''), COALESCE(u.email, ''),
		m.role, m.created_at
		FROM org_memberships m LEFT JOIN users u ON u.id = m.user_id
		WHERE m.org_id = ? ORDER BY m.user_id ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]*OrgMemberRecord, 0)
	for rows.Next() {
		var member OrgMemberRecord
		if err := rows.Scan(&member.OrgID, &member.UserID, &member.Username, &member.Email,
			&member.Role, &member.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, &member)
	}
	return list, rows.Err()
}

// ListUserOrgs returns every organization a user belongs to.
func (r *Repository) ListUserOrgs(userID int64) ([]*OrgRecord, error) {
	rows, err := r.db.Query(`SELECT o.id, COALESCE(o.parent_id, 0), o.level, o.name, o.code, o.path,
		o.depth, o.status, o.created_at, o.updated_at
		FROM orgs o JOIN org_memberships m ON m.org_id = o.id
		WHERE m.user_id = ? ORDER BY o.path ASC, o.id ASC`, userID)
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

// OrgMemberRole returns a user's role in an organization, or sql.ErrNoRows when
// they are not a member.
func (r *Repository) OrgMemberRole(orgID, userID int64) (string, error) {
	var role string
	err := r.db.QueryRow(`SELECT role FROM org_memberships WHERE org_id = ? AND user_id = ?`, orgID, userID).Scan(&role)
	if err != nil {
		return "", err
	}
	if role == "" {
		return "", sql.ErrNoRows
	}
	return role, nil
}
