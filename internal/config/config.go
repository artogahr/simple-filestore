package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds all application configuration, persisted to workspace/config.json.
type Config struct {
	AdminPassword string   `json:"admin_password"`
	Port          int      `json:"port"`
	SecretKey     string   `json:"secret_key"`
	Folders       []string `json:"folders"`
}

// Load reads the config from path. If the file does not exist, a default
// config is created and saved. A missing SecretKey is generated automatically.
func Load(path string) (*Config, error) {
	cfg := &Config{
		AdminPassword: "changeme",
		Port:          8080,
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if err := cfg.generateSecretKey(); err != nil {
			return nil, err
		}
		return cfg, Save(path, cfg)
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if cfg.SecretKey == "" {
		if err := cfg.generateSecretKey(); err != nil {
			return nil, err
		}
		return cfg, Save(path, cfg)
	}

	return cfg, nil
}

// Save atomically writes cfg to path (write to temp file, then rename).
func Save(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// HasFolder reports whether name is a registered folder.
func (c *Config) HasFolder(name string) bool {
	for _, f := range c.Folders {
		if f == name {
			return true
		}
	}
	return false
}

// AddFolder appends name to the folders list (caller must call Save).
func (c *Config) AddFolder(name string) {
	c.Folders = append(c.Folders, name)
}

// RemoveFolder removes name from the folders list (caller must call Save).
func (c *Config) RemoveFolder(name string) {
	out := c.Folders[:0]
	for _, f := range c.Folders {
		if f != name {
			out = append(out, f)
		}
	}
	c.Folders = out
}

func (c *Config) generateSecretKey() error {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	c.SecretKey = hex.EncodeToString(b)
	return nil
}
