package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/BurntSushi/toml"
)

type LLM struct {
	BaseURL       string `toml:"base_url"`
	APIKey        string `toml:"api_key"`
	SkipTLSVerify bool   `toml:"skip_tls_verify"`

	RequestTimeout           string        `toml:"request_timeout"`
	IdleTimeout              string        `toml:"idle_timeout"`
	FirstByteTimeout         string        `toml:"first_byte_timeout"`
	MaxRetries               int           `toml:"max_retries"`
	RequestTimeoutDuration   time.Duration `toml:"-"`
	IdleTimeoutDuration      time.Duration `toml:"-"`
	FirstByteTimeoutDuration time.Duration `toml:"-"`
}

type Agent struct {
	SubagentTimeout         string        `toml:"subagent_timeout"`
	SubagentTimeoutDuration time.Duration `toml:"-"`
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
	Agent    Agent    `toml:"agent"`
}

const (
	defaultMode            = "sdd"
	defaultMaxConvocations = 8
	defaultTokenBudget     = int64(200000)

	defaultRequestTimeout   = "10m"
	defaultIdleTimeout      = "5m"
	defaultFirstByteTimeout = "60s"
	defaultMaxRetries       = 2
	defaultSubagentTimeout  = "10m"
)

func validMode(mode string) bool {
	return mode == "sdd" || mode == "squad"
}

func parseDurationField(key, value string) (time.Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", key, value, err)
	}
	return d, nil
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
	if v := os.Getenv("KTERMINAL_LLM_REQUEST_TIMEOUT"); v != "" {
		cfg.LLM.RequestTimeout = v
	}
	if v := os.Getenv("KTERMINAL_LLM_IDLE_TIMEOUT"); v != "" {
		cfg.LLM.IdleTimeout = v
	}
	if v := os.Getenv("KTERMINAL_LLM_FIRST_BYTE_TIMEOUT"); v != "" {
		cfg.LLM.FirstByteTimeout = v
	}
	if v := os.Getenv("KTERMINAL_LLM_MAX_RETRIES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid llm.max_retries %q: %w", v, err)
		}
		cfg.LLM.MaxRetries = n
	}
	if v := os.Getenv("KTERMINAL_AGENT_SUBAGENT_TIMEOUT"); v != "" {
		cfg.Agent.SubagentTimeout = v
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
	if cfg.LLM.RequestTimeout == "" {
		cfg.LLM.RequestTimeout = defaultRequestTimeout
	}
	if cfg.LLM.IdleTimeout == "" {
		cfg.LLM.IdleTimeout = defaultIdleTimeout
	}
	if cfg.LLM.FirstByteTimeout == "" {
		cfg.LLM.FirstByteTimeout = defaultFirstByteTimeout
	}
	if cfg.LLM.MaxRetries == 0 {
		cfg.LLM.MaxRetries = defaultMaxRetries
	}
	if cfg.LLM.MaxRetries < 0 {
		return nil, fmt.Errorf("invalid llm.max_retries %d: must be >= 0", cfg.LLM.MaxRetries)
	}
	if cfg.Agent.SubagentTimeout == "" {
		cfg.Agent.SubagentTimeout = defaultSubagentTimeout
	}
	var err error
	if cfg.LLM.RequestTimeoutDuration, err = parseDurationField("llm.request_timeout", cfg.LLM.RequestTimeout); err != nil {
		return nil, err
	}
	if cfg.LLM.IdleTimeoutDuration, err = parseDurationField("llm.idle_timeout", cfg.LLM.IdleTimeout); err != nil {
		return nil, err
	}
	if cfg.LLM.FirstByteTimeoutDuration, err = parseDurationField("llm.first_byte_timeout", cfg.LLM.FirstByteTimeout); err != nil {
		return nil, err
	}
	if cfg.Agent.SubagentTimeoutDuration, err = parseDurationField("agent.subagent_timeout", cfg.Agent.SubagentTimeout); err != nil {
		return nil, err
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
