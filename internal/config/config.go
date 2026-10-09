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

type Squad struct {
	DefaultMode     string            `toml:"default_mode"`
	MaxConvocations int               `toml:"max_convocations"`
	TokenBudget     int64             `toml:"token_budget"`
	Pins            map[string]string `toml:"pins"`
}

type Config struct {
	LLM      LLM      `toml:"llm"`
	Typesafe Typesafe `toml:"typesafe"`
	Squad    Squad    `toml:"squad"`
}

const (
	defaultMode            = "sdd"
	defaultMaxConvocations = 8
	defaultTokenBudget     = int64(200000)
)

func validMode(mode string) bool {
	return mode == "sdd" || mode == "squad"
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
	if cfg.Squad.DefaultMode == "" {
		cfg.Squad.DefaultMode = defaultMode
	}
	if cfg.Squad.MaxConvocations == 0 {
		cfg.Squad.MaxConvocations = defaultMaxConvocations
	}
	if cfg.Squad.TokenBudget == 0 {
		cfg.Squad.TokenBudget = defaultTokenBudget
	}
	if !validMode(cfg.Squad.DefaultMode) {
		return nil, fmt.Errorf("invalid squad.default_mode %q: must be \"sdd\" or \"squad\"", cfg.Squad.DefaultMode)
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
