package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type LLM struct {
	BaseURL       string `toml:"base_url"`
	APIKey        string `toml:"api_key"`
	SkipTLSVerify bool   `toml:"skip_tls_verify"`
}

type Typesafe struct {
	APIKey string `toml:"api_key"`
}

type Config struct {
	LLM      LLM      `toml:"llm"`
	Typesafe Typesafe `toml:"typesafe"`
}

func Dir() string {
	if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
		return filepath.Join(base, "kterminal")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".kterminal"
	}
	return filepath.Join(home, ".config", "kterminal")
}

func Path() string {
	return filepath.Join(Dir(), "config.toml")
}

func Exists() bool {
	_, err := os.Stat(Path())
	return err == nil
}

func Load() (*Config, error) {
	cfg := &Config{}
	if data, err := os.ReadFile(Path()); err == nil {
		if err := toml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse %s: %w", Path(), err)
		}
	}
	if v := os.Getenv("KTERMINAL_LLM_BASE_URL"); v != "" {
		cfg.LLM.BaseURL = v
	}
	if v := os.Getenv("KTERMINAL_LLM_API_KEY"); v != "" {
		cfg.LLM.APIKey = v
	}
	if v := os.Getenv("TYPESAFE_API_KEY"); v != "" {
		cfg.Typesafe.APIKey = v
	}
	return cfg, nil
}

func (c *Config) Save() error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	data, err := toml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), data, 0o600)
}

func (c *Config) Ready() bool {
	return c.LLM.BaseURL != "" && c.LLM.APIKey != ""
}
