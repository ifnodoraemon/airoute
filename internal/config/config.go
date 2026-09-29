package config

import (
	"fmt"
	"os"
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

// Config represents the complete gateway configuration.
type Config struct {
	Server                 ServerConfig              `yaml:"server"`
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
		EnableFallback:        true,
		MaxRetries:            3,
		DefaultTimeoutSeconds: 60,
		Channels:              []model.ChannelConfig{},
		APIKeys:               []model.APIKeyConfig{},
		ModelValidation: ModelValidationConfig{
			DefaultLevel: "off",
			Rules:        []ModelValidationRule{},
		},
		apiKeysMap: make(map[string]*model.APIKeyConfig),
	}
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
