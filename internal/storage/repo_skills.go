package storage

import (
	"encoding/json"
	"strings"
	"time"
)

// SkillRecord represents an Agent Skill capability.
type SkillRecord struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Tools       []string `json:"tools"`
	LoadingMode string   `json:"loading_mode"` // "lazy" (progressive disclosure) or "eager" (instant)
	Manifest    string   `json:"manifest"`     // Markdown SKILL.md specification
	Author      string   `json:"author"`
	Version     string   `json:"version"`
	Enabled     bool     `json:"enabled"`
	UpdatedAt   string   `json:"updated_at"`
}

// ListSkills returns all registered Agent Skills.
func (r *Repository) ListSkills() ([]*SkillRecord, error) {
	_ = r.SeedDefaultSkills()

	rows, err := r.db.Query(`SELECT id, name, description, category, tools, loading_mode, manifest, author, version, enabled, updated_at FROM system_skills ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*SkillRecord
	for rows.Next() {
		var s SkillRecord
		var toolsStr string
		var enabledInt int
		var updatedAt time.Time
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.Category, &toolsStr, &s.LoadingMode, &s.Manifest, &s.Author, &s.Version, &enabledInt, &updatedAt); err != nil {
			return nil, err
		}
		s.Enabled = enabledInt == 1
		s.UpdatedAt = updatedAt.Format("2006-01-02 15:04:05")
		_ = json.Unmarshal([]byte(toolsStr), &s.Tools)
		list = append(list, &s)
	}
	return list, nil
}

// GetSkill retrieves a specific skill by ID.
func (r *Repository) GetSkill(id string) (*SkillRecord, error) {
	_ = r.SeedDefaultSkills()

	var s SkillRecord
	var toolsStr string
	var enabledInt int
	var updatedAt time.Time
	err := r.db.QueryRow(`SELECT id, name, description, category, tools, loading_mode, manifest, author, version, enabled, updated_at FROM system_skills WHERE id = ?`, id).
		Scan(&s.ID, &s.Name, &s.Description, &s.Category, &toolsStr, &s.LoadingMode, &s.Manifest, &s.Author, &s.Version, &enabledInt, &updatedAt)
	if err != nil {
		return nil, err
	}
	s.Enabled = enabledInt == 1
	s.UpdatedAt = updatedAt.Format("2006-01-02 15:04:05")
	_ = json.Unmarshal([]byte(toolsStr), &s.Tools)
	return &s, nil
}

// SetSkillEnabled toggles a skill's enabled state on-demand.
func (r *Repository) SetSkillEnabled(id string, enabled bool) error {
	_ = r.SeedDefaultSkills()
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	_, err := r.db.Exec(`UPDATE system_skills SET enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, enabledInt, id)
	return err
}

// SaveSkill creates or updates an Agent Skill.
func (r *Repository) SaveSkill(s *SkillRecord) error {
	toolsJSON, _ := json.Marshal(s.Tools)
	enabledInt := 0
	if s.Enabled {
		enabledInt = 1
	}
	if s.LoadingMode == "" {
		s.LoadingMode = "lazy"
	}
	if s.Author == "" {
		s.Author = "Custom"
	}
	if s.Version == "" {
		s.Version = "1.0.0"
	}

	_, err := r.db.Exec(`
		INSERT INTO system_skills (id, name, description, category, tools, loading_mode, manifest, author, version, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			category = excluded.category,
			tools = excluded.tools,
			loading_mode = excluded.loading_mode,
			manifest = excluded.manifest,
			author = excluded.author,
			version = excluded.version,
			enabled = excluded.enabled,
			updated_at = CURRENT_TIMESTAMP
	`, s.ID, s.Name, s.Description, s.Category, string(toolsJSON), s.LoadingMode, s.Manifest, s.Author, s.Version, enabledInt)
	return err
}

// DeleteSkill deletes an Agent Skill.
func (r *Repository) DeleteSkill(id string) error {
	_, err := r.db.Exec(`DELETE FROM system_skills WHERE id = ?`, id)
	return err
}

// IsSkillEnabled checks if a skill is active.
func (r *Repository) IsSkillEnabled(id string) bool {
	_ = r.SeedDefaultSkills()
	var enabledInt int
	err := r.db.QueryRow(`SELECT enabled FROM system_skills WHERE id = ?`, id).Scan(&enabledInt)
	if err != nil {
		return true
	}
	return enabledInt == 1
}

// IsToolEnabled checks if any enabled skill contains this tool.
func (r *Repository) IsToolEnabled(toolName string) bool {
	if strings.HasPrefix(toolName, "airoute_search_skills") ||
		strings.HasPrefix(toolName, "airoute_discover_skills") ||
		strings.HasPrefix(toolName, "airoute_inspect_skill") ||
		strings.HasPrefix(toolName, "airoute_get_skill_manifest") ||
		strings.HasPrefix(toolName, "nano_") {
		return true
	}
	skills, err := r.ListSkills()
	if err != nil {
		return true
	}
	for _, s := range skills {
		for _, t := range s.Tools {
			if t == toolName {
				return s.Enabled
			}
		}
	}
	return true
}
