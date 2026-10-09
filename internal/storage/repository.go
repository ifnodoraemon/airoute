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

// APIKeyRecord represents the database row for API keys.
type APIKeyRecord struct {
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
	UserID           int64     `json:"user_id"`
	Status           string    `json:"status"`
	FormatValidation string    `json:"format_validation,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// UsageLogRecord represents an audit log entry.
type UsageLogRecord struct {
	ID               int64     `json:"id"`
	TraceID          string    `json:"trace_id"`
	ChatID           string    `json:"chat_id,omitempty"`
	SessionID        string    `json:"session_id,omitempty"`
	APIKey           string    `json:"api_key"`
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

	id, err := r.db.InsertGetID(`INSERT INTO channels (name, type, base_url, api_key, models, model_mapping, protocols, priority, weight, timeout_seconds, status, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		rec.Name, rec.Type, rec.BaseURL, rec.APIKey, string(modelsBytes), string(mappingBytes), string(protocolsBytes), rec.Priority, rec.Weight, rec.TimeoutSeconds, rec.Status)
	if err != nil {
		return err
	}
	rec.ID = id
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

// ListAPIKeys returns all API keys.
func (r *Repository) ListAPIKeys() ([]*APIKeyRecord, error) {
	rows, err := r.db.Query(`SELECT id, key, tenant_id, allowed_models, rpm, tpm, budget, used_tokens, COALESCE(used_cost, 0.0), COALESCE(group_name, 'default'), COALESCE(user_id, 0), status, COALESCE(format_validation, ''), created_at, updated_at FROM api_keys ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*APIKeyRecord, 0)
	for rows.Next() {
		var rec APIKeyRecord
		var allowedJSON string
		err := rows.Scan(&rec.ID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.GroupName, &rec.UserID, &rec.Status, &rec.FormatValidation, &rec.CreatedAt, &rec.UpdatedAt)
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

// ListAPIKeysByUser returns API keys belonging to a specific user.
func (r *Repository) ListAPIKeysByUser(userID int64) ([]*APIKeyRecord, error) {
	rows, err := r.db.Query(`SELECT id, key, tenant_id, allowed_models, rpm, tpm, budget, used_tokens, COALESCE(used_cost, 0.0), COALESCE(group_name, 'default'), COALESCE(user_id, 0), status, COALESCE(format_validation, ''), created_at, updated_at FROM api_keys WHERE user_id = ? ORDER BY id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*APIKeyRecord, 0)
	for rows.Next() {
		var rec APIKeyRecord
		var allowedJSON string
		err := rows.Scan(&rec.ID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.GroupName, &rec.UserID, &rec.Status, &rec.FormatValidation, &rec.CreatedAt, &rec.UpdatedAt)
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

// CreateAPIKey inserts a new API key.
func (r *Repository) CreateAPIKey(rec *APIKeyRecord) error {
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
	if rec.UserID <= 0 {
		if rec.TenantID != "" {
			if u, _ := r.GetUserByUsername(rec.TenantID); u != nil && u.ID > 0 {
				rec.UserID = u.ID
			}
		}
		if rec.UserID <= 0 {
			if admin, _ := r.GetUserByUsername("admin"); admin != nil && admin.ID > 0 {
				rec.UserID = admin.ID
			}
		}
	}

	id, err := r.db.InsertGetID(`INSERT INTO api_keys (key, tenant_id, allowed_models, rpm, tpm, budget, used_tokens, used_cost, group_name, user_id, status, format_validation, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		rec.Key, rec.TenantID, string(allowedBytes), rec.RPM, rec.TPM, rec.Budget, rec.UsedTokens, rec.UsedCost, rec.GroupName, rec.UserID, rec.Status, rec.FormatValidation)
	if err != nil {
		return err
	}
	rec.ID = id
	return nil
}

// DeleteAPIKey deletes an API key by ID.
func (r *Repository) DeleteAPIKey(id int64) error {
	_, err := r.db.Exec(`DELETE FROM api_keys WHERE id=?`, id)
	return err
}

// GetAPIKey returns a single API key by ID.
func (r *Repository) GetAPIKey(id int64) (*APIKeyRecord, error) {
	row := r.db.QueryRow(`SELECT id, key, tenant_id, allowed_models, rpm, tpm, budget, used_tokens, COALESCE(used_cost, 0.0), COALESCE(group_name, 'default'), COALESCE(user_id, 0), status, COALESCE(format_validation, ''), created_at, updated_at FROM api_keys WHERE id = ?`, id)
	var rec APIKeyRecord
	var allowedJSON string
	if err := row.Scan(&rec.ID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.GroupName, &rec.UserID, &rec.Status, &rec.FormatValidation, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, err
	}
	if allowedJSON != "" {
		_ = json.Unmarshal([]byte(allowedJSON), &rec.AllowedModels)
	}
	return &rec, nil
}

// GetAPIKeyByKey returns a single API key by key string.
func (r *Repository) GetAPIKeyByKey(key string) (*APIKeyRecord, error) {
	row := r.db.QueryRow(`SELECT id, key, tenant_id, allowed_models, rpm, tpm, budget, used_tokens, COALESCE(used_cost, 0.0), COALESCE(group_name, 'default'), COALESCE(user_id, 0), status, COALESCE(format_validation, ''), created_at, updated_at FROM api_keys WHERE key = ?`, key)
	var rec APIKeyRecord
	var allowedJSON string
	if err := row.Scan(&rec.ID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.GroupName, &rec.UserID, &rec.Status, &rec.FormatValidation, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, err
	}
	if allowedJSON != "" {
		_ = json.Unmarshal([]byte(allowedJSON), &rec.AllowedModels)
	}
	return &rec, nil
}

// UpdateAPIKey updates an existing API key (e.g. status, RPM, tenant, allowed models, group, format_validation).
func (r *Repository) UpdateAPIKey(rec *APIKeyRecord) error {
	allowedBytes, _ := json.Marshal(rec.AllowedModels)
	if rec.Status == "" {
		rec.Status = "active"
	}
	if rec.GroupName == "" {
		rec.GroupName = "default"
	}
	_, err := r.db.Exec(`UPDATE api_keys SET tenant_id = ?, allowed_models = ?, rpm = ?, tpm = ?, budget = ?, group_name = ?, user_id = ?, status = ?, format_validation = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		rec.TenantID, string(allowedBytes), rec.RPM, rec.TPM, rec.Budget, rec.GroupName, rec.UserID, rec.Status, rec.FormatValidation, rec.ID)
	return err
}

// LogFilter defines search/filtering criteria for usage audit logs.
type LogFilter struct {
	Limit     int
	Offset    int
	StartTime string // e.g. "2026-09-26 00:00:00" or ISO8601
	EndTime   string
	TraceID   string
	ChatID    string
	SessionID string
	Model     string
	TenantID  string
	// APIKeys restricts results to records produced by these key values.
	// Filtering happens in SQL so that LIMIT pagination stays correct
	// (no post-filter truncation). By default an empty slice means "no
	// key-based restriction" (admin scope); set ScopeByAPIKeys to make
	// an empty slice fail-closed instead (non-admin scope).
	APIKeys []string
	// ScopeByAPIKeys makes an empty APIKeys list return no rows instead
	// of every row. Callers scoping non-admin users must set it.
	ScopeByAPIKeys bool
}

// RecordUsageLog records an audit log asynchronously and updates key quota/cost atomically.
// clampRunes returns s truncated to at most max runes ("" passes through).
func clampRunes(s string, max int) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// normalizeUsageLog clamps string fields to their storage column limits
// (model/channel/tenant_id VARCHAR(128), api_key VARCHAR(255) on Postgres).
// Unbounded client input — e.g. a request-body model name — must never reach
// the INSERT: a single oversized value fails the row, and in batched writes
// it takes the whole audit batch down with it.
func normalizeUsageLog(log *UsageLogRecord) {
	if log == nil {
		return
	}
	log.Model = clampRunes(log.Model, 128)
	log.Channel = clampRunes(log.Channel, 128)
	log.TenantID = clampRunes(log.TenantID, 128)
	log.APIKey = clampRunes(log.APIKey, 255)
}

func (r *Repository) RecordUsageLog(log *UsageLogRecord) error {
	normalizeUsageLog(log)
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

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	keyVal := log.APIKey

	if !log.CreatedAt.IsZero() {
		_, err = tx.Exec(`INSERT INTO usage_logs (trace_id, chat_id, session_id, api_key, tenant_id, model, channel, prompt_tokens, completion_tokens, cached_tokens, total_tokens, cost, is_off_peak, off_peak_discount, duration_ms, ttft_ms, status_code, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			log.TraceID, log.ChatID, log.SessionID, keyVal, log.TenantID, log.Model, log.Channel, log.PromptTokens, log.CompletionTokens, log.CachedTokens, log.TotalTokens, log.Cost, isOff, discount, log.DurationMs, log.TTFTMs, log.StatusCode, log.CreatedAt.UTC().Format("2006-01-02 15:04:05"))
	} else {
		_, err = tx.Exec(`INSERT INTO usage_logs (trace_id, chat_id, session_id, api_key, tenant_id, model, channel, prompt_tokens, completion_tokens, cached_tokens, total_tokens, cost, is_off_peak, off_peak_discount, duration_ms, ttft_ms, status_code) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			log.TraceID, log.ChatID, log.SessionID, keyVal, log.TenantID, log.Model, log.Channel, log.PromptTokens, log.CompletionTokens, log.CachedTokens, log.TotalTokens, log.Cost, isOff, discount, log.DurationMs, log.TTFTMs, log.StatusCode)
	}
	if err != nil {
		return err
	}

	if keyVal != "" && (log.Cost > 0 || log.TotalTokens > 0) {
		_, _ = tx.Exec(r.db.Rebind(`UPDATE api_keys SET used_cost = used_cost + ?, used_tokens = used_tokens + ?, updated_at = CURRENT_TIMESTAMP WHERE key = ?`),
			log.Cost, log.TotalTokens, keyVal)
		if log.Cost > 0 {
			res, _ := tx.Exec(r.db.Rebind(`UPDATE users SET balance = balance - ?, updated_at = CURRENT_TIMESTAMP WHERE id = (SELECT user_id FROM api_keys WHERE key = ?) AND role != 'admin'`),
				log.Cost, keyVal)
			if res != nil {
				affected, _ := res.RowsAffected()
				if affected == 0 && log.TenantID != "" {
					var isOwnerAdmin bool
					row := tx.QueryRow(r.db.Rebind(`SELECT (role = 'admin') FROM users WHERE id = (SELECT user_id FROM api_keys WHERE key = ?)`), keyVal)
					if err := row.Scan(&isOwnerAdmin); err == nil && isOwnerAdmin {
						// Admin keys are exempt from balance deduction; do not fallback to tenant
					} else {
						_, _ = tx.Exec(r.db.Rebind(`UPDATE users SET balance = balance - ?, updated_at = CURRENT_TIMESTAMP WHERE (username = ? OR email = ?) AND role != 'admin'`),
							log.Cost, log.TenantID, log.TenantID)
					}
				}
			}
		}
	} else if log.TenantID != "" && log.Cost > 0 {
		_, _ = tx.Exec(r.db.Rebind(`UPDATE users SET balance = balance - ?, updated_at = CURRENT_TIMESTAMP WHERE (username = ? OR email = ?) AND role != 'admin'`),
			log.Cost, log.TenantID, log.TenantID)
	}

	return tx.Commit()
}

// BatchRecordUsageLogs writes multiple usage logs in a single atomic transaction with aggregated cost updates.
func (r *Repository) BatchRecordUsageLogs(logs []*UsageLogRecord) error {
	if len(logs) == 0 {
		return nil
	}
	if len(logs) == 1 {
		return r.RecordUsageLog(logs[0])
	}
	for _, log := range logs {
		normalizeUsageLog(log)
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	keyCostMap := make(map[string]float64)
	keyTokensMap := make(map[string]int64)
	keyTenantMap := make(map[string]string)
	orphanTenantCostMap := make(map[string]float64)

	insertWithTimeSQL := r.db.Rebind(`INSERT INTO usage_logs (trace_id, chat_id, session_id, api_key, tenant_id, model, channel, prompt_tokens, completion_tokens, cached_tokens, total_tokens, cost, is_off_peak, off_peak_discount, duration_ms, ttft_ms, status_code, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	insertSQL := r.db.Rebind(`INSERT INTO usage_logs (trace_id, chat_id, session_id, api_key, tenant_id, model, channel, prompt_tokens, completion_tokens, cached_tokens, total_tokens, cost, is_off_peak, off_peak_discount, duration_ms, ttft_ms, status_code) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)

	stmtWithTime, err := tx.Prepare(insertWithTimeSQL)
	if err != nil {
		return err
	}
	defer stmtWithTime.Close()

	stmt, err := tx.Prepare(insertSQL)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, log := range logs {
		if log == nil {
			continue
		}
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

		keyVal := log.APIKey
		if !log.CreatedAt.IsZero() {
			_, err = stmtWithTime.Exec(log.TraceID, log.ChatID, log.SessionID, keyVal, log.TenantID, log.Model, log.Channel, log.PromptTokens, log.CompletionTokens, log.CachedTokens, log.TotalTokens, log.Cost, isOff, discount, log.DurationMs, log.TTFTMs, log.StatusCode, log.CreatedAt.UTC().Format("2006-01-02 15:04:05"))
		} else {
			_, err = stmt.Exec(log.TraceID, log.ChatID, log.SessionID, keyVal, log.TenantID, log.Model, log.Channel, log.PromptTokens, log.CompletionTokens, log.CachedTokens, log.TotalTokens, log.Cost, isOff, discount, log.DurationMs, log.TTFTMs, log.StatusCode)
		}
		if err != nil {
			return err
		}

		if keyVal != "" {
			keyCostMap[keyVal] += log.Cost
			keyTokensMap[keyVal] += int64(log.TotalTokens)
			if log.TenantID != "" {
				keyTenantMap[keyVal] = log.TenantID
			}
		} else if log.TenantID != "" && log.Cost > 0 {
			orphanTenantCostMap[log.TenantID] += log.Cost
		}
	}

	// Aggregated batch update for API keys
	updateKeySQL := r.db.Rebind(`UPDATE api_keys SET used_cost = used_cost + ?, used_tokens = used_tokens + ?, updated_at = CURRENT_TIMESTAMP WHERE key = ?`)
	updateKeyStmt, err := tx.Prepare(updateKeySQL)
	if err == nil {
		defer updateKeyStmt.Close()
		for k, cost := range keyCostMap {
			tokens := keyTokensMap[k]
			if cost > 0 || tokens > 0 {
				_, _ = updateKeyStmt.Exec(cost, tokens, k)
			}
		}
	}

	// Aggregated batch update for user wallet balances
	updateUserSQL := r.db.Rebind(`UPDATE users SET balance = balance - ?, updated_at = CURRENT_TIMESTAMP WHERE id = (SELECT user_id FROM api_keys WHERE key = ?) AND role != 'admin'`)
	updateUserStmt, err := tx.Prepare(updateUserSQL)
	if err == nil {
		defer updateUserStmt.Close()
		for k, cost := range keyCostMap {
			if cost > 0 {
				res, _ := updateUserStmt.Exec(cost, k)
				if res != nil {
					affected, _ := res.RowsAffected()
					if affected == 0 {
						var isOwnerAdmin bool
						row := tx.QueryRow(r.db.Rebind(`SELECT (role = 'admin') FROM users WHERE id = (SELECT user_id FROM api_keys WHERE key = ?)`), k)
						if err := row.Scan(&isOwnerAdmin); err == nil && isOwnerAdmin {
							// Admin keys are exempt; do not fallback to tenant
						} else {
							// Delegate to tenant deduction
							if tID := keyTenantMap[k]; tID != "" {
								orphanTenantCostMap[tID] += cost
							}
						}
					}
				}
			}
		}
	}

	// Deduct for orphan or keyless tenant requests exactly once
	if len(orphanTenantCostMap) > 0 {
		fallbackSQL := r.db.Rebind(`UPDATE users SET balance = balance - ?, updated_at = CURRENT_TIMESTAMP WHERE (username = ? OR email = ?) AND role != 'admin'`)
		for tenantID, tCost := range orphanTenantCostMap {
			if tCost > 0 {
				_, _ = tx.Exec(fallbackSQL, tCost, tenantID, tenantID)
			}
		}
	}

	return tx.Commit()
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
	if f.ScopeByAPIKeys && len(f.APIKeys) == 0 {
		// Fail-closed: key scoping was requested but no keys remain (new
		// user, revoked keys, failed lookup). Returning everything here
		// would silently widen a non-admin scope to admin visibility.
		return make([]*UsageLogRecord, 0), nil
	}

	query := `SELECT id, trace_id, COALESCE(chat_id, ''), COALESCE(session_id, ''), COALESCE(api_key, ''), COALESCE(tenant_id, ''), COALESCE(model, ''), COALESCE(channel, ''), prompt_tokens, completion_tokens, COALESCE(cached_tokens, 0), total_tokens, COALESCE(cost, 0.0), COALESCE(is_off_peak, 0), COALESCE(off_peak_discount, 1.0), duration_ms, ttft_ms, status_code, created_at FROM usage_logs WHERE 1=1`
	var args []interface{}

	// Time filters: SQLite normalizes both sides via datetime(); Postgres
	// compares natively (timestamptz implicitly casts the string parameter).
	// Rebind's string translation cannot cover the parameter-side datetime(?),
	// so the dialect branch must be explicit here.
	var startClause, endClause string
	if r.db.Dialect() == "postgres" {
		startClause = ` AND created_at >= ?`
		endClause = ` AND created_at <= ?`
	} else {
		startClause = ` AND datetime(created_at) >= datetime(?)`
		endClause = ` AND datetime(created_at) <= datetime(?)`
	}
	if f.StartTime != "" {
		query += startClause
		args = append(args, f.StartTime)
	}
	if f.EndTime != "" {
		query += endClause
		args = append(args, f.EndTime)
	}
	if f.TraceID != "" {
		query += ` AND (trace_id = ? OR trace_id LIKE ?)`
		args = append(args, f.TraceID, "%"+f.TraceID+"%")
	}
	if f.ChatID != "" {
		query += ` AND (chat_id = ? OR chat_id LIKE ?)`
		args = append(args, f.ChatID, "%"+f.ChatID+"%")
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
	if len(f.APIKeys) > 0 {
		placeholders := make([]string, 0, len(f.APIKeys))
		for _, k := range f.APIKeys {
			placeholders = append(placeholders, "?")
			args = append(args, k)
		}
		query += ` AND api_key IN (` + strings.Join(placeholders, ",") + `)`
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
		if err := rows.Scan(&l.ID, &l.TraceID, &l.ChatID, &l.SessionID, &l.APIKey, &l.TenantID, &l.Model, &l.Channel, &l.PromptTokens, &l.CompletionTokens, &l.CachedTokens, &l.TotalTokens, &l.Cost, &isOff, &l.OffPeakDiscount, &l.DurationMs, &l.TTFTMs, &l.StatusCode, &createdAt); err != nil {
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

// BatchDeleteAPIKeys deletes multiple API keys.
func (r *Repository) BatchDeleteAPIKeys(ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf(`DELETE FROM api_keys WHERE id IN (%s)`, strings.Join(placeholders, ","))
	res, err := r.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// BatchUpdateAPIKeyStatus updates status ('active'/'disabled') for multiple API keys.
func (r *Repository) BatchUpdateAPIKeyStatus(ids []int64, status string) (int64, error) {
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
	q := fmt.Sprintf(`UPDATE api_keys SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id IN (%s)`, strings.Join(placeholders, ","))
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
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM api_keys WHERE status='active'`).Scan(&stats.ActiveKeys)

	return stats, nil
}

// CountActiveKeys returns the count of active API keys in the database.
func (r *Repository) CountActiveKeys() int {
	if r == nil || r.db == nil {
		return 0
	}
	var count int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM api_keys WHERE status='active'`).Scan(&count)
	return count
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

// ToModelAPIKeys converts database APIKeyRecords to Data Plane model.APIKeyConfigs.
func (r *Repository) ToModelAPIKeys() ([]model.APIKeyConfig, error) {
	records, err := r.ListAPIKeys()
	if err != nil {
		return nil, err
	}
	var res []model.APIKeyConfig
	for _, rec := range records {
		if rec.Status != "active" {
			continue
		}
		res = append(res, model.APIKeyConfig{
			Key:              rec.Key,
			TenantID:         rec.TenantID,
			AllowedModels:    rec.AllowedModels,
			RPM:              rec.RPM,
			TPM:              rec.TPM,
			Budget:           rec.Budget,
			GroupName:        rec.GroupName,
			UserID:           rec.UserID,
			FormatValidation: rec.FormatValidation,
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
	id, err := r.db.InsertGetID(`INSERT INTO users (username, email, password_hash, role, status, balance, group_name, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		u.Username, u.Email, u.PasswordHash, u.Role, u.Status, u.Balance, u.GroupName)
	if err != nil {
		return err
	}
	u.ID = id
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

// DeleteUser removes a user by username and cleans up associated API keys.
func (r *Repository) DeleteUser(username string) error {
	u, _ := r.GetUserByUsername(username)
	if u != nil && u.ID > 0 {
		_, _ = r.db.Exec(`DELETE FROM api_keys WHERE user_id = ?`, u.ID)
	}
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
	id, err := r.db.InsertGetID(`INSERT INTO redemption_codes (code, name, amount, status, updated_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		rec.Code, rec.Name, rec.Amount, rec.Status)
	if err != nil {
		// Table doesn't have updated_at, handle gracefully
		id, err = r.db.InsertGetID(`INSERT INTO redemption_codes (code, name, amount, status) VALUES (?, ?, ?, ?)`,
			rec.Code, rec.Name, rec.Amount, rec.Status)
		if err != nil {
			return err
		}
	}
	rec.ID = id
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
	row := tx.QueryRow(r.db.Rebind(`SELECT id, code, COALESCE(name, ''), amount, COALESCE(status, 'active'), COALESCE(used_by, ''), used_at, created_at FROM redemption_codes WHERE code = ?`), code)
	if err := row.Scan(&rec.ID, &rec.Code, &rec.Name, &rec.Amount, &rec.Status, &rec.UsedBy, &usedAt, &rec.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("无效的兑换码")
		}
		return nil, err
	}

	if rec.Status != "active" {
		return nil, fmt.Errorf("该兑换码已被使用或已失效")
	}

	// Mark as used with atomic status check to prevent race-condition double redemption
	now := time.Now()
	res, err := tx.Exec(r.db.Rebind(`UPDATE redemption_codes SET status = 'used', used_by = ?, used_at = CURRENT_TIMESTAMP WHERE id = ? AND status = 'active'`), username, rec.ID)
	if err != nil {
		return nil, fmt.Errorf("更新兑换状态失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil || affected == 0 {
		return nil, fmt.Errorf("该兑换码已被使用或已失效")
	}

	// Credit user balance
	_, err = tx.Exec(r.db.Rebind(`UPDATE users SET balance = balance + ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`), rec.Amount, username)
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
	id, err := r.db.InsertGetID(`INSERT INTO recharge_orders (order_no, username, amount, currency, channel, stripe_session_id, status, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		rec.OrderNo, rec.Username, rec.Amount, rec.Currency, rec.Channel, rec.StripeSessionID, rec.Status)
	if err != nil {
		return err
	}
	rec.ID = id
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
	// Expiry comparison: SQLite normalizes via datetime(); Postgres compares
	// natively with NOW(). Rebind's string translation covers neither the
	// expires_at column nor the 'now' literal, so the branch is explicit.
	codeQuery := `SELECT id FROM verification_codes WHERE email = ? AND code = ? AND purpose = ? AND used = 0 AND datetime(expires_at) > datetime('now') ORDER BY id DESC LIMIT 1`
	if r.db.Dialect() == "postgres" {
		codeQuery = `SELECT id FROM verification_codes WHERE email = ? AND code = ? AND purpose = ? AND used = 0 AND expires_at > NOW() ORDER BY id DESC LIMIT 1`
	}
	row := r.db.QueryRow(codeQuery, email, code, purpose)
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
		// 1. Series benchmarks & wildcards (auto-match entire series)
		{Model: "DeepSeek 系列", PromptPrice: 2.0, CompletionPrice: 8.0, CacheReadPrice: 0.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "deepseek*", PromptPrice: 2.0, CompletionPrice: 8.0, CacheReadPrice: 0.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "OpenAI GPT 系列", PromptPrice: 15.0, CompletionPrice: 60.0, CacheReadPrice: 7.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gpt-*", PromptPrice: 15.0, CompletionPrice: 60.0, CacheReadPrice: 7.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "o1*", PromptPrice: 105.0, CompletionPrice: 420.0, CacheReadPrice: 52.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "o3*", PromptPrice: 7.7, CompletionPrice: 30.8, CacheReadPrice: 3.85, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "Anthropic Claude 系列", PromptPrice: 20.0, CompletionPrice: 100.0, CacheReadPrice: 2.0, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "claude-*", PromptPrice: 20.0, CompletionPrice: 100.0, CacheReadPrice: 2.0, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "Google Gemini 系列", PromptPrice: 2.0, CompletionPrice: 8.0, CacheReadPrice: 0.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gemini-*", PromptPrice: 2.0, CompletionPrice: 8.0, CacheReadPrice: 0.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "通义千问 Qwen 系列", PromptPrice: 4.0, CompletionPrice: 12.0, CacheReadPrice: 0.8, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "qwen-*", PromptPrice: 4.0, CompletionPrice: 12.0, CacheReadPrice: 0.8, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "FLUX 图像生成系列", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.10, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "flux-*", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.10, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "视频生成系列", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.50, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "cogvideo*", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.50, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "Whisper 语音识别系列", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.03, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "whisper-*", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.03, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},

		// 2. 2026 Frontier & Canonical Models
		// OpenAI 2026 (GPT-6 series & live transcribe)
		{Model: "gpt-6", PromptPrice: 25.0, CompletionPrice: 100.0, CacheReadPrice: 12.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gpt-6-luna", PromptPrice: 12.0, CompletionPrice: 48.0, CacheReadPrice: 6.0, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gpt-6.1-sol", PromptPrice: 28.0, CompletionPrice: 112.0, CacheReadPrice: 14.0, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gpt-5.5-instant", PromptPrice: 1.5, CompletionPrice: 6.0, CacheReadPrice: 0.75, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gpt-live-transcribe", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.03, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gpt-4o", PromptPrice: 18.0, CompletionPrice: 72.0, CacheReadPrice: 9.0, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},

		// Anthropic Claude 2026 (Claude 5.5 series & Fable)
		{Model: "claude-opus-5.5", PromptPrice: 30.0, CompletionPrice: 150.0, CacheReadPrice: 3.0, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "claude-sonnet-5.5", PromptPrice: 15.0, CompletionPrice: 75.0, CacheReadPrice: 1.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "claude-haiku-5.5", PromptPrice: 3.0, CompletionPrice: 15.0, CacheReadPrice: 0.3, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "claude-fable-5.1", PromptPrice: 50.0, CompletionPrice: 250.0, CacheReadPrice: 5.0, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "claude-3-7-sonnet", PromptPrice: 21.0, CompletionPrice: 105.0, CacheReadPrice: 2.1, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},

		// Google Gemini 2026 (Gemini 4 Argon & 3.8 Flash)
		{Model: "gemini-4-argon", PromptPrice: 18.0, CompletionPrice: 72.0, CacheReadPrice: 4.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gemini-3.8-flash", PromptPrice: 1.0, CompletionPrice: 4.0, CacheReadPrice: 0.25, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gemini-3.5-flash-lite", PromptPrice: 0.4, CompletionPrice: 1.6, CacheReadPrice: 0.1, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gemini-nano-banana-2.1", PromptPrice: 2.0, CompletionPrice: 8.0, CacheReadPrice: 0.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gemini-2.0-flash", PromptPrice: 0.7, CompletionPrice: 2.8, CacheReadPrice: 0.175, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gemini-1.5-pro", PromptPrice: 9.0, CompletionPrice: 36.0, CacheReadPrice: 2.25, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},

		// DeepSeek 2026 (V4-Pro & V4.1-Flash)
		{Model: "deepseek-v4-pro", PromptPrice: 4.0, CompletionPrice: 16.0, CacheReadPrice: 1.0, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "deepseek-v4.1-flash", PromptPrice: 1.5, CompletionPrice: 6.0, CacheReadPrice: 0.35, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "deepseek-r1", PromptPrice: 4.0, CompletionPrice: 16.0, CacheReadPrice: 1.0, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "deepseek-chat", PromptPrice: 2.0, CompletionPrice: 8.0, CacheReadPrice: 0.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},

		// 国内主流 2026 (Qwen 3.8 / GLM-5.3 / Doubao-Seed-2.1 / Kimi-K3)
		{Model: "qwen-3.8", PromptPrice: 5.0, CompletionPrice: 20.0, CacheReadPrice: 1.25, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "qwen-coder", PromptPrice: 3.5, CompletionPrice: 14.0, CacheReadPrice: 0.85, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "glm-5.3", PromptPrice: 6.0, CompletionPrice: 24.0, CacheReadPrice: 1.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "doubao-seed-2.1-pro", PromptPrice: 1.5, CompletionPrice: 6.0, CacheReadPrice: 0.35, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "doubao-seed-2.1-turbo", PromptPrice: 0.8, CompletionPrice: 3.2, CacheReadPrice: 0.2, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "doubao-seed-2.0", PromptPrice: 1.2, CompletionPrice: 4.8, CacheReadPrice: 0.3, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "kimi-k3", PromptPrice: 10.0, CompletionPrice: 40.0, CacheReadPrice: 2.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "kimi-latest", PromptPrice: 8.0, CompletionPrice: 32.0, CacheReadPrice: 2.0, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},

		// 多模态与检索 2026 (FLUX.1-Pro / Kling 4.0 / CogVideoX / EmbeddingGemma-2)
		{Model: "flux-1.1-pro", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.15, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "flux-1-schnell", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.05, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "kling-4.0", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.80, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "cogvideox-5b", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.80, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "cogvideox", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.50, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "embeddinggemma-2", PromptPrice: 0.08, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "whisper-large-v3-turbo", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.02, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "whisper-1", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.05, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "dall-e-3", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.28, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "tts-1", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.10, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
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

// SeedDefaultSkills ensures standard open Agent Skills exist, complying with agentskills.io standard.
func (r *Repository) SeedDefaultSkills() error {
	// Clean up legacy skills to keep it clean and focused
	_, _ = r.db.Exec(`DELETE FROM system_skills WHERE id NOT IN ('git-workflow', 'test-driven-development', 'browser-automation', 'security-audit') AND id NOT LIKE 'skill_%' AND id NOT LIKE 'custom_%'`)

	defaults := []SkillRecord{
		{
			ID:          "git-workflow",
			Name:        "Git 规范协作与代码审查工作流",
			Description: "标准化 Git 分支管理、Conventional Commits 提交规范、冲突解决与 GitHub PR 审查流",
			Category:    "dev",
			Tools:       []string{"run_command", "git", "view_file"},
			LoadingMode: "lazy",
			Author:      "Community Standard",
			Version:     "1.0.0",
			Enabled:     true,
			Manifest: `---
name: git-workflow
description: 标准化 Git 分支管理、Conventional Commits 提交规范、冲突解决与 GitHub PR 审查流。当用户需要提交代码、排查冲突、发起或审查 Pull Request 时自动激活。
category: dev
author: Community Standard
version: 1.0.0
loading_mode: lazy
allowed-tools:
  - run_command
  - git
  - view_file
---

# Git 协作与代码审查标准作业程序 (git-workflow)

规范智能体在工程协同中的 Git 操作流程，杜绝杂乱提交与破坏性操作。

## 适用场景
- 用户要求提交代码变更、整理 commit 历史
- 分支合并、rebase 与代码冲突排障
- 发起或自动化审查 Pull Request

## SOP 执行工作流
1. **工作区状态探查**：执行 run_command("git status") 与 run_command("git diff --stat")，确认所有待提交文件，严格禁止将 .env、临时文件或编译产物加入暂存区。
2. **规范化提交信息**：遵循 Conventional Commits 规范：
   - feat: 新增功能
   - fix: 缺陷修复
   - refactor: 代码重构（不改变外部行为）
   - test: 单元测试与用例补充
   - chore: 依赖更新与配置维护
3. **冲突解决策略**：先拉取远程最新主干，使用 rebase 模式合并，逐个文件对比解决冲突后执行测试验证。
4. **代码审查清单**：审查 PR 时核验改动行数、边界保护、向下兼容性与测试覆盖。
`,
		},
		{
			ID:          "test-driven-development",
			Name:        "TDD 测试驱动开发与缺陷排查",
			Description: "红-绿-重构闭环（Red-Green-Refactor）、单元测试用例构造、边界条件防御与防回归验证",
			Category:    "test",
			Tools:       []string{"run_command", "view_file", "replace_file_content"},
			LoadingMode: "lazy",
			Author:      "Kent Beck / Community",
			Version:     "1.0.0",
			Enabled:     true,
			Manifest: `---
name: test-driven-development
description: 严谨的测试驱动开发（TDD）与质量保障规范。通过“先写失败测试、最小实现、安全重构”确保代码正确性与可维护性。当用户要求实现新功能、修复 Bug 或增加单元测试时激活。
category: test
author: Kent Beck / Community
version: 1.0.0
loading_mode: lazy
allowed-tools:
  - run_command
  - view_file
  - replace_file_content
---

# TDD 测试驱动开发标准作业程序 (test-driven-development)

确保代码正确性、防止 AI 盲目修改与消除回归缺陷的黄金法则。

## 适用场景
- 实现全新业务功能或算法模块
- 复现并修复线上 Bug
- 重构现有复杂模块

## SOP 执行工作流
1. **Red（编写失败用例）**：在编写任何实现代码前，先构造一个针对新需求或 Bug 的最小失败测试，执行测试确认其失败。
2. **Green（最小化通过）**：编写最简实现代码，禁止过度设计，直到测试 100% 绿色通过。
3. **Refactor（安全重构）**：在已有测试套件全量覆盖保护下，优化命名与架构结构，消除坏味道。
4. **回归全量验证**：执行全量测试套件，确保没有破坏现有功能。
`,
		},
		{
			ID:          "browser-automation",
			Name:        "Playwright 浏览器自动化与 UI 验收",
			Description: "基于 Playwright / Puppeteer 的无头浏览器页面交互、端到端测试、状态抓取与视觉全景截图",
			Category:    "automation",
			Tools:       []string{"puppeteer_navigate", "puppeteer_screenshot", "puppeteer_click"},
			LoadingMode: "lazy",
			Author:      "Microsoft Playwright Team",
			Version:     "1.1.0",
			Enabled:     true,
			Manifest: `---
name: browser-automation
description: 基于无头浏览器的网页自动化导航、元素定位、端到端 UI 测试与全景截图。当用户需要测试网页交互、抓取动态单页应用（SPA）、验证响应式排版时激活。
category: automation
author: Microsoft Playwright Team
version: 1.1.0
loading_mode: lazy
allowed-tools:
  - puppeteer_navigate
  - puppeteer_screenshot
  - puppeteer_click
---

# 网页端到端自动化与视觉验收 (browser-automation)

赋予智能体操作与感知真实网页界面的能力，实现无头浏览器交互闭环。

## 适用场景
- 网页 UI 视觉排版自检与全景截图
- SPA 单页应用的动态数据加载验证
- 端到端表单提交与用户交互链路验收

## SOP 执行工作流
1. **导航与页面加载**：调用 puppeteer_navigate 打开目标页面，等待 networkidle 网络空闲。
2. **视图与状态捕获**：调用 puppeteer_screenshot 捕获页面视口或全景，比对关键排版元素。
3. **模拟交互与触发**：精确定位选择器，调用 puppeteer_click 执行点击或输入。
4. **控制台与网络审计**：捕获页面所有 Console 报错及未处理的 Promise Rejection。
`,
		},
		{
			ID:          "security-audit",
			Name:        "生产级代码安全审计与凭据防护",
			Description: "静态代码扫描、OWASP Top 10 漏洞自检、硬编码 API Key/Token 扫描、SQL 注入与 XSS 防护",
			Category:    "security",
			Tools:       []string{"view_file", "grep", "run_command"},
			LoadingMode: "lazy",
			Author:      "OWASP Community",
			Version:     "1.0.0",
			Enabled:     true,
			Manifest: `---
name: security-audit
description: 全面的软件安全合规与静态分析。覆盖 OWASP Top 10、敏感凭据泄露扫描、反注入防护与依赖漏洞识别。当需要进行发布前安全评估、排查潜在漏洞时激活。
category: security
author: OWASP Community
version: 1.0.0
loading_mode: lazy
allowed-tools:
  - view_file
  - grep
  - run_command
---

# 生产级代码安全审计标准作业程序 (security-audit)

代码合入生产前的静态安全合规防线。

## 适用场景
- 发布前代码审计与合规评估
- 检索历史代码中的硬编码凭据与敏感信息
- 排查潜在的 SQL 注入、XSS 与越权访问漏洞

## SOP 执行工作流
1. **凭据与秘钥扫描**：使用静态规则扫描全库，检测是否存在以 sk-、Bearer、私钥等特征命名的明文硬编码。
2. **SQL 与命令注入审计**：检查数据库操作是否全部采用预编译参数化绑定（Parameterized Queries），禁止拼接未经消毒的用户输入。
3. **越权与鉴权边界校验**：检查接口层是否具备完整的 Session/Token 与角色鉴权守卫。
4. **生成审计整改清单**：输出结构化风险评估报告，明确危险等级与修复代码片段。
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

// SeedDefaultMCPServers ensures default curated ModelScope-style MCP servers exist.
func (r *Repository) SeedDefaultMCPServers() error {
	defaults := []MCPServerRecord{
		{
			ID:          "airoute-gateway",
			Name:        "Airoute 网关原生核心服务",
			Description: "企业级 AI 网关核心管控与路由服务，暴露集群熔断、模型拓扑、数据脱敏与智能仲裁",
			Category:    "ops",
			Transport:   "sse",
			Endpoint:    "/mcp/sse",
			Status:      "online",
			Author:      "Airoute Official",
			Version:     "1.2.0",
			Tools:       []string{"airoute_cluster_status", "airoute_model_topology", "airoute_data_redact", "airoute_recommend_model", "airoute_query_logs"},
			Prompts:     []string{"cluster_health_report", "route_optimization_guide"},
			Resources:   []string{"airoute://topology/matrix", "airoute://metrics/sli"},
			Enabled:     true,
		},
		{
			ID:          "modelscope-search",
			Name:        "ModelScope 联网检索与正文提取",
			Description: "魔搭社区与开源生态精选多源 Web 检索、动态抓取与结构化 Markdown 提炼",
			Category:    "search",
			Transport:   "sse",
			Endpoint:    "https://mcp.modelscope.cn/servers/search/sse",
			Status:      "online",
			Author:      "ModelScope",
			Version:     "2.1.0",
			Tools:       []string{"airoute_deep_search", "web_fetch_markdown", "academic_paper_search"},
			Prompts:     []string{"deep_research_brief"},
			Resources:   []string{"search://history"},
			Enabled:     true,
		},
		{
			ID:          "github-mcp",
			Name:        "GitHub 研发协作协议服务",
			Description: "仓库代码探查、Pull Request 代码审查、Issue 追踪与 GitHub Actions 工作流联动",
			Category:    "dev",
			Transport:   "stdio",
			Endpoint:    "npx -y @modelcontextprotocol/server-github",
			Status:      "active",
			Author:      "GitHub / Anthropic",
			Version:     "1.0.4",
			Tools:       []string{"search_repositories", "create_issue", "get_file_contents", "create_pull_request"},
			Prompts:     []string{"pull_request_review_summary"},
			Resources:   []string{"github://repos/recent"},
			Enabled:     true,
		},
		{
			ID:          "postgres-mcp",
			Name:        "PostgreSQL 企业数据安全审计服务",
			Description: "企业级关系型数据库 Schema 自动探测、只读隔离查询与慢 SQL 诊断",
			Category:    "db",
			Transport:   "stdio",
			Endpoint:    "npx -y @modelcontextprotocol/server-postgres postgresql://localhost/db",
			Status:      "active",
			Author:      "ModelContextProtocol",
			Version:     "0.9.2",
			Tools:       []string{"read_query", "list_tables", "describe_table", "airoute_sql_security_check"},
			Prompts:     []string{"explain_slow_query"},
			Resources:   []string{"db://schema/public"},
			Enabled:     true,
		},
		{
			ID:          "browser-fetch-mcp",
			Name:        "Puppeteer 无头浏览器渲染服务",
			Description: "动态 JS 页面无头渲染、网页全景截图与 SPA 应用深度爬取",
			Category:    "search",
			Transport:   "stdio",
			Endpoint:    "npx -y @modelcontextprotocol/server-puppeteer",
			Status:      "active",
			Author:      "Puppeteer Community",
			Version:     "1.3.1",
			Tools:       []string{"puppeteer_navigate", "puppeteer_screenshot", "puppeteer_click", "puppeteer_evaluate"},
			Prompts:     []string{"web_page_inspect"},
			Resources:   []string{"browser://active_pages"},
			Enabled:     true,
		},
		{
			ID:          "sequential-thinking",
			Name:        "Sequential Thinking 动态思维链推理",
			Description: "Anthropic 官方深度反思与长思维链问题解决服务，提供动态假设验证与复杂逻辑分步演进",
			Category:    "ai",
			Transport:   "stdio",
			Endpoint:    "npx -y @modelcontextprotocol/server-sequential-thinking",
			Status:      "active",
			Author:      "Anthropic Official",
			Version:     "0.6.2",
			Tools:       []string{"sequentialthinking"},
			Prompts:     []string{"deep_reasoning_prompt"},
			Resources:   []string{"thinking://history"},
			Enabled:     true,
		},
		{
			ID:          "brave-search",
			Name:        "Brave Search 全球实时网络检索",
			Description: "官方无追踪隐私搜索协议服务，提供实时新闻、技术文章与结构化网页正文索引",
			Category:    "search",
			Transport:   "stdio",
			Endpoint:    "npx -y @modelcontextprotocol/server-brave-search",
			Status:      "active",
			Author:      "Brave Software",
			Version:     "1.0.2",
			Tools:       []string{"brave_web_search", "brave_local_search"},
			Prompts:     []string{"search_enrichment"},
			Resources:   []string{"search://trending"},
			Enabled:     true,
		},
		{
			ID:          "feishu-lark-mcp",
			Name:        "飞书 / Lark 办公智能体协议服务",
			Description: "飞书多维表格读写、知识库文档交互、群消息卡片推达与审批流联动",
			Category:    "productivity",
			Transport:   "sse",
			Endpoint:    "https://open.feishu.cn/mcp/v1/sse",
			Status:      "standby",
			Author:      "Feishu Open Platform",
			Version:     "2.0.0",
			Tools:       []string{"feishu_send_card", "feishu_read_wiki", "feishu_bitable_query"},
			Prompts:     []string{"format_weekly_digest"},
			Resources:   []string{"feishu://wiki/root"},
			Enabled:     false,
		},
		{
			ID:          "docker-k8s-mcp",
			Name:        "Docker / K8s 容器与集群探针",
			Description: "容器生命周期管理、Pod 运行状态监控与分布式日志排查",
			Category:    "ops",
			Transport:   "stdio",
			Endpoint:    "npx -y @modelcontextprotocol/server-docker",
			Status:      "active",
			Author:      "DevOps Community",
			Version:     "1.1.0",
			Tools:       []string{"docker_ps", "docker_logs", "docker_restart_container"},
			Prompts:     []string{"troubleshoot_container"},
			Resources:   []string{"docker://containers/list"},
			Enabled:     true,
		},
		{
			ID:          "memory-graph",
			Name:        "Knowledge Graph 跨会话长期记忆",
			Description: "Anthropic 官方知识图谱持久化记忆服务，支持实体、关联与观测沉淀，实现智能体跨对话状态连续性",
			Category:    "ai",
			Transport:   "stdio",
			Endpoint:    "npx -y @modelcontextprotocol/server-memory",
			Status:      "active",
			Author:      "Anthropic Official",
			Version:     "1.0.2",
			Tools:       []string{"create_entities", "create_relations", "add_observations", "read_graph", "search_nodes"},
			Prompts:     []string{"summarize_entity_context"},
			Resources:   []string{"memory://graph/entities"},
			Enabled:     true,
		},
		{
			ID:          "git-mcp",
			Name:        "Git 本地版本控制与代码审计",
			Description: "官方 Git 仓库管理服务，提供安全只读 diff、提交树审查、分支检出与历史变更溯源",
			Category:    "dev",
			Transport:   "stdio",
			Endpoint:    "npx -y @modelcontextprotocol/server-git",
			Status:      "active",
			Author:      "ModelContextProtocol",
			Version:     "1.1.0",
			Tools:       []string{"git_status", "git_diff", "git_commit", "git_log", "git_create_branch", "git_checkout"},
			Prompts:     []string{"git_commit_summary"},
			Resources:   []string{"git://history/recent"},
			Enabled:     true,
		},
		{
			ID:          "filesystem-mcp",
			Name:        "Filesystem 沙箱文件安全交互",
			Description: "官方受限目录安全文件操作服务，支持安全读写、目录树构建、递归搜索与多文件聚合",
			Category:    "dev",
			Transport:   "stdio",
			Endpoint:    "npx -y @modelcontextprotocol/server-filesystem /workspace",
			Status:      "active",
			Author:      "Anthropic Official",
			Version:     "1.2.1",
			Tools:       []string{"read_file", "read_multiple_files", "write_file", "list_directory", "directory_tree", "search_files"},
			Prompts:     []string{"review_file_diff"},
			Resources:   []string{"file://workspace"},
			Enabled:     true,
		},
		{
			ID:          "fetch-mcp",
			Name:        "Fetch 快速网页提取与 Markdown 转换",
			Description: "官方超轻量网页内容抓取器，零浏览器开销将任意 HTTP/HTTPS 页面转换为紧凑 LLM 结构化 Markdown",
			Category:    "search",
			Transport:   "stdio",
			Endpoint:    "npx -y @modelcontextprotocol/server-fetch",
			Status:      "active",
			Author:      "ModelContextProtocol",
			Version:     "1.0.1",
			Tools:       []string{"fetch_markdown", "fetch_raw"},
			Prompts:     []string{"summarize_webpage"},
			Resources:   []string{"web://recent_fetches"},
			Enabled:     true,
		},
		{
			ID:          "sqlite-mcp",
			Name:        "SQLite 轻量嵌入式分析数据库",
			Description: "官方嵌入式 SQLite 数据库交互服务，支持本地数据快速探索、表结构审计与只读 SQL 执行",
			Category:    "db",
			Transport:   "stdio",
			Endpoint:    "npx -y @modelcontextprotocol/server-sqlite --db-path ./data/gateway.db",
			Status:      "active",
			Author:      "ModelContextProtocol",
			Version:     "1.0.0",
			Tools:       []string{"read_query", "list_tables", "describe_table"},
			Prompts:     []string{"sqlite_schema_analysis"},
			Resources:   []string{"sqlite://schema/main"},
			Enabled:     true,
		},
		{
			ID:          "sentry-mcp",
			Name:        "Sentry 生产异常监控与错误追踪",
			Description: "Sentry 官方错误监控服务，允许智能体直接查询生产链路崩溃、堆栈信息与问题事件定位",
			Category:    "ops",
			Transport:   "stdio",
			Endpoint:    "npx -y @modelcontextprotocol/server-sentry",
			Status:      "standby",
			Author:      "Sentry Official",
			Version:     "1.0.0",
			Tools:       []string{"get_issue", "search_issues", "get_event_stack_trace"},
			Prompts:     []string{"diagnose_issue_root_cause"},
			Resources:   []string{"sentry://issues/unresolved"},
			Enabled:     false,
		},
	}

	for _, s := range defaults {
		toolsJSON, _ := json.Marshal(s.Tools)
		promptsJSON, _ := json.Marshal(s.Prompts)
		resourcesJSON, _ := json.Marshal(s.Resources)
		enabledInt := 0
		if s.Enabled {
			enabledInt = 1
		}
		_, _ = r.db.Exec(`
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
				resources = excluded.resources
		`, s.ID, s.Name, s.Description, s.Category, s.Transport, s.Endpoint, s.Status, s.Author, s.Version, string(toolsJSON), string(promptsJSON), string(resourcesJSON), s.EnvVars, enabledInt)
	}
	return nil
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
