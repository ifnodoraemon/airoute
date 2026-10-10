package config

import (
	"os"
	"strings"
)

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

// GetStorageConfig resolves effective object storage configuration with environment overrides.
func (c *Config) GetStorageConfig() StorageConfig {
	sc := StorageConfig{
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
