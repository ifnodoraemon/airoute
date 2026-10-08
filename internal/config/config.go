package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/ifnodoraemon/airoute/internal/model"
	"gopkg.in/yaml.v3"
)

// ServerConfig defines HTTP server listening parameters.
type ServerConfig struct {
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	ReadTimeoutSec  int    `yaml:"read_timeout_sec"`
	WriteTimeoutSec int    `yaml:"write_timeout_sec"`
	LogLevel        string `yaml:"log_level"`
	RedisURL        string `yaml:"redis_url"`
}

// AdminConfig defines initial administrator credentials.
type AdminConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// AuthConfig defines user authentication and governance policies.
type AuthConfig struct {
	AllowRegistration        *bool   `yaml:"allow_registration"`
	RequireEmailVerification *bool   `yaml:"require_email_verification"`
	InitialUserBalance       float64 `yaml:"initial_user_balance"`
	TokenExpiryHours         int     `yaml:"token_expiry_hours"`
}

// ModelValidationRule defines per-model format validation rules.
type ModelValidationRule struct {
	Model               string `yaml:"model" json:"model"`                             // glob/prefix pattern, e.g. "claude-3-5-sonnet*", "o1*"
	Protocol            string `yaml:"protocol" json:"protocol"`                       // "openai", "anthropic", "gemini" (optional)
	Level               string `yaml:"level" json:"level"`                             // "off", "lenient", "strict"
	DisallowTemperature bool   `yaml:"disallow_temperature" json:"disallow_temperature"` // e.g. for o1/o3 reasoning models
}

// ModelValidationConfig defines format validation configurations.
type ModelValidationConfig struct {
	DefaultLevel string                `yaml:"default_level" json:"default_level"` // "off" | "lenient" | "strict" (default: "off")
	Rules        []ModelValidationRule `yaml:"rules" json:"rules"`
}

// StorageConfig defines object storage configuration (Local / RustFS / MinIO / S3).
type StorageConfig struct {
	Driver    string   `yaml:"driver" json:"driver"`         // "local" (default) | "s3" | "rustfs"
	LocalPath string   `yaml:"local_path" json:"local_path"` // defaults to "data/storage"
	S3        S3Config `yaml:"s3" json:"s3"`
}

// S3Config defines AWS S3 / RustFS / MinIO connection parameters.
type S3Config struct {
	Endpoint        string `yaml:"endpoint" json:"endpoint"`                 // e.g. "http://rustfs:9000"
	Bucket          string `yaml:"bucket" json:"bucket"`                     // e.g. "airoute-skills"
	AccessKey       string `yaml:"access_key" json:"access_key"`             // e.g. "rustfsadmin"
	SecretKey       string `yaml:"secret_key" json:"secret_key"`             // e.g. "rustfssecret"
	Region          string `yaml:"region" json:"region"`                     // default "us-east-1"
	UseSSL          bool   `yaml:"use_ssl" json:"use_ssl"`                   // default false
	PathStyle       bool   `yaml:"path_style" json:"path_style"`             // default true
	PublicURLPrefix string `yaml:"public_url_prefix" json:"public_url_prefix"` // optional CDN/external domain prefix
}

// Config represents the complete gateway configuration.
type Config struct {
	Server                 ServerConfig              `yaml:"server"`
	Admin                  AdminConfig               `yaml:"admin"`
	Auth                   AuthConfig                `yaml:"auth"`
	Storage                StorageConfig             `yaml:"storage"`
	Channels               []model.ChannelConfig     `yaml:"channels"`
	APIKeys                []model.APIKeyConfig      `yaml:"api_keys"`
	ModelValidation        ModelValidationConfig     `yaml:"model_validation"`
	EnableFallback         bool                      `yaml:"enable_fallback"`
	MaxRetries             int                       `yaml:"max_retries"`
	DefaultTimeoutSeconds  int                       `yaml:"default_timeout_seconds"`
	HasConfiguredKeys      bool                      `yaml:"-"`
	keyMu                  sync.RWMutex              `yaml:"-"`
	apiKeysMap             map[string]*model.APIKeyConfig
}

// GetAPIKey returns the APIKeyConfig in O(1) constant time safely.
func (c *Config) GetAPIKey(key string) *model.APIKeyConfig {
	if c == nil {
		return nil
	}
	c.keyMu.RLock()
	defer c.keyMu.RUnlock()
	if c.apiKeysMap == nil {
		return nil
	}
	return c.apiKeysMap[key]
}

// UpdateAPIKeys safely updates keys and their lookup map in-place without copying lock values.
func (c *Config) UpdateAPIKeys(keys []model.APIKeyConfig, hasConfiguredKeys bool) {
	if c == nil {
		return
	}
	m := make(map[string]*model.APIKeyConfig, len(keys))
	for i := range keys {
		k := &keys[i]
		m[k.Key] = k
	}
	c.keyMu.Lock()
	c.APIKeys = keys
	c.HasConfiguredKeys = hasConfiguredKeys
	c.apiKeysMap = m
	c.keyMu.Unlock()
}


// GetValidationRule resolves the validation rule and level for a given model and protocol.
func (c *Config) GetValidationRule(modelName, protocol string) (string, *ModelValidationRule) {
	defaultLvl := "off"
	if c != nil && c.ModelValidation.DefaultLevel != "" {
		defaultLvl = c.ModelValidation.DefaultLevel
	}

	if c == nil || len(c.ModelValidation.Rules) == 0 {
		return defaultLvl, nil
	}

	modelLower := strings.ToLower(strings.TrimSpace(modelName))
	protoLower := strings.ToLower(strings.TrimSpace(protocol))

	for i := range c.ModelValidation.Rules {
		r := &c.ModelValidation.Rules[i]
		// Protocol match check if rule specifies protocol
		if r.Protocol != "" && protoLower != "" && !strings.EqualFold(r.Protocol, protoLower) {
			continue
		}

		pat := strings.ToLower(strings.TrimSpace(r.Model))
		matched := false
		if pat == "*" || pat == "" {
			matched = true
		} else if strings.HasSuffix(pat, "*") {
			prefix := strings.TrimSuffix(pat, "*")
			matched = strings.HasPrefix(modelLower, prefix)
		} else if strings.HasPrefix(pat, "*") {
			suffix := strings.TrimPrefix(pat, "*")
			matched = strings.HasSuffix(modelLower, suffix)
		} else {
			matched = (pat == modelLower)
		}

		if matched {
			lvl := r.Level
			if lvl == "" {
				lvl = defaultLvl
			}
			return lvl, r
		}
	}

	return defaultLvl, nil
}

// ResolveValidationLevel resolves the effective validation level and rule given model, protocol, and an optional key config.
// Individual API keys or explicit per-model rules can enable validation ("strict" or "lenient"), while defaulting to "off".
func (c *Config) ResolveValidationLevel(modelName, protocol string, keyCfg *model.APIKeyConfig) (string, *ModelValidationRule) {
	level, rule := c.GetValidationRule(modelName, protocol)
	if keyCfg != nil && keyCfg.FormatValidation != "" {
		level = strings.ToLower(strings.TrimSpace(keyCfg.FormatValidation))
	}
	return level, rule
}

var (
	globalConfig *Config
	configMutex  sync.RWMutex
)

func boolPtr(b bool) *bool {
	return &b
}

// DefaultConfig provides sensible defaults.
func DefaultConfig() *Config {
	redisURL := os.Getenv("REDIS_URL")
	return &Config{
		Server: ServerConfig{
			Host:            "0.0.0.0",
			Port:            8080,
			ReadTimeoutSec:  120,
			WriteTimeoutSec: 120,
			LogLevel:        "info",
			RedisURL:        redisURL,
		},
		Admin: AdminConfig{
			Username: "admin",
			Password: "admin123",
		},
		Auth: AuthConfig{
			AllowRegistration:        boolPtr(true),
			RequireEmailVerification: boolPtr(false),
			InitialUserBalance:       5.0,
			TokenExpiryHours:         168,
		},
		EnableFallback:        true,
		MaxRetries:            3,
		DefaultTimeoutSeconds: 60,
		Storage: StorageConfig{
			Driver:    "local",
			LocalPath: "data/storage",
			S3: S3Config{
				Endpoint:  "http://localhost:9000",
				Bucket:    "airoute-skills",
				AccessKey: "airoute",
				SecretKey: "airoute_cluster_secret_pass_2026",
				Region:    "us-east-1",
				UseSSL:    false,
				PathStyle: true,
			},
		},
		ModelValidation: ModelValidationConfig{
			DefaultLevel: "off",
			Rules:        []ModelValidationRule{},
		},
		apiKeysMap: make(map[string]*model.APIKeyConfig),
	}
}

// GetAdminUsername returns configured admin username, prioritizing env over config.
func (c *Config) GetAdminUsername() string {
	if u := os.Getenv("GATEWAY_ADMIN_USER"); u != "" {
		return strings.TrimSpace(u)
	}
	if u := os.Getenv("GATEWAY_ADMIN_USERNAME"); u != "" {
		return strings.TrimSpace(u)
	}
	if c != nil && c.Admin.Username != "" {
		return strings.TrimSpace(c.Admin.Username)
	}
	return "admin"
}

// GetAdminPassword returns configured admin password, prioritizing env over config.
func (c *Config) GetAdminPassword() string {
	if p := os.Getenv("GATEWAY_ADMIN_PASSWORD"); p != "" {
		return p
	}
	if c != nil && c.Admin.Password != "" {
		return c.Admin.Password
	}
	return "admin123"
}

// IsRegistrationAllowed checks if public self-registration is enabled.
func (c *Config) IsRegistrationAllowed() bool {
	if v := os.Getenv("ALLOW_REGISTRATION"); v != "" {
		return strings.EqualFold(v, "true") || v == "1"
	}
	if c != nil && c.Auth.AllowRegistration != nil {
		return *c.Auth.AllowRegistration
	}
	return true
}

// IsEmailVerificationRequired checks if email verification code is required during registration.
func (c *Config) IsEmailVerificationRequired() bool {
	if v := os.Getenv("REQUIRE_EMAIL_VERIFICATION"); v != "" {
		return strings.EqualFold(v, "true") || v == "1"
	}
	if c != nil && c.Auth.RequireEmailVerification != nil {
		return *c.Auth.RequireEmailVerification
	}
	return strings.TrimSpace(os.Getenv("SMTP_HOST")) != ""
}

// GetInitialUserBalance returns the initial balance for newly registered users.
func (c *Config) GetInitialUserBalance() float64 {
	if v := os.Getenv("INITIAL_USER_BALANCE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			return f
		}
	}
	if v := os.Getenv("DEFAULT_TRIAL_BALANCE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			return f
		}
	}
	if c != nil && c.Auth.InitialUserBalance >= 0 {
		return c.Auth.InitialUserBalance
	}
	return 5.0
}

// GetTokenExpiryHours returns JWT token expiration duration in hours.
func (c *Config) GetTokenExpiryHours() int {
	if v := os.Getenv("TOKEN_EXPIRY_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	if c != nil && c.Auth.TokenExpiryHours > 0 {
		return c.Auth.TokenExpiryHours
	}
	return 168
}

// GetStorageConfig resolves effective object storage configuration with environment overrides.
func (c *Config) GetStorageConfig() StorageConfig {
	sc := StorageConfig{
		Driver:    "local",
		LocalPath: "data/storage",
		S3: S3Config{
			Endpoint:  "http://localhost:9000",
			Bucket:    "airoute-skills",
			AccessKey: "airoute",
			SecretKey: "airoute_cluster_secret_pass_2026",
			Region:    "us-east-1",
			UseSSL:    false,
			PathStyle: true,
		},
	}
	if c != nil && c.Storage.Driver != "" {
		sc = c.Storage
	}
	if d := os.Getenv("STORAGE_DRIVER"); d != "" {
		sc.Driver = strings.ToLower(strings.TrimSpace(d))
	}
	if lp := os.Getenv("STORAGE_LOCAL_PATH"); lp != "" {
		sc.LocalPath = lp
	}
	if ep := os.Getenv("STORAGE_S3_ENDPOINT"); ep != "" {
		sc.S3.Endpoint = ep
	}
	if b := os.Getenv("STORAGE_S3_BUCKET"); b != "" {
		sc.S3.Bucket = b
	}
	if ak := os.Getenv("STORAGE_S3_ACCESS_KEY"); ak != "" {
		sc.S3.AccessKey = ak
	}
	if sk := os.Getenv("STORAGE_S3_SECRET_KEY"); sk != "" {
		sc.S3.SecretKey = sk
	}
	if reg := os.Getenv("STORAGE_S3_REGION"); reg != "" {
		sc.S3.Region = reg
	}
	if ssl := os.Getenv("STORAGE_S3_USE_SSL"); ssl != "" {
		sc.S3.UseSSL = strings.EqualFold(ssl, "true") || ssl == "1"
	}
	if ps := os.Getenv("STORAGE_S3_PATH_STYLE"); ps != "" {
		sc.S3.PathStyle = strings.EqualFold(ps, "true") || ps == "1"
	}
	if pub := os.Getenv("STORAGE_S3_PUBLIC_URL_PREFIX"); pub != "" {
		sc.S3.PublicURLPrefix = pub
	}
	return sc
}

// LoadConfig loads the configuration from a YAML file.
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default config if file not found
			SetGlobalConfig(cfg)
			return cfg, nil
		}
		return nil, fmt.Errorf("read config file error: %w", err)
	}

	expanded := []byte(os.ExpandEnv(string(data)))
	if err := yaml.Unmarshal(expanded, cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config yaml error: %w", err)
	}

	if env := os.Getenv("REDIS_URL"); env != "" {
		cfg.Server.RedisURL = env
	}

	SetGlobalConfig(cfg)
	return cfg, nil
}

// SetGlobalConfig sets the singleton config.
func SetGlobalConfig(cfg *Config) {
	if cfg != nil {
		m := make(map[string]*model.APIKeyConfig, len(cfg.APIKeys))
		for i := range cfg.APIKeys {
			k := &cfg.APIKeys[i]
			m[k.Key] = k
		}
		cfg.keyMu.Lock()
		cfg.apiKeysMap = m
		cfg.keyMu.Unlock()
	}
	configMutex.Lock()
	globalConfig = cfg
	configMutex.Unlock()
}

// GetGlobalConfig returns the singleton config.
func GetGlobalConfig() *Config {
	configMutex.RLock()
	defer configMutex.RUnlock()
	if globalConfig == nil {
		globalConfig = DefaultConfig()
	}
	return globalConfig
}
