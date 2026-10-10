package storage

import (
	"encoding/json"
	"time"
)

// MCPServerRecord represents an MCP server registered in the gateway plaza.
type MCPServerRecord struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`  // ops, dev, search, db, productivity, storage
	Transport   string   `json:"transport"` // sse, stdio, http
	Endpoint    string   `json:"endpoint"`
	Status      string   `json:"status"`    // online, active, standby
	Author      string   `json:"author"`
	Version     string   `json:"version"`
	Tools       []string `json:"tools"`
	Prompts     []string `json:"prompts"`
	Resources   []string `json:"resources"`
	EnvVars     string   `json:"env_vars,omitempty"`
	Enabled     bool     `json:"enabled"`
	CreatedAt   string   `json:"created_at,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
}

// ListMCPServers returns all registered MCP Servers.
func (r *Repository) ListMCPServers() ([]*MCPServerRecord, error) {
	_ = r.SeedDefaultMCPServers()

	rows, err := r.db.Query(`SELECT id, name, description, category, transport, endpoint, status, author, version, tools, prompts, resources, env_vars, enabled, updated_at FROM system_mcp_servers ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*MCPServerRecord
	for rows.Next() {
		var s MCPServerRecord
		var toolsStr, promptsStr, resourcesStr string
		var enabledInt int
		var updatedAt time.Time
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.Category, &s.Transport, &s.Endpoint, &s.Status, &s.Author, &s.Version, &toolsStr, &promptsStr, &resourcesStr, &s.EnvVars, &enabledInt, &updatedAt); err != nil {
			return nil, err
		}
		s.Enabled = enabledInt == 1
		s.UpdatedAt = updatedAt.Format("2006-01-02 15:04:05")
		_ = json.Unmarshal([]byte(toolsStr), &s.Tools)
		_ = json.Unmarshal([]byte(promptsStr), &s.Prompts)
		_ = json.Unmarshal([]byte(resourcesStr), &s.Resources)
		list = append(list, &s)
	}
	return list, nil
}

// GetMCPServer retrieves an MCP server by ID.
func (r *Repository) GetMCPServer(id string) (*MCPServerRecord, error) {
	_ = r.SeedDefaultMCPServers()

	var s MCPServerRecord
	var toolsStr, promptsStr, resourcesStr string
	var enabledInt int
	var updatedAt time.Time
	err := r.db.QueryRow(`SELECT id, name, description, category, transport, endpoint, status, author, version, tools, prompts, resources, env_vars, enabled, updated_at FROM system_mcp_servers WHERE id = ?`, id).
		Scan(&s.ID, &s.Name, &s.Description, &s.Category, &s.Transport, &s.Endpoint, &s.Status, &s.Author, &s.Version, &toolsStr, &promptsStr, &resourcesStr, &s.EnvVars, &enabledInt, &updatedAt)
	if err != nil {
		return nil, err
	}
	s.Enabled = enabledInt == 1
	s.UpdatedAt = updatedAt.Format("2006-01-02 15:04:05")
	_ = json.Unmarshal([]byte(toolsStr), &s.Tools)
	_ = json.Unmarshal([]byte(promptsStr), &s.Prompts)
	_ = json.Unmarshal([]byte(resourcesStr), &s.Resources)
	return &s, nil
}

// SaveMCPServer saves or updates an MCP Server.
func (r *Repository) SaveMCPServer(s *MCPServerRecord) error {
	toolsJSON, _ := json.Marshal(s.Tools)
	promptsJSON, _ := json.Marshal(s.Prompts)
	resourcesJSON, _ := json.Marshal(s.Resources)
	enabledInt := 0
	if s.Enabled {
		enabledInt = 1
	}
	if s.Status == "" {
		s.Status = "online"
	}
	if s.Version == "" {
		s.Version = "1.0.0"
	}
	if s.Author == "" {
		s.Author = "Custom"
	}

	_, err := r.db.Exec(`
		INSERT INTO system_mcp_servers (id, name, description, category, transport, endpoint, status, author, version, tools, prompts, resources, env_vars, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			category = excluded.category,
			transport = excluded.transport,
			endpoint = excluded.endpoint,
			status = excluded.status,
			author = excluded.author,
			version = excluded.version,
			tools = excluded.tools,
			prompts = excluded.prompts,
			resources = excluded.resources,
			env_vars = excluded.env_vars,
			enabled = excluded.enabled,
			updated_at = CURRENT_TIMESTAMP
	`, s.ID, s.Name, s.Description, s.Category, s.Transport, s.Endpoint, s.Status, s.Author, s.Version, string(toolsJSON), string(promptsJSON), string(resourcesJSON), s.EnvVars, enabledInt)
	return err
}

// SetMCPServerEnabled toggles an MCP server's enabled state.
func (r *Repository) SetMCPServerEnabled(id string, enabled bool) error {
	_ = r.SeedDefaultMCPServers()
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	_, err := r.db.Exec(`UPDATE system_mcp_servers SET enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, enabledInt, id)
	return err
}

// DeleteMCPServer removes an MCP server.
func (r *Repository) DeleteMCPServer(id string) error {
	_, err := r.db.Exec(`DELETE FROM system_mcp_servers WHERE id = ?`, id)
	return err
}
