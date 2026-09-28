package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"golang.org/x/crypto/bcrypt"
)

// ChannelRecord represents the database row for channels.
type ChannelRecord struct {
	ID             int64              `json:"id"`
	Name           string             `json:"name"`
	Type           model.ProviderType `json:"type"`
	BaseURL        string             `json:"base_url"`
	APIKey         string             `json:"api_key"`
	Models         []string           `json:"models"`
	ModelMapping   map[string]string  `json:"model_mapping"`
	Protocols      []string           `json:"protocols,omitempty"`
	Priority       int                `json:"priority"`
	Weight         int                `json:"weight"`
	TimeoutSeconds int                `json:"timeout_seconds"`
	Status         string             `json:"status"` // active, inactive
	BreakerStatus  string             `json:"breaker_status,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

// VirtualKeyRecord represents the database row for virtual keys.
type VirtualKeyRecord struct {
	ID            int64     `json:"id"`
	Key           string    `json:"key"`
	TenantID      string    `json:"tenant_id"`
	AllowedModels []string  `json:"allowed_models"`
	RPM           int       `json:"rpm"`
	TPM           int       `json:"tpm"`
	Budget        float64   `json:"budget"`
	UsedTokens    int64     `json:"used_tokens"`
	UsedCost      float64   `json:"used_cost"`
	GroupName     string    `json:"group_name"`
	UserID        int64     `json:"user_id"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// UsageLogRecord represents an audit log entry.
type UsageLogRecord struct {
	ID               int64     `json:"id"`
	TraceID          string    `json:"trace_id"`
	SessionID        string    `json:"session_id,omitempty"`
	VirtualKey       string    `json:"virtual_key"`
	TenantID         string    `json:"tenant_id"`
	Model            string    `json:"model"`
	Channel          string    `json:"channel"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	CachedTokens     int       `json:"cached_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	Cost             float64   `json:"cost"`
	IsOffPeak        bool      `json:"is_off_peak"`
	OffPeakDiscount  float64   `json:"off_peak_discount"`
	DurationMs       int64     `json:"duration_ms"`
	TTFTMs           int64     `json:"ttft_ms"`
	StatusCode       int       `json:"status_code"`
	CreatedAt        time.Time `json:"created_at"`
}

// ModelPriceRecord defines rates for a model.
type ModelPriceRecord struct {
	ID              int64     `json:"id"`
	Model           string    `json:"model"`
	GroupName       string    `json:"group_name"`       // "default", "vip", "enterprise", etc.
	PromptPrice     float64   `json:"prompt_price"`     // per 1,000,000 prompt tokens (CNY/USD)
	CompletionPrice float64   `json:"completion_price"` // per 1,000,000 completion tokens
	CacheReadPrice  float64   `json:"cache_read_price"` // per 1,000,000 cached tokens
	FixedPrice      float64   `json:"fixed_price"`      // per request (e.g. image/video)
	Currency        string    `json:"currency"`          // CNY or USD
	OffPeakEnabled  bool      `json:"off_peak_enabled"`  // whether time-of-use discount is active
	OffPeakMode     string    `json:"off_peak_mode"`     // "custom", "night", "none"
	OffPeakSlots    string    `json:"off_peak_slots"`    // JSON array of OffPeakSlot
	WeekendAllDay   bool      `json:"weekend_all_day"`   // whether weekends are all-day off-peak
	OffPeakStart    string    `json:"off_peak_start"`    // fallback/simple start
	OffPeakEnd      string    `json:"off_peak_end"`      // fallback/simple end
	OffPeakDiscount float64   `json:"off_peak_discount"` // e.g. 0.5 (50% discount)
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// StatsOverview aggregates system stats for the dashboard.
type StatsOverview struct {
	TotalRequests     int64   `json:"total_requests"`
	TotalTokens       int64   `json:"total_tokens"`
	PromptTokens      int64   `json:"prompt_tokens"`
	CompletionTokens  int64   `json:"completion_tokens"`
	TotalCachedTokens int64   `json:"total_cached_tokens"`
	TotalCost         float64 `json:"total_cost"`
	SavedCost         float64 `json:"saved_cost"`
	ActiveChannels    int     `json:"active_channels"`
	ActiveKeys        int     `json:"active_keys"`
	AvgTTFTMs         float64 `json:"avg_ttft_ms"`
	AvgDurationMs     float64 `json:"avg_duration_ms"`
}

// Repository manages persistence for channels, keys, and logs.
type Repository struct {
	db *DB
}

var globalRepo *Repository

// SetGlobalRepository registers the global storage repository instance.
func SetGlobalRepository(repo *Repository) {
	globalRepo = repo
}

// GetGlobalRepository returns the active global repository instance.
func GetGlobalRepository() *Repository {
	return globalRepo
}

// NewRepository creates a new Repository.
func NewRepository(db *DB) *Repository {
	r := &Repository{db: db}
	globalRepo = r
	return r
}

// GetDB returns the database instance.
func (r *Repository) GetDB() *DB {
	if r == nil {
		return nil
	}
	return r.db
}

// Dialect returns the database engine dialect ("sqlite" or "postgres").
func (r *Repository) Dialect() string {
	if r == nil || r.db == nil {
		return "sqlite"
	}
	return r.db.Dialect()
}

// ListChannels returns all channels.
func (r *Repository) ListChannels() ([]*ChannelRecord, error) {
	rows, err := r.db.Query(`SELECT id, name, type, base_url, api_key, models, model_mapping, protocols, priority, weight, timeout_seconds, status, created_at, updated_at FROM channels ORDER BY priority ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*ChannelRecord
	for rows.Next() {
		var rec ChannelRecord
		var modelsJSON, mappingJSON, protocolsJSON sql.NullString
		err := rows.Scan(&rec.ID, &rec.Name, &rec.Type, &rec.BaseURL, &rec.APIKey, &modelsJSON, &mappingJSON, &protocolsJSON, &rec.Priority, &rec.Weight, &rec.TimeoutSeconds, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if modelsJSON.Valid && modelsJSON.String != "" {
			_ = json.Unmarshal([]byte(modelsJSON.String), &rec.Models)
		}
		if mappingJSON.Valid && mappingJSON.String != "" {
			_ = json.Unmarshal([]byte(mappingJSON.String), &rec.ModelMapping)
		}
		if protocolsJSON.Valid && protocolsJSON.String != "" {
			_ = json.Unmarshal([]byte(protocolsJSON.String), &rec.Protocols)
		}
		list = append(list, &rec)
	}
	return list, nil
}

// CreateChannel inserts a new channel.
func (r *Repository) CreateChannel(rec *ChannelRecord) error {
	modelsBytes, _ := json.Marshal(rec.Models)
	mappingBytes, _ := json.Marshal(rec.ModelMapping)
	protocolsBytes, _ := json.Marshal(rec.Protocols)
	if rec.Status == "" {
		rec.Status = "active"
	}
	if rec.Priority == 0 {
		rec.Priority = 1
	}
	if rec.Weight == 0 {
		rec.Weight = 10
	}
	if rec.TimeoutSeconds == 0 {
		rec.TimeoutSeconds = 60
	}

	res, err := r.db.Exec(`INSERT INTO channels (name, type, base_url, api_key, models, model_mapping, protocols, priority, weight, timeout_seconds, status, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		rec.Name, rec.Type, rec.BaseURL, rec.APIKey, string(modelsBytes), string(mappingBytes), string(protocolsBytes), rec.Priority, rec.Weight, rec.TimeoutSeconds, rec.Status)
	if err != nil {
		return err
	}
	rec.ID, _ = res.LastInsertId()
	return nil
}

// GetChannel retrieves a single channel by ID.
func (r *Repository) GetChannel(id int64) (*ChannelRecord, error) {
	row := r.db.QueryRow(`SELECT id, name, type, base_url, api_key, models, model_mapping, protocols, priority, weight, timeout_seconds, status, created_at, updated_at FROM channels WHERE id = ?`, id)
	var rec ChannelRecord
	var modelsJSON, mappingJSON, protocolsJSON sql.NullString
	if err := row.Scan(&rec.ID, &rec.Name, &rec.Type, &rec.BaseURL, &rec.APIKey, &modelsJSON, &mappingJSON, &protocolsJSON, &rec.Priority, &rec.Weight, &rec.TimeoutSeconds, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, err
	}
	if modelsJSON.Valid {
		_ = json.Unmarshal([]byte(modelsJSON.String), &rec.Models)
	}
	if mappingJSON.Valid {
		_ = json.Unmarshal([]byte(mappingJSON.String), &rec.ModelMapping)
	}
	if protocolsJSON.Valid {
		_ = json.Unmarshal([]byte(protocolsJSON.String), &rec.Protocols)
	}
	return &rec, nil
}

// UpdateChannel updates an existing channel.
func (r *Repository) UpdateChannel(rec *ChannelRecord) error {
	modelsBytes, _ := json.Marshal(rec.Models)
	mappingBytes, _ := json.Marshal(rec.ModelMapping)
	protocolsBytes, _ := json.Marshal(rec.Protocols)

	if rec.Status == "" {
		rec.Status = "active"
	}
	if rec.Priority == 0 {
		rec.Priority = 1
	}
	if rec.Weight == 0 {
		rec.Weight = 10
	}
	if rec.TimeoutSeconds == 0 {
		rec.TimeoutSeconds = 60
	}

	_, err := r.db.Exec(`UPDATE channels SET name=?, type=?, base_url=?, api_key=?, models=?, model_mapping=?, protocols=?, priority=?, weight=?, timeout_seconds=?, status=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		rec.Name, rec.Type, rec.BaseURL, rec.APIKey, string(modelsBytes), string(mappingBytes), string(protocolsBytes), rec.Priority, rec.Weight, rec.TimeoutSeconds, rec.Status, rec.ID)
	return err
}

// DeleteChannel deletes a channel by ID.
func (r *Repository) DeleteChannel(id int64) error {
	_, err := r.db.Exec(`DELETE FROM channels WHERE id=?`, id)
	return err
}

// ListVirtualKeys returns all virtual keys.
func (r *Repository) ListVirtualKeys() ([]*VirtualKeyRecord, error) {
	rows, err := r.db.Query(`SELECT id, key, tenant_id, allowed_models, rpm, tpm, budget, used_tokens, COALESCE(used_cost, 0.0), COALESCE(group_name, 'default'), COALESCE(user_id, 0), status, created_at, updated_at FROM virtual_keys ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*VirtualKeyRecord, 0)
	for rows.Next() {
		var rec VirtualKeyRecord
		var allowedJSON string
		err := rows.Scan(&rec.ID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.GroupName, &rec.UserID, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if allowedJSON != "" {
			_ = json.Unmarshal([]byte(allowedJSON), &rec.AllowedModels)
		}
		list = append(list, &rec)
	}
	return list, nil
}

// ListVirtualKeysByUser returns virtual keys belonging to a specific user.
func (r *Repository) ListVirtualKeysByUser(userID int64) ([]*VirtualKeyRecord, error) {
	rows, err := r.db.Query(`SELECT id, key, tenant_id, allowed_models, rpm, tpm, budget, used_tokens, COALESCE(used_cost, 0.0), COALESCE(group_name, 'default'), COALESCE(user_id, 0), status, created_at, updated_at FROM virtual_keys WHERE user_id = ? ORDER BY id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*VirtualKeyRecord, 0)
	for rows.Next() {
		var rec VirtualKeyRecord
		var allowedJSON string
		err := rows.Scan(&rec.ID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.GroupName, &rec.UserID, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if allowedJSON != "" {
			_ = json.Unmarshal([]byte(allowedJSON), &rec.AllowedModels)
		}
		list = append(list, &rec)
	}
	return list, nil
}

// CreateVirtualKey inserts a new virtual key.
func (r *Repository) CreateVirtualKey(rec *VirtualKeyRecord) error {
	allowedBytes, _ := json.Marshal(rec.AllowedModels)
	if rec.Status == "" {
		rec.Status = "active"
	}
	if rec.RPM < 0 {
		rec.RPM = 0
	}
	if rec.GroupName == "" {
		rec.GroupName = "default"
	}

	res, err := r.db.Exec(`INSERT INTO virtual_keys (key, tenant_id, allowed_models, rpm, tpm, budget, used_tokens, used_cost, group_name, user_id, status, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		rec.Key, rec.TenantID, string(allowedBytes), rec.RPM, rec.TPM, rec.Budget, rec.UsedTokens, rec.UsedCost, rec.GroupName, rec.UserID, rec.Status)
	if err != nil {
		return err
	}
	rec.ID, _ = res.LastInsertId()
	return nil
}

// DeleteVirtualKey deletes a key by ID.
func (r *Repository) DeleteVirtualKey(id int64) error {
	_, err := r.db.Exec(`DELETE FROM virtual_keys WHERE id=?`, id)
	return err
}

// GetVirtualKey returns a single virtual key by ID.
func (r *Repository) GetVirtualKey(id int64) (*VirtualKeyRecord, error) {
	row := r.db.QueryRow(`SELECT id, key, tenant_id, allowed_models, rpm, tpm, budget, used_tokens, COALESCE(used_cost, 0.0), COALESCE(group_name, 'default'), COALESCE(user_id, 0), status, created_at, updated_at FROM virtual_keys WHERE id = ?`, id)
	var rec VirtualKeyRecord
	var allowedJSON string
	if err := row.Scan(&rec.ID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.GroupName, &rec.UserID, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, err
	}
	if allowedJSON != "" {
		_ = json.Unmarshal([]byte(allowedJSON), &rec.AllowedModels)
	}
	return &rec, nil
}

// GetVirtualKeyByKey returns a single virtual key by key string.
func (r *Repository) GetVirtualKeyByKey(key string) (*VirtualKeyRecord, error) {
	row := r.db.QueryRow(`SELECT id, key, tenant_id, allowed_models, rpm, tpm, budget, used_tokens, COALESCE(used_cost, 0.0), COALESCE(group_name, 'default'), COALESCE(user_id, 0), status, created_at, updated_at FROM virtual_keys WHERE key = ?`, key)
	var rec VirtualKeyRecord
	var allowedJSON string
	if err := row.Scan(&rec.ID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.GroupName, &rec.UserID, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, err
	}
	if allowedJSON != "" {
		_ = json.Unmarshal([]byte(allowedJSON), &rec.AllowedModels)
	}
	return &rec, nil
}

// UpdateVirtualKey updates an existing virtual key (e.g. status, RPM, tenant, allowed models, group).
func (r *Repository) UpdateVirtualKey(rec *VirtualKeyRecord) error {
	allowedBytes, _ := json.Marshal(rec.AllowedModels)
	if rec.Status == "" {
		rec.Status = "active"
	}
	if rec.GroupName == "" {
		rec.GroupName = "default"
	}
	_, err := r.db.Exec(`UPDATE virtual_keys SET tenant_id = ?, allowed_models = ?, rpm = ?, tpm = ?, budget = ?, group_name = ?, user_id = ?, status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		rec.TenantID, string(allowedBytes), rec.RPM, rec.TPM, rec.Budget, rec.GroupName, rec.UserID, rec.Status, rec.ID)
	return err
}

// LogFilter defines search/filtering criteria for usage audit logs.
type LogFilter struct {
	Limit     int
	Offset    int
	StartTime string // e.g. "2026-09-26 00:00:00" or ISO8601
	EndTime   string
	TraceID   string
	SessionID string
	Model     string
	TenantID  string
}

// RecordUsageLog records an audit log asynchronously and updates key quota/cost.
func (r *Repository) RecordUsageLog(log *UsageLogRecord) error {
	isOff := 0
	if log.IsOffPeak {
		isOff = 1
	}
	discount := log.OffPeakDiscount
	if discount <= 0 {
		discount = 1.0
	}
	if log.TraceID == "" {
		if log.SessionID != "" {
			log.TraceID = log.SessionID
		} else {
			log.TraceID = fmt.Sprintf("tr-%x", time.Now().UnixNano())
		}
	}
	var err error
	if !log.CreatedAt.IsZero() {
		_, err = r.db.Exec(`INSERT INTO usage_logs (trace_id, session_id, virtual_key, tenant_id, model, channel, prompt_tokens, completion_tokens, cached_tokens, total_tokens, cost, is_off_peak, off_peak_discount, duration_ms, ttft_ms, status_code, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			log.TraceID, log.SessionID, log.VirtualKey, log.TenantID, log.Model, log.Channel, log.PromptTokens, log.CompletionTokens, log.CachedTokens, log.TotalTokens, log.Cost, isOff, discount, log.DurationMs, log.TTFTMs, log.StatusCode, log.CreatedAt.UTC().Format("2006-01-02 15:04:05"))
	} else {
		_, err = r.db.Exec(`INSERT INTO usage_logs (trace_id, session_id, virtual_key, tenant_id, model, channel, prompt_tokens, completion_tokens, cached_tokens, total_tokens, cost, is_off_peak, off_peak_discount, duration_ms, ttft_ms, status_code) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			log.TraceID, log.SessionID, log.VirtualKey, log.TenantID, log.Model, log.Channel, log.PromptTokens, log.CompletionTokens, log.CachedTokens, log.TotalTokens, log.Cost, isOff, discount, log.DurationMs, log.TTFTMs, log.StatusCode)
	}
	if log.VirtualKey != "" && (log.Cost > 0 || log.TotalTokens > 0) {
		_, _ = r.db.Exec(`UPDATE virtual_keys SET used_cost = used_cost + ?, used_tokens = used_tokens + ?, updated_at = CURRENT_TIMESTAMP WHERE key = ?`,
			log.Cost, log.TotalTokens, log.VirtualKey)
		if log.Cost > 0 {
			res, _ := r.db.Exec(`UPDATE users SET balance = balance - ?, updated_at = CURRENT_TIMESTAMP WHERE id = (SELECT user_id FROM virtual_keys WHERE key = ?) AND role != 'admin'`,
				log.Cost, log.VirtualKey)
			if res != nil {
				affected, _ := res.RowsAffected()
				if affected == 0 && log.TenantID != "" {
					_, _ = r.db.Exec(`UPDATE users SET balance = balance - ?, updated_at = CURRENT_TIMESTAMP WHERE (username = ? OR email = ?) AND role != 'admin'`,
						log.Cost, log.TenantID, log.TenantID)
				}
			}
		}
	} else if log.TenantID != "" && log.Cost > 0 {
		_, _ = r.db.Exec(`UPDATE users SET balance = balance - ?, updated_at = CURRENT_TIMESTAMP WHERE (username = ? OR email = ?) AND role != 'admin'`,
			log.Cost, log.TenantID, log.TenantID)
	}
	return err
}

// ListUsageLogs returns recent usage logs for audit and monitoring.
func (r *Repository) ListUsageLogs(limit int, offset int) ([]*UsageLogRecord, error) {
	return r.ListUsageLogsWithFilter(LogFilter{Limit: limit, Offset: offset})
}

// ListUsageLogsWithFilter queries logs with flexible filters (time range, trace ID, session ID, model, tenant).
func (r *Repository) ListUsageLogsWithFilter(f LogFilter) ([]*UsageLogRecord, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}

	query := `SELECT id, trace_id, COALESCE(session_id, ''), COALESCE(virtual_key, ''), COALESCE(tenant_id, ''), COALESCE(model, ''), COALESCE(channel, ''), prompt_tokens, completion_tokens, COALESCE(cached_tokens, 0), total_tokens, COALESCE(cost, 0.0), COALESCE(is_off_peak, 0), COALESCE(off_peak_discount, 1.0), duration_ms, ttft_ms, status_code, created_at FROM usage_logs WHERE 1=1`
	var args []interface{}

	if f.StartTime != "" {
		query += ` AND datetime(created_at) >= datetime(?)`
		args = append(args, f.StartTime)
	}
	if f.EndTime != "" {
		query += ` AND datetime(created_at) <= datetime(?)`
		args = append(args, f.EndTime)
	}
	if f.TraceID != "" {
		query += ` AND (trace_id = ? OR trace_id LIKE ?)`
		args = append(args, f.TraceID, "%"+f.TraceID+"%")
	}
	if f.SessionID != "" {
		query += ` AND (session_id = ? OR session_id LIKE ?)`
		args = append(args, f.SessionID, "%"+f.SessionID+"%")
	}
	if f.Model != "" {
		query += ` AND model = ?`
		args = append(args, f.Model)
	}
	if f.TenantID != "" {
		query += ` AND tenant_id = ?`
		args = append(args, f.TenantID)
	}

	query += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, f.Limit, f.Offset)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*UsageLogRecord
	for rows.Next() {
		var l UsageLogRecord
		var isOff int
		var createdAt time.Time
		if err := rows.Scan(&l.ID, &l.TraceID, &l.SessionID, &l.VirtualKey, &l.TenantID, &l.Model, &l.Channel, &l.PromptTokens, &l.CompletionTokens, &l.CachedTokens, &l.TotalTokens, &l.Cost, &isOff, &l.OffPeakDiscount, &l.DurationMs, &l.TTFTMs, &l.StatusCode, &createdAt); err != nil {
			return nil, err
		}
		l.IsOffPeak = isOff == 1
		l.CreatedAt = createdAt
		logs = append(logs, &l)
	}
	return logs, nil
}

// DeleteUsageLog deletes a single audit log by ID.
func (r *Repository) DeleteUsageLog(id int64) error {
	_, err := r.db.Exec(`DELETE FROM usage_logs WHERE id = ?`, id)
	return err
}

// BatchDeleteUsageLogs deletes multiple usage logs by their IDs.
func (r *Repository) BatchDeleteUsageLogs(ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf(`DELETE FROM usage_logs WHERE id IN (%s)`, strings.Join(placeholders, ","))
	res, err := r.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ClearAllUsageLogs wipes all audit logs.
func (r *Repository) ClearAllUsageLogs() error {
	_, err := r.db.Exec(`DELETE FROM usage_logs`)
	return err
}

// BatchDeleteChannels deletes channels by ID list.
func (r *Repository) BatchDeleteChannels(ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf(`DELETE FROM channels WHERE id IN (%s)`, strings.Join(placeholders, ","))
	res, err := r.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// BatchUpdateChannelStatus updates status ('active'/'disabled') for multiple channels.
func (r *Repository) BatchUpdateChannelStatus(ids []int64, status string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids)+1)
	args[0] = status
	for i, id := range ids {
		placeholders[i] = "?"
		args[i+1] = id
	}
	q := fmt.Sprintf(`UPDATE channels SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id IN (%s)`, strings.Join(placeholders, ","))
	res, err := r.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// BatchDeleteVirtualKeys deletes multiple virtual keys.
func (r *Repository) BatchDeleteVirtualKeys(ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf(`DELETE FROM virtual_keys WHERE id IN (%s)`, strings.Join(placeholders, ","))
	res, err := r.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// BatchUpdateVirtualKeyStatus updates status ('active'/'disabled') for multiple virtual keys.
func (r *Repository) BatchUpdateVirtualKeyStatus(ids []int64, status string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids)+1)
	args[0] = status
	for i, id := range ids {
		placeholders[i] = "?"
		args[i+1] = id
	}
	q := fmt.Sprintf(`UPDATE virtual_keys SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id IN (%s)`, strings.Join(placeholders, ","))
	res, err := r.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// BatchDeleteModelPrices deletes pricing for multiple models.
func (r *Repository) BatchDeleteModelPrices(models []string) (int64, error) {
	if len(models) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(models))
	args := make([]interface{}, len(models))
	for i, m := range models {
		placeholders[i] = "?"
		args[i] = m
	}
	q := fmt.Sprintf(`DELETE FROM model_prices WHERE model IN (%s)`, strings.Join(placeholders, ","))
	res, err := r.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ModelPriceKey identifies a specific model within a pricing group.
type ModelPriceKey struct {
	Model string `json:"model"`
	Group string `json:"group"`
}

// BatchDeleteModelPriceKeys deletes specific (model, group) combinations.
func (r *Repository) BatchDeleteModelPriceKeys(keys []ModelPriceKey) (int64, error) {
	if len(keys) == 0 {
		return 0, nil
	}
	var total int64
	for _, k := range keys {
		grp := strings.TrimSpace(k.Group)
		if grp == "" {
			grp = "default"
		}
		res, err := r.db.Exec(`DELETE FROM model_prices WHERE model = ? AND group_name = ?`, k.Model, grp)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}

// GetStatsOverview queries summary metrics.
func (r *Repository) GetStatsOverview() (*StatsOverview, error) {
	stats := &StatsOverview{}

	row := r.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(total_tokens), 0), COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(completion_tokens), 0), COALESCE(SUM(cached_tokens), 0), COALESCE(SUM(cost), 0.0), COALESCE(AVG(ttft_ms), 0), COALESCE(AVG(duration_ms), 0) FROM usage_logs`)
	err := row.Scan(&stats.TotalRequests, &stats.TotalTokens, &stats.PromptTokens, &stats.CompletionTokens, &stats.TotalCachedTokens, &stats.TotalCost, &stats.AvgTTFTMs, &stats.AvgDurationMs)
	if err != nil {
		return nil, fmt.Errorf("scan usage stats error: %w", err)
	}

	if stats.TotalCachedTokens > 0 {
		// Benchmark prompt cache savings (e.g. 1.8 CNY per 1M cached tokens discount)
		stats.SavedCost = float64(stats.TotalCachedTokens) * 1.8 / 1000000.0
	}

	_ = r.db.QueryRow(`SELECT COUNT(*) FROM channels WHERE status='active'`).Scan(&stats.ActiveChannels)
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM virtual_keys WHERE status='active'`).Scan(&stats.ActiveKeys)

	return stats, nil
}

// ToModelChannels converts database ChannelRecords to Data Plane model.ChannelConfigs.
func (r *Repository) ToModelChannels() ([]model.ChannelConfig, error) {
	records, err := r.ListChannels()
	if err != nil {
		return nil, err
	}
	var res []model.ChannelConfig
	for _, rec := range records {
		if rec.Status != "active" {
			continue
		}
		res = append(res, model.ChannelConfig{
			ID:             rec.ID,
			Name:           rec.Name,
			Type:           rec.Type,
			BaseURL:        rec.BaseURL,
			APIKey:         rec.APIKey,
			Models:         rec.Models,
			ModelMapping:   rec.ModelMapping,
			Protocols:      rec.Protocols,
			Priority:       rec.Priority,
			Weight:         rec.Weight,
			TimeoutSeconds: rec.TimeoutSeconds,
			Status:         rec.Status,
		})
	}
	return res, nil
}

// ToModelVirtualKeys converts database VirtualKeyRecords to Data Plane model.VirtualKeyConfigs.
func (r *Repository) ToModelVirtualKeys() ([]model.VirtualKeyConfig, error) {
	records, err := r.ListVirtualKeys()
	if err != nil {
		return nil, err
	}
	var res []model.VirtualKeyConfig
	for _, rec := range records {
		if rec.Status != "active" {
			continue
		}
		res = append(res, model.VirtualKeyConfig{
			Key:           rec.Key,
			TenantID:      rec.TenantID,
			AllowedModels: rec.AllowedModels,
			RPM:           rec.RPM,
			TPM:           rec.TPM,
			Budget:        rec.Budget,
			GroupName:     rec.GroupName,
			UserID:        rec.UserID,
		})
	}
	return res, nil
}

// UserRecord represents an administrator, operator, or standard user account.
type UserRecord struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`       // "admin" or "user"
	Status       string    `json:"status"`     // "active" or "locked"
	Balance      float64   `json:"balance"`    // Wallet balance in CNY
	GroupName    string    `json:"group_name"` // "default", "vip", "enterprise"
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// GetUserByUsername finds a user by username or email.
func (r *Repository) GetUserByUsername(username string) (*UserRecord, error) {
	row := r.db.QueryRow(`SELECT id, username, COALESCE(email, ''), password_hash, COALESCE(role, 'user'), COALESCE(status, 'active'), COALESCE(balance, 0.0), COALESCE(group_name, 'default'), created_at, updated_at FROM users WHERE username = ? OR email = ?`, username, username)
	var u UserRecord
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.Balance, &u.GroupName, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserByID finds a user by primary key ID.
func (r *Repository) GetUserByID(id int64) (*UserRecord, error) {
	row := r.db.QueryRow(`SELECT id, username, COALESCE(email, ''), password_hash, COALESCE(role, 'user'), COALESCE(status, 'active'), COALESCE(balance, 0.0), COALESCE(group_name, 'default'), created_at, updated_at FROM users WHERE id = ?`, id)
	var u UserRecord
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.Balance, &u.GroupName, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserByEmail finds a user by email address.
func (r *Repository) GetUserByEmail(email string) (*UserRecord, error) {
	if r == nil || r.db == nil || strings.TrimSpace(email) == "" {
		return nil, sql.ErrNoRows
	}
	row := r.db.QueryRow(`SELECT id, username, COALESCE(email, ''), password_hash, COALESCE(role, 'user'), COALESCE(status, 'active'), COALESCE(balance, 0.0), COALESCE(group_name, 'default'), created_at, updated_at FROM users WHERE email = ?`, strings.TrimSpace(email))
	var u UserRecord
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.Balance, &u.GroupName, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	return &u, nil
}

// CreateUser inserts a new user record.
func (r *Repository) CreateUser(u *UserRecord) error {
	if u.Role == "" {
		u.Role = "user"
	}
	if u.Status == "" {
		u.Status = "active"
	}
	if u.GroupName == "" {
		u.GroupName = "default"
	}
	res, err := r.db.Exec(`INSERT INTO users (username, email, password_hash, role, status, balance, group_name, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		u.Username, u.Email, u.PasswordHash, u.Role, u.Status, u.Balance, u.GroupName)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err == nil {
		u.ID = id
	}
	return nil
}

// UpdateUserPassword updates the password hash for a user.
func (r *Repository) UpdateUserPassword(username, newHash string) error {
	_, err := r.db.Exec(`UPDATE users SET password_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, newHash, username)
	return err
}

// UpdateUserStatus updates user status ('active' or 'locked').
func (r *Repository) UpdateUserStatus(username, status string) error {
	_, err := r.db.Exec(`UPDATE users SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, status, username)
	return err
}

// UpdateUserBalance adjusts a user's wallet balance by a delta amount.
func (r *Repository) UpdateUserBalance(username string, delta float64) error {
	_, err := r.db.Exec(`UPDATE users SET balance = balance + ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, delta, username)
	return err
}

// SetUserBalance sets an exact wallet balance for a user.
func (r *Repository) SetUserBalance(username string, balance float64) error {
	_, err := r.db.Exec(`UPDATE users SET balance = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, balance, username)
	return err
}

// UpdateUserGroup updates a user's pricing group.
func (r *Repository) UpdateUserGroup(username, groupName string) error {
	if groupName == "" {
		groupName = "default"
	}
	_, err := r.db.Exec(`UPDATE users SET group_name = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, groupName, username)
	return err
}

// UpdateUserRole updates a user's system role ('admin' or 'user').
func (r *Repository) UpdateUserRole(username, role string) error {
	if role != "admin" && role != "user" {
		role = "user"
	}
	_, err := r.db.Exec(`UPDATE users SET role = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, role, username)
	return err
}

// DeductUserBalance deducts quota/cost from user wallet. Admins are exempt.
func (r *Repository) DeductUserBalance(userID int64, cost float64) error {
	if userID <= 0 || cost <= 0 {
		return nil
	}
	_, err := r.db.Exec(`UPDATE users SET balance = balance - ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND role != 'admin'`, cost, userID)
	return err
}

// ListUsers returns all registered users without password hashes.
func (r *Repository) ListUsers() ([]*UserRecord, error) {
	rows, err := r.db.Query(`SELECT id, username, COALESCE(email, ''), COALESCE(role, 'user'), COALESCE(status, 'active'), COALESCE(balance, 0.0), COALESCE(group_name, 'default'), created_at, updated_at FROM users ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*UserRecord
	for rows.Next() {
		var u UserRecord
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.Status, &u.Balance, &u.GroupName, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, &u)
	}
	return list, nil
}

// DeleteUser removes a user by username.
func (r *Repository) DeleteUser(username string) error {
	_, err := r.db.Exec(`DELETE FROM users WHERE username = ?`, username)
	return err
}

// EnsureDefaultAdmin initializes the default admin user if it does not already exist.
func (r *Repository) EnsureDefaultAdmin(username, plainPass string) error {
	existing, _ := r.GetUserByUsername(username)
	if existing != nil {
		// Ensure admin has infinite balance flag and active status
		if existing.Balance < 999999 {
			_ = r.SetUserBalance(username, 9999999.0)
		}
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(plainPass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return r.CreateUser(&UserRecord{
		Username:     username,
		Email:        "admin@airoute.local",
		PasswordHash: string(hash),
		Role:         "admin",
		Status:       "active",
		Balance:      9999999.0, // Admin has infinite quota
		GroupName:    "default",
	})
}

// RedemptionCodeRecord represents a gift or balance redemption code.
type RedemptionCodeRecord struct {
	ID        int64      `json:"id"`
	Code      string     `json:"code"`
	Name      string     `json:"name"`
	Amount    float64    `json:"amount"`
	Status    string     `json:"status"` // "active", "used"
	UsedBy    string     `json:"used_by"`
	UsedAt    *time.Time `json:"used_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// CreateRedemptionCode creates a new redemption card.
func (r *Repository) CreateRedemptionCode(rec *RedemptionCodeRecord) error {
	if rec.Status == "" {
		rec.Status = "active"
	}
	res, err := r.db.Exec(`INSERT INTO redemption_codes (code, name, amount, status, updated_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		rec.Code, rec.Name, rec.Amount, rec.Status)
	if err != nil {
		// SQLite table doesn't have updated_at, handle gracefully
		res, err = r.db.Exec(`INSERT INTO redemption_codes (code, name, amount, status) VALUES (?, ?, ?, ?)`,
			rec.Code, rec.Name, rec.Amount, rec.Status)
		if err != nil {
			return err
		}
	}
	id, err := res.LastInsertId()
	if err == nil {
		rec.ID = id
	}
	return nil
}

// ListRedemptionCodes returns all redemption codes.
func (r *Repository) ListRedemptionCodes() ([]*RedemptionCodeRecord, error) {
	rows, err := r.db.Query(`SELECT id, code, COALESCE(name, ''), amount, COALESCE(status, 'active'), COALESCE(used_by, ''), used_at, created_at FROM redemption_codes ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*RedemptionCodeRecord
	for rows.Next() {
		var rec RedemptionCodeRecord
		var usedAt sql.NullTime
		if err := rows.Scan(&rec.ID, &rec.Code, &rec.Name, &rec.Amount, &rec.Status, &rec.UsedBy, &usedAt, &rec.CreatedAt); err != nil {
			return nil, err
		}
		if usedAt.Valid {
			t := usedAt.Time
			rec.UsedAt = &t
		}
		list = append(list, &rec)
	}
	return list, nil
}

// RedeemCode redeems a code and adds its amount to the specified user's balance.
func (r *Repository) RedeemCode(code, username string) (*RedemptionCodeRecord, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, fmt.Errorf("兑换码不能为空")
	}

	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var rec RedemptionCodeRecord
	var usedAt sql.NullTime
	row := tx.QueryRow(`SELECT id, code, COALESCE(name, ''), amount, COALESCE(status, 'active'), COALESCE(used_by, ''), used_at, created_at FROM redemption_codes WHERE code = ?`, code)
	if err := row.Scan(&rec.ID, &rec.Code, &rec.Name, &rec.Amount, &rec.Status, &rec.UsedBy, &usedAt, &rec.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("无效的兑换码")
		}
		return nil, err
	}

	if rec.Status != "active" {
		return nil, fmt.Errorf("该兑换码已被使用或已失效")
	}

	// Mark as used
	now := time.Now()
	_, err = tx.Exec(`UPDATE redemption_codes SET status = 'used', used_by = ?, used_at = CURRENT_TIMESTAMP WHERE id = ?`, username, rec.ID)
	if err != nil {
		return nil, fmt.Errorf("更新兑换状态失败: %w", err)
	}

	// Credit user balance
	_, err = tx.Exec(`UPDATE users SET balance = balance + ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, rec.Amount, username)
	if err != nil {
		return nil, fmt.Errorf("充值到账户余额失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	rec.Status = "used"
	rec.UsedBy = username
	rec.UsedAt = &now
	return &rec, nil
}

// DeleteRedemptionCode removes a redemption code.
func (r *Repository) DeleteRedemptionCode(id int64) error {
	_, err := r.db.Exec(`DELETE FROM redemption_codes WHERE id = ?`, id)
	return err
}

// RechargeOrderRecord represents a user wallet top-up order.
type RechargeOrderRecord struct {
	ID              int64     `json:"id"`
	OrderNo         string    `json:"order_no"`
	Username        string    `json:"username"`
	Amount          float64   `json:"amount"`
	Currency        string    `json:"currency"`
	Channel         string    `json:"channel"`           // "stripe", "sandbox"
	StripeSessionID string    `json:"stripe_session_id"` // Stripe Checkout Session ID
	Status          string    `json:"status"`            // "pending", "paid", "cancelled"
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// CreateRechargeOrder inserts a new recharge order.
func (r *Repository) CreateRechargeOrder(rec *RechargeOrderRecord) error {
	if rec.Currency == "" {
		rec.Currency = "CNY"
	}
	if rec.Status == "" {
		rec.Status = "pending"
	}
	res, err := r.db.Exec(`INSERT INTO recharge_orders (order_no, username, amount, currency, channel, stripe_session_id, status, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		rec.OrderNo, rec.Username, rec.Amount, rec.Currency, rec.Channel, rec.StripeSessionID, rec.Status)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err == nil {
		rec.ID = id
	}
	return nil
}

// CompleteRechargeOrder marks an order as paid and credits the user's wallet balance.
func (r *Repository) CompleteRechargeOrder(orderNo string) (*RechargeOrderRecord, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var rec RechargeOrderRecord
	row := tx.QueryRow(`SELECT id, order_no, username, amount, currency, channel, stripe_session_id, status, created_at, updated_at FROM recharge_orders WHERE order_no = ?`, orderNo)
	if err := row.Scan(&rec.ID, &rec.OrderNo, &rec.Username, &rec.Amount, &rec.Currency, &rec.Channel, &rec.StripeSessionID, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, fmt.Errorf("订单不存在: %w", err)
	}

	if rec.Status == "paid" {
		return &rec, nil // Already completed (idempotent)
	}

	_, err = tx.Exec(`UPDATE recharge_orders SET status = 'paid', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, rec.ID)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(`UPDATE users SET balance = balance + ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, rec.Amount, rec.Username)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	rec.Status = "paid"
	return &rec, nil
}

// ListRechargeOrders returns recharge orders for a specific user or all users if username is empty.
func (r *Repository) ListRechargeOrders(username string) ([]*RechargeOrderRecord, error) {
	var rows *sql.Rows
	var err error
	if username != "" {
		rows, err = r.db.Query(`SELECT id, order_no, username, amount, currency, channel, stripe_session_id, status, created_at, updated_at FROM recharge_orders WHERE username = ? ORDER BY id DESC LIMIT 50`, username)
	} else {
		rows, err = r.db.Query(`SELECT id, order_no, username, amount, currency, channel, stripe_session_id, status, created_at, updated_at FROM recharge_orders ORDER BY id DESC LIMIT 100`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*RechargeOrderRecord
	for rows.Next() {
		var rec RechargeOrderRecord
		if err := rows.Scan(&rec.ID, &rec.OrderNo, &rec.Username, &rec.Amount, &rec.Currency, &rec.Channel, &rec.StripeSessionID, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, &rec)
	}
	return list, nil
}

// VerificationCodeRecord stores temporary email verification codes.
type VerificationCodeRecord struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Code      string    `json:"code"`
	Purpose   string    `json:"purpose"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
	CreatedAt time.Time `json:"created_at"`
}

// SaveVerificationCode stores a verification code with an expiration window.
func (r *Repository) SaveVerificationCode(email, code, purpose string, duration time.Duration) error {
	expiresAt := time.Now().Add(duration)
	_, err := r.db.Exec(`INSERT INTO verification_codes (email, code, purpose, expires_at, used) VALUES (?, ?, ?, ?, 0)`,
		strings.ToLower(strings.TrimSpace(email)), strings.TrimSpace(code), purpose, expiresAt.UTC().Format("2006-01-02 15:04:05"))
	return err
}

// VerifyCode validates and consumes a verification code.
func (r *Repository) VerifyCode(email, code, purpose string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	code = strings.TrimSpace(code)
	if email == "" || code == "" {
		return false
	}

	var id int64
	row := r.db.QueryRow(`SELECT id FROM verification_codes WHERE email = ? AND code = ? AND purpose = ? AND used = 0 AND datetime(expires_at) > datetime('now') ORDER BY id DESC LIMIT 1`,
		email, code, purpose)
	if err := row.Scan(&id); err != nil {
		return false
	}

	// Mark as used
	_, _ = r.db.Exec(`UPDATE verification_codes SET used = 1 WHERE id = ?`, id)
	return true
}

// ModelFallbackRecord represents a cross-model fallback rule.
type ModelFallbackRecord struct {
	Model         string    `json:"model"`
	FallbackModel string    `json:"fallback_model"`
	Enabled       bool      `json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// GetModelFallbacks returns all active model fallbacks mapped as target -> fallback.
func (r *Repository) GetModelFallbacks() (map[string]string, error) {
	rows, err := r.db.Query(`SELECT model, fallback_model FROM model_fallbacks WHERE enabled = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]string)
	for rows.Next() {
		var src, dst string
		if err := rows.Scan(&src, &dst); err == nil && src != "" && dst != "" {
			m[src] = dst
		}
	}
	return m, nil
}

// SetModelFallback creates or updates a model fallback mapping.
func (r *Repository) SetModelFallback(model, fallbackModel string, enabled bool) error {
	en := 1
	if !enabled {
		en = 0
	}
	_, err := r.db.Exec(`
		INSERT INTO model_fallbacks (model, fallback_model, enabled, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(model) DO UPDATE SET fallback_model = excluded.fallback_model, enabled = excluded.enabled, updated_at = CURRENT_TIMESTAMP
	`, model, fallbackModel, en)
	return err
}

// DeleteModelFallback removes a model fallback mapping.
func (r *Repository) DeleteModelFallback(model string) error {
	_, err := r.db.Exec(`DELETE FROM model_fallbacks WHERE model = ?`, model)
	return err
}

// ListModelPrices returns all configured model pricing rates.
func (r *Repository) ListModelPrices() ([]*ModelPriceRecord, error) {
	rows, err := r.db.Query(`SELECT id, model, COALESCE(group_name, 'default'), prompt_price, completion_price, cache_read_price, fixed_price, currency, COALESCE(off_peak_enabled, 1), COALESCE(off_peak_start, '00:00'), COALESCE(off_peak_end, '08:30'), COALESCE(off_peak_discount, 0.5), COALESCE(off_peak_mode, 'custom'), COALESCE(off_peak_slots, ''), COALESCE(weekend_all_day, 1), created_at, updated_at FROM model_prices ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*ModelPriceRecord, 0)
	for rows.Next() {
		var rec ModelPriceRecord
		var offEnabled, weekendAllDay int
		if err := rows.Scan(&rec.ID, &rec.Model, &rec.GroupName, &rec.PromptPrice, &rec.CompletionPrice, &rec.CacheReadPrice, &rec.FixedPrice, &rec.Currency, &offEnabled, &rec.OffPeakStart, &rec.OffPeakEnd, &rec.OffPeakDiscount, &rec.OffPeakMode, &rec.OffPeakSlots, &weekendAllDay, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			return nil, err
		}
		rec.OffPeakEnabled = offEnabled == 1
		rec.WeekendAllDay = weekendAllDay == 1
		list = append(list, &rec)
	}
	return list, nil
}

// GetModelPrice returns pricing rates for a specific model (defaulting to default group).
func (r *Repository) GetModelPrice(modelName string) (*ModelPriceRecord, error) {
	return r.GetModelPriceWithGroup(modelName, "default")
}

// GetModelPriceExact returns pricing rates for an exact model and group without fallback.
func (r *Repository) GetModelPriceExact(modelName, groupName string) (*ModelPriceRecord, error) {
	if groupName == "" {
		groupName = "default"
	}
	row := r.db.QueryRow(`SELECT id, model, COALESCE(group_name, 'default'), prompt_price, completion_price, cache_read_price, fixed_price, currency, COALESCE(off_peak_enabled, 1), COALESCE(off_peak_start, '00:00'), COALESCE(off_peak_end, '08:30'), COALESCE(off_peak_discount, 0.5), COALESCE(off_peak_mode, 'custom'), COALESCE(off_peak_slots, ''), COALESCE(weekend_all_day, 1), created_at, updated_at FROM model_prices WHERE model = ? AND group_name = ?`, modelName, groupName)
	var rec ModelPriceRecord
	var offEnabled, weekendAllDay int
	if err := row.Scan(&rec.ID, &rec.Model, &rec.GroupName, &rec.PromptPrice, &rec.CompletionPrice, &rec.CacheReadPrice, &rec.FixedPrice, &rec.Currency, &offEnabled, &rec.OffPeakStart, &rec.OffPeakEnd, &rec.OffPeakDiscount, &rec.OffPeakMode, &rec.OffPeakSlots, &weekendAllDay, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, err
	}
	rec.OffPeakEnabled = offEnabled == 1
	rec.WeekendAllDay = weekendAllDay == 1
	return &rec, nil
}

// GetModelPriceWithGroup returns pricing rates for a specific model and group (with fallback to default group).
func (r *Repository) GetModelPriceWithGroup(modelName, groupName string) (*ModelPriceRecord, error) {
	if groupName == "" {
		groupName = "default"
	}
	rec, err := r.GetModelPriceExact(modelName, groupName)
	if err == nil {
		return rec, nil
	}
	// Fallback to default group if specific group not found
	if groupName != "default" {
		return r.GetModelPriceExact(modelName, "default")
	}
	return nil, sql.ErrNoRows
}

// SaveModelPrice inserts or updates pricing rates for a model within a specific group.
func (r *Repository) SaveModelPrice(rec *ModelPriceRecord) error {
	if rec.GroupName == "" {
		rec.GroupName = "default"
	}
	if rec.Currency == "" {
		rec.Currency = "CNY"
	}
	if rec.OffPeakMode == "" {
		rec.OffPeakMode = "custom"
	}
	if rec.OffPeakSlots == "" && rec.OffPeakEnabled {
		rec.OffPeakSlots = `[{"start":"00:00","end":"08:30","discount":0.5}]`
	}
	if rec.OffPeakStart == "" {
		rec.OffPeakStart = "00:00"
	}
	if rec.OffPeakEnd == "" {
		rec.OffPeakEnd = "08:30"
	}
	if rec.OffPeakDiscount <= 0 || rec.OffPeakDiscount > 1.0 {
		rec.OffPeakDiscount = 0.5
	}
	offEnabled := 0
	if rec.OffPeakEnabled {
		offEnabled = 1
	}
	weekendAll := 0
	if rec.WeekendAllDay {
		weekendAll = 1
	}

	_, err := r.db.Exec(`
		INSERT INTO model_prices (model, group_name, prompt_price, completion_price, cache_read_price, fixed_price, currency, off_peak_enabled, off_peak_start, off_peak_end, off_peak_discount, off_peak_mode, off_peak_slots, weekend_all_day, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(model, group_name) DO UPDATE SET 
			prompt_price = excluded.prompt_price,
			completion_price = excluded.completion_price,
			cache_read_price = excluded.cache_read_price,
			fixed_price = excluded.fixed_price,
			currency = excluded.currency,
			off_peak_enabled = excluded.off_peak_enabled,
			off_peak_start = excluded.off_peak_start,
			off_peak_end = excluded.off_peak_end,
			off_peak_discount = excluded.off_peak_discount,
			off_peak_mode = excluded.off_peak_mode,
			off_peak_slots = excluded.off_peak_slots,
			weekend_all_day = excluded.weekend_all_day,
			updated_at = CURRENT_TIMESTAMP
	`, rec.Model, rec.GroupName, rec.PromptPrice, rec.CompletionPrice, rec.CacheReadPrice, rec.FixedPrice, rec.Currency, offEnabled, rec.OffPeakStart, rec.OffPeakEnd, rec.OffPeakDiscount, rec.OffPeakMode, rec.OffPeakSlots, weekendAll)
	return err
}

// DeleteModelPrice removes pricing rates for a model.
func (r *Repository) DeleteModelPrice(modelName string) error {
	_, err := r.db.Exec(`DELETE FROM model_prices WHERE model = ?`, modelName)
	return err
}

// DeleteModelPriceWithGroup removes pricing rates for a specific model and group.
func (r *Repository) DeleteModelPriceWithGroup(modelName, groupName string) error {
	if groupName == "" {
		groupName = "default"
	}
	_, err := r.db.Exec(`DELETE FROM model_prices WHERE model = ? AND group_name = ?`, modelName, groupName)
	return err
}

// SeedDefaultModelPrices seeds industry-standard pricing benchmark presets if table is empty.
func (r *Repository) SeedDefaultModelPrices() error {
	var count int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM model_prices`).Scan(&count)
	if count > 0 {
		return nil
	}

	defaults := []ModelPriceRecord{
		{Model: "claude-opus-5.5", PromptPrice: 35.0, CompletionPrice: 175.0, CacheReadPrice: 3.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gpt-6-astra", PromptPrice: 30.0, CompletionPrice: 120.0, CacheReadPrice: 3.0, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "claude-fable-5.1", PromptPrice: 18.0, CompletionPrice: 90.0, CacheReadPrice: 1.8, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gemini-3.8-flash", PromptPrice: 1.5, CompletionPrice: 6.0, CacheReadPrice: 0.15, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gemini-3.8-live", PromptPrice: 5.0, CompletionPrice: 20.0, CacheReadPrice: 0.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "deepseek-v4.1-flash", PromptPrice: 1.0, CompletionPrice: 4.0, CacheReadPrice: 0.1, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "deepseek-r1", PromptPrice: 4.0, CompletionPrice: 16.0, CacheReadPrice: 0.4, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "qwen-3.8-max", PromptPrice: 6.0, CompletionPrice: 24.0, CacheReadPrice: 1.2, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gpt-6-sol", PromptPrice: 12.0, CompletionPrice: 48.0, CacheReadPrice: 1.2, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "flux-1.1-pro", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.20, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "sora-2", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 1.50, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "whisper-large-v3-turbo", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.04, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "deepseek-chat", PromptPrice: 2.0, CompletionPrice: 8.0, CacheReadPrice: 0.2, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "deepseek-reasoner", PromptPrice: 4.0, CompletionPrice: 16.0, CacheReadPrice: 0.4, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gpt-4o", PromptPrice: 18.0, CompletionPrice: 72.0, CacheReadPrice: 9.0, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "claude-3-5-sonnet", PromptPrice: 21.0, CompletionPrice: 105.0, CacheReadPrice: 2.1, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "dall-e-3", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.28, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "tts-1", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.10, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "whisper-1", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.05, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
	}

	for _, d := range defaults {
		_ = r.SaveModelPrice(&d)
	}
	return nil
}

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

// SeedDefaultSkills ensures built-in skills exist with rich manifests and progressive loading configs.
func (r *Repository) SeedDefaultSkills() error {
	defaults := []SkillRecord{
		{
			ID:          "gateway_ops",
			Name:        "网关运维与状态探针",
			Description: "查询实时可用模型拓扑、上游渠道熔断状态与历史审计日志",
			Category:    "ops",
			Tools:       []string{"nano_list_models", "nano_check_status", "nano_query_logs"},
			LoadingMode: "lazy",
			Author:      "Nano Official",
			Version:     "1.0.0",
			Enabled:     true,
			Manifest: `---
name: gateway_ops
description: 网关运维与状态探针
category: ops
author: Nano Official
version: 1.0.0
loading_mode: lazy
tools:
  - nano_list_models
  - nano_check_status
  - nano_query_logs
---

# 网关运维与状态探针 (gateway_ops)

提供大模型网关集群的健康探针、模型拓扑治理与用量审计能力。

## 触发场景
- 查询网关当前可用模型列表及其支持的模态（如 chat, embeddings, images 等）
- 检查各上游提供商与下游渠道的实时连通性、时延与熔断器健康状态
- 检索历史请求日志，按会话 Session ID 追踪 Token 消耗与成本明细

## 工具清单
- nano_list_models(modality?: string): 查询统一模型路由拓扑
- nano_check_status(): 获取上游渠道健康状态与熔断指标
- nano_query_logs(session_id?: string, limit?: number): 查询会话调用明细
`,
		},
		{
			ID:          "model_router",
			Name:        "多模型协作与智能对话代理",
			Description: "跨渠道分发会话请求，支持自动会话粘连和前缀缓存亲和性",
			Category:    "agent",
			Tools:       []string{"nano_chat"},
			LoadingMode: "eager",
			Author:      "Nano Official",
			Version:     "1.0.0",
			Enabled:     true,
			Manifest: `---
name: model_router
description: 多模型协作与智能对话代理
category: agent
author: Nano Official
version: 1.0.0
loading_mode: eager
tools:
  - nano_chat
---

# 多模型协作与智能对话代理 (model_router)

支持将任务委派给指定模型执行对话补全，享受网关内置的零配置会话保持与前缀缓存亲和性。

## 触发场景
- Agent 需要借助另一个模型协助完成子任务时（如深思链、代码生成、摘要提炼）

## 工具清单
- nano_chat(model: string, message: string, session_id?: string): 向指定模型发起对话
`,
		},
		{
			ID:          "web_search",
			Name:        "实时联网检索与知识增强",
			Description: "为接入的 AI Agent 提供全局联网搜索能力，返回实时权威网页结果与摘要",
			Category:    "search",
			Tools:       []string{"nano_web_search"},
			LoadingMode: "lazy",
			Author:      "Nano Official",
			Version:     "1.0.0",
			Enabled:     true,
			Manifest: `---
name: web_search
description: 实时联网检索与知识增强
category: search
author: Nano Official
version: 1.0.0
loading_mode: lazy
tools:
  - nano_web_search
---

# 实时联网检索与知识增强 (web_search)

提供高质量公网信息检索能力，返回权威网页摘要与参考引用链接。

## 触发场景
- 用户问题涉及最新时事、最新发布的框架/库版本、实时股票/天气或外部实时资料
- 知识库截断日期之后的问题解答

## 工具清单
- nano_web_search(query: string): 执行公网搜索并获取摘要和引用
`,
		},
		{
			ID:          "datetime_clock",
			Name:        "高精度时区与闲时感知",
			Description: "精确获取服务器当前时间、时区、星期以及实时闲时半价时段判定",
			Category:    "utility",
			Tools:       []string{"nano_get_current_time"},
			LoadingMode: "lazy",
			Author:      "Nano Official",
			Version:     "1.0.0",
			Enabled:     true,
			Manifest: `---
name: datetime_clock
description: 高精度时区与闲时感知
category: utility
author: Nano Official
version: 1.0.0
loading_mode: lazy
tools:
  - nano_get_current_time
---

# 高精度时区与闲时感知 (datetime_clock)

精确获取服务器当前时间、时区、星期，并自动计算当前是否处于 DeepSeek 等模型官方闲时优惠窗口。

## 触发场景
- 用户询问当前时间、日期、星期几或调度任务规划时
- 需要根据当前时间判断模型计费是否享受闲时折扣时

## 工具清单
- nano_get_current_time(): 获取当前标准时间、时区及闲时半价命中状态
`,
		},
		{
			ID:          "code_runner",
			Name:        "轻量代码执行与数学表达式计算",
			Description: "提供安全的四则运算、高精度数学计算、单位换算与逻辑求值",
			Category:    "utility",
			Tools:       []string{"nano_calc_eval"},
			LoadingMode: "lazy",
			Author:      "Nano Official",
			Version:     "1.0.0",
			Enabled:     true,
			Manifest: `---
name: code_runner
description: 轻量代码执行与数学表达式计算
category: utility
author: Nano Official
version: 1.0.0
loading_mode: lazy
tools:
  - nano_calc_eval
---

# 轻量代码执行与数学表达式计算 (code_runner)

提供高精度的四则运算、指数对数、复合数学公式求值，消除大语言模型的计算幻觉。

## 触发场景
- 用户输入复杂的代数运算、汇率或比例换算、大数相乘
- 需要准确数值计算而非估算的场景

## 工具清单
- nano_calc_eval(expression: string): 评估并计算数学表达式（如 "(128 * 1024) / 0.85"）
`,
		},
	}

	for _, s := range defaults {
		toolsJSON, _ := json.Marshal(s.Tools)
		enabledInt := 0
		if s.Enabled {
			enabledInt = 1
		}
		_, _ = r.db.Exec(`
			INSERT INTO system_skills (id, name, description, category, tools, loading_mode, manifest, author, version, enabled, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name,
				description = excluded.description,
				category = excluded.category,
				tools = excluded.tools,
				loading_mode = excluded.loading_mode,
				manifest = excluded.manifest,
				author = excluded.author,
				version = excluded.version
		`, s.ID, s.Name, s.Description, s.Category, string(toolsJSON), s.LoadingMode, s.Manifest, s.Author, s.Version, enabledInt)
	}
	return nil
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
	if toolName == "nano_search_skills" || toolName == "nano_discover_skills" || toolName == "nano_inspect_skill" || toolName == "nano_get_skill_manifest" {
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

// GetSetting retrieves a system configuration value.
func (r *Repository) GetSetting(key, defaultVal string) string {
	var val string
	err := r.db.QueryRow(`SELECT value FROM system_settings WHERE key = ?`, key).Scan(&val)
	if err != nil {
		return defaultVal
	}
	return val
}

// SetSetting saves a system configuration value.
func (r *Repository) SetSetting(key, val string) error {
	_, err := r.db.Exec(`
		INSERT INTO system_settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
	`, key, val)
	return err
}
