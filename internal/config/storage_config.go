package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// StorageConfig defines object storage configuration (Local / RustFS / MinIO / S3).
type StorageConfig struct {
	Driver    string   `yaml:"driver" json:"driver"`         // "local" (default) | "s3" | "rustfs"
	LocalPath string   `yaml:"local_path" json:"local_path"` // defaults to "data/storage"
	S3        S3Config `yaml:"s3" json:"s3"`
}

// S3Config defines AWS S3 / RustFS / MinIO connection parameters.
type S3Config struct {
	Endpoint        string `yaml:"endpoint" json:"endpoint"`                   // e.g. "http://rustfs:9000"
	Bucket          string `yaml:"bucket" json:"bucket"`                       // e.g. "airoute-skills"
	AccessKey       string `yaml:"access_key" json:"access_key"`               // e.g. "rustfsadmin"
	SecretKey       string `yaml:"secret_key" json:"secret_key"`               // e.g. "rustfssecret"
	Region          string `yaml:"region" json:"region"`                       // default "us-east-1"
	UseSSL          bool   `yaml:"use_ssl" json:"use_ssl"`                     // default false
	PathStyle       bool   `yaml:"path_style" json:"path_style"`               // default true
	PublicURLPrefix string `yaml:"public_url_prefix" json:"public_url_prefix"` // optional CDN/external domain prefix
}

// defaultStorageConfig is the safe baseline: local disk storage with no
// credentials. It used to contain hardcoded S3 access/secret values that also
// appear in this repository, so an S3 deployment that forgot to configure
// credentials silently authenticated with a published secret.
func defaultStorageConfig() StorageConfig {
	return StorageConfig{
		Driver:    "local",
		LocalPath: "data/storage",
		S3: S3Config{
			Endpoint:  os.Getenv("STORAGE_S3_ENDPOINT"),
			Bucket:    "airoute-skills",
			Region:    "us-east-1",
			UseSSL:    false,
			PathStyle: true,
		},
	}
}

// UnmarshalYAML merges a partial `storage:` block over the safe defaults.
//
// Previously GetStorageConfig() replaced the whole resolved configuration with
// c.Storage only when `driver` happened to be set, so a config that overrode a
// single field (for example `s3: {bucket: my-bucket}`) silently lost it, and a
// block without `driver` was discarded entirely. Decoding field-by-field keeps
// every omitted key at its default.
func (s *StorageConfig) UnmarshalYAML(value *yaml.Node) error {
	type rawS3 struct {
		Endpoint        *string `yaml:"endpoint"`
		Bucket          *string `yaml:"bucket"`
		AccessKey       *string `yaml:"access_key"`
		SecretKey       *string `yaml:"secret_key"`
		Region          *string `yaml:"region"`
		UseSSL          *bool   `yaml:"use_ssl"`
		PathStyle       *bool   `yaml:"path_style"`
		PublicURLPrefix *string `yaml:"public_url_prefix"`
	}
	type rawStorage struct {
		Driver    *string `yaml:"driver"`
		LocalPath *string `yaml:"local_path"`
		S3        *rawS3  `yaml:"s3"`
	}

	var raw rawStorage
	if err := value.Decode(&raw); err != nil {
		return err
	}

	resolved := defaultStorageConfig()
	if raw.Driver != nil {
		resolved.Driver = normalizeStorageDriver(*raw.Driver)
	}
	if raw.LocalPath != nil && strings.TrimSpace(*raw.LocalPath) != "" {
		resolved.LocalPath = strings.TrimSpace(*raw.LocalPath)
	}
	if raw.S3 != nil {
		if raw.S3.Endpoint != nil {
			resolved.S3.Endpoint = strings.TrimSpace(*raw.S3.Endpoint)
		}
		if raw.S3.Bucket != nil && strings.TrimSpace(*raw.S3.Bucket) != "" {
			resolved.S3.Bucket = strings.TrimSpace(*raw.S3.Bucket)
		}
		if raw.S3.AccessKey != nil {
			resolved.S3.AccessKey = strings.TrimSpace(*raw.S3.AccessKey)
		}
		if raw.S3.SecretKey != nil {
			resolved.S3.SecretKey = strings.TrimSpace(*raw.S3.SecretKey)
		}
		if raw.S3.Region != nil && strings.TrimSpace(*raw.S3.Region) != "" {
			resolved.S3.Region = strings.TrimSpace(*raw.S3.Region)
		}
		if raw.S3.UseSSL != nil {
			resolved.S3.UseSSL = *raw.S3.UseSSL
		}
		if raw.S3.PathStyle != nil {
			resolved.S3.PathStyle = *raw.S3.PathStyle
		}
		if raw.S3.PublicURLPrefix != nil {
			resolved.S3.PublicURLPrefix = strings.TrimSpace(*raw.S3.PublicURLPrefix)
		}
	}

	*s = resolved
	return nil
}

func normalizeStorageDriver(driver string) string {
	return strings.ToLower(strings.TrimSpace(driver))
}

// mergeStorageConfig overlays a programmatically built StorageConfig on the
// defaults. YAML input is already merged by UnmarshalYAML; this covers callers
// that construct a partial Config in code (tests, embedders). Empty strings mean
// "not configured"; an explicitly set driver means the block is present, so its
// bool fields are honoured.
func mergeStorageConfig(base, over StorageConfig) StorageConfig {
	blockPresent := over.Driver != "" || over.LocalPath != "" || over.S3 != (S3Config{})
	if !blockPresent {
		return base
	}
	if d := normalizeStorageDriver(over.Driver); d != "" {
		base.Driver = d
	}
	if strings.TrimSpace(over.LocalPath) != "" {
		base.LocalPath = strings.TrimSpace(over.LocalPath)
	}
	if over.S3.Endpoint != "" {
		base.S3.Endpoint = over.S3.Endpoint
	}
	if over.S3.Bucket != "" {
		base.S3.Bucket = over.S3.Bucket
	}
	if over.S3.AccessKey != "" {
		base.S3.AccessKey = over.S3.AccessKey
	}
	if over.S3.SecretKey != "" {
		base.S3.SecretKey = over.S3.SecretKey
	}
	if over.S3.Region != "" {
		base.S3.Region = over.S3.Region
	}
	if over.S3.PublicURLPrefix != "" {
		base.S3.PublicURLPrefix = over.S3.PublicURLPrefix
	}
	// Booleans have no "unset" state outside YAML, so honour them only when the
	// caller declared the block explicitly (driver/local_path/s3 set).
	if blockPresent {
		base.S3.UseSSL = over.S3.UseSSL
		base.S3.PathStyle = over.S3.PathStyle
	}
	return base
}

// GetStorageConfig resolves effective object storage configuration with environment overrides.
func (c *Config) GetStorageConfig() StorageConfig {
	sc := defaultStorageConfig()
	if c != nil {
		sc = mergeStorageConfig(sc, c.Storage)
	}
	if d := os.Getenv("STORAGE_DRIVER"); d != "" {
		sc.Driver = normalizeStorageDriver(d)
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

// Validate rejects storage configurations that cannot work. In particular a
// remote driver without explicit credentials must fail loudly instead of falling
// back to a local directory or (as before) to a repository-published default
// secret; and an unknown driver must not silently behave like `local`.
func (sc StorageConfig) Validate() error {
	switch sc.Driver {
	case "", "local":
		if strings.TrimSpace(sc.LocalPath) == "" {
			return fmt.Errorf("storage driver %q requires a non-empty local_path (STORAGE_LOCAL_PATH)", "local")
		}
		return nil
	case "s3", "rustfs":
		if strings.TrimSpace(sc.S3.Endpoint) == "" {
			return fmt.Errorf("storage driver %q requires STORAGE_S3_ENDPOINT (or storage.s3.endpoint)", sc.Driver)
		}
		if strings.TrimSpace(sc.S3.AccessKey) == "" || strings.TrimSpace(sc.S3.SecretKey) == "" {
			return fmt.Errorf("storage driver %q requires STORAGE_S3_ACCESS_KEY and STORAGE_S3_SECRET_KEY; no default credential is shipped", sc.Driver)
		}
		return nil
	default:
		return fmt.Errorf("unknown storage driver %q: want local, s3 or rustfs", sc.Driver)
	}
}
