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
	PublicURL       string `yaml:"public_url"`
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

// Config represents the complete gateway configuration.
type Config struct {
	Server                ServerConfig          `yaml:"server"`
	Admin                 AdminConfig           `yaml:"admin"`
	Auth                  AuthConfig            `yaml:"auth"`
	Storage               StorageConfig         `yaml:"storage"`
	Channels              []model.ChannelConfig `yaml:"channels"`
	APIKeys               []model.APIKeyConfig  `yaml:"api_keys"`
	ModelValidation       ModelValidationConfig `yaml:"model_validation"`
	EnableFallback        bool                  `yaml:"enable_fallback"`
	MaxRetries            int                   `yaml:"max_retries"`
	DefaultTimeoutSeconds int                   `yaml:"default_timeout_seconds"`
	HasConfiguredKeys     bool                  `yaml:"-"`
	keyMu                 sync.RWMutex          `yaml:"-"`
	apiKeysMap            map[string]*model.APIKeyConfig
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

// ActiveAPIKeyCount returns the number of in-memory API keys under the key lock.
// Callers must use this (and HasRegisteredAPIKeys) instead of reading the
// APIKeys / HasConfiguredKeys fields directly: those are rewritten by
// UpdateAPIKeys on every hot reload, so an unguarded read is a data race.
func (c *Config) ActiveAPIKeyCount() int {
	if c == nil {
		return 0
	}
	c.keyMu.RLock()
	defer c.keyMu.RUnlock()
	return len(c.APIKeys)
}

// HasRegisteredAPIKeys reports whether any API key has ever been registered in
// the gateway, even if all of them have since been disabled or deleted.
func (c *Config) HasRegisteredAPIKeys() bool {
	if c == nil {
		return false
	}
	c.keyMu.RLock()
	defer c.keyMu.RUnlock()
	return c.HasConfiguredKeys
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
			PublicURL:       os.Getenv("PUBLIC_URL"),
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
				Endpoint:  os.Getenv("STORAGE_S3_ENDPOINT"),
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

	expanded := []byte(expandConfigEnv(string(data)))
	if err := yaml.Unmarshal(expanded, cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config yaml error: %w", err)
	}

	if env := os.Getenv("REDIS_URL"); env != "" {
		cfg.Server.RedisURL = env
	}
	if env := os.Getenv("PUBLIC_URL"); env != "" && (strings.HasPrefix(env, "http://") || strings.HasPrefix(env, "https://")) {
		cfg.Server.PublicURL = env
	} else if env := os.Getenv("GATEWAY_PUBLIC_URL"); env != "" && (strings.HasPrefix(env, "http://") || strings.HasPrefix(env, "https://")) {
		cfg.Server.PublicURL = env
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
	cfg := globalConfig
	configMutex.RUnlock()
	if cfg != nil {
		return cfg
	}
	configMutex.Lock()
	defer configMutex.Unlock()
	if globalConfig == nil {
		globalConfig = DefaultConfig()
	}
	return globalConfig
}

// expandConfigEnv expands ${NAME}, the documented ${NAME:-default} form and
// $NAME references in a config document, with `$$` as the escape for a literal
// dollar sign.
//
// It deliberately does not delegate to os.ExpandEnv: that pass treats every '$'
// as a variable reference, so a secret such as `pa$$word` was silently rewritten
// to `paword` and a value like `pass$2026` to `pass2026` — changing credentials
// (admin password, channel api_key, S3 secret) with no warning at all. Values
// substituted from the environment are inserted verbatim and never re-expanded.
func expandConfigEnv(value string) string {
	var out strings.Builder
	out.Grow(len(value))

	for i := 0; i < len(value); {
		if value[i] != '$' {
			out.WriteByte(value[i])
			i++
			continue
		}

		// "$$" is the documented escape for a literal '$'.
		if i+1 < len(value) && value[i+1] == '$' {
			out.WriteByte('$')
			i += 2
			continue
		}

		// ${NAME} / ${NAME:-default}
		if i+1 < len(value) && value[i+1] == '{' {
			end := strings.IndexByte(value[i+2:], '}')
			if end < 0 {
				// Unterminated reference: keep it verbatim instead of eating it.
				out.WriteByte('$')
				i++
				continue
			}
			expr := value[i+2 : i+2+end]
			name, fallback, hasFallback := expr, "", false
			if sep := strings.Index(expr, ":-"); sep >= 0 {
				name, fallback, hasFallback = expr[:sep], expr[sep+2:], true
			}
			if envValue := os.Getenv(name); envValue != "" {
				out.WriteString(envValue)
			} else if hasFallback {
				out.WriteString(fallback)
			}
			i += 2 + end + 1
			continue
		}

		// $NAME
		if i+1 < len(value) && isConfigEnvNameStart(value[i+1]) {
			end := i + 2
			for end < len(value) && isConfigEnvNameChar(value[end]) {
				end++
			}
			out.WriteString(os.Getenv(value[i+1 : end]))
			i = end
			continue
		}

		// A '$' that starts no reference (e.g. a trailing '$', or "$1" in text)
		// stays literal rather than expanding to nothing.
		out.WriteByte('$')
		i++
	}

	return out.String()
}

// isConfigEnvNameStart reports whether b can start an environment variable name.
func isConfigEnvNameStart(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// isConfigEnvNameChar reports whether b can continue an environment variable name.
func isConfigEnvNameChar(b byte) bool {
	return isConfigEnvNameStart(b) || (b >= '0' && b <= '9')
}
