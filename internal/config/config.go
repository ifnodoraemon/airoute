package config

import (
	"fmt"
	"os"
	"sync"

	"github.com/ifnodoraemon/nano-gateway/internal/model"
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

// Config represents the complete gateway configuration.
type Config struct {
	Server                 ServerConfig              `yaml:"server"`
	Channels               []model.ChannelConfig     `yaml:"channels"`
	VirtualKeys            []model.VirtualKeyConfig  `yaml:"virtual_keys"`
	EnableFallback         bool                      `yaml:"enable_fallback"`
	MaxRetries             int                       `yaml:"max_retries"`
	DefaultTimeoutSeconds  int                       `yaml:"default_timeout_seconds"`
	HasConfiguredKeys      bool                      `yaml:"-"`
	virtualKeysMap         map[string]*model.VirtualKeyConfig
}

// GetVirtualKey returns the VirtualKeyConfig in O(1) constant time.
func (c *Config) GetVirtualKey(key string) *model.VirtualKeyConfig {
	if c == nil || c.virtualKeysMap == nil {
		return nil
	}
	return c.virtualKeysMap[key]
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
		VirtualKeys:           []model.VirtualKeyConfig{},
		virtualKeysMap:        make(map[string]*model.VirtualKeyConfig),
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
	configMutex.Lock()
	defer configMutex.Unlock()
	if cfg != nil {
		m := make(map[string]*model.VirtualKeyConfig, len(cfg.VirtualKeys))
		for i := range cfg.VirtualKeys {
			vk := &cfg.VirtualKeys[i]
			m[vk.Key] = vk
		}
		cfg.virtualKeysMap = m
	}
	globalConfig = cfg
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
