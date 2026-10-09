package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	xdg := filepath.Join(dir, "xdg")
	if err := os.MkdirAll(filepath.Join(xdg, "kterminal"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if content == "" {
		return filepath.Join(xdg, "kterminal", "config.toml")
	}
	path := filepath.Join(xdg, "kterminal", "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadSquadDefaults(t *testing.T) {
	writeTestConfig(t, "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Squad.DefaultMode != "sdd" {
		t.Errorf("DefaultMode = %q, want \"sdd\"", cfg.Squad.DefaultMode)
	}
	if cfg.Squad.MaxConvocations != 8 {
		t.Errorf("MaxConvocations = %d, want 8", cfg.Squad.MaxConvocations)
	}
	if cfg.Squad.TokenBudget != 200000 {
		t.Errorf("TokenBudget = %d, want 200000", cfg.Squad.TokenBudget)
	}
}

func TestLoadSquadExplicit(t *testing.T) {
	writeTestConfig(t, `
[llm]
base_url = "http://localhost"
api_key = "key"

[squad]
default_mode = "squad"
max_convocations = 5
token_budget = 50000

[squad.pins]
architect = "glm-5.3"
qa = "gpt-4o-mini"
`)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Squad.DefaultMode != "squad" {
		t.Errorf("DefaultMode = %q, want \"squad\"", cfg.Squad.DefaultMode)
	}
	if cfg.Squad.MaxConvocations != 5 {
		t.Errorf("MaxConvocations = %d, want 5", cfg.Squad.MaxConvocations)
	}
	if cfg.Squad.TokenBudget != 50000 {
		t.Errorf("TokenBudget = %d, want 50000", cfg.Squad.TokenBudget)
	}
	if cfg.Squad.Pins["architect"] != "glm-5.3" {
		t.Errorf("Pins[architect] = %q, want \"glm-5.3\"", cfg.Squad.Pins["architect"])
	}
	if cfg.Squad.Pins["qa"] != "gpt-4o-mini" {
		t.Errorf("Pins[qa] = %q, want \"gpt-4o-mini\"", cfg.Squad.Pins["qa"])
	}
}

func TestLoadSquadPartialDefaults(t *testing.T) {
	writeTestConfig(t, `
[squad]
default_mode = "squad"
`)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Squad.DefaultMode != "squad" {
		t.Errorf("DefaultMode = %q, want \"squad\"", cfg.Squad.DefaultMode)
	}
	if cfg.Squad.MaxConvocations != 8 {
		t.Errorf("MaxConvocations = %d, want 8", cfg.Squad.MaxConvocations)
	}
	if cfg.Squad.TokenBudget != 200000 {
		t.Errorf("TokenBudget = %d, want 200000", cfg.Squad.TokenBudget)
	}
}

func TestLoadSquadInvalidMode(t *testing.T) {
	writeTestConfig(t, `
[squad]
default_mode = "invalid"
`)
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid default_mode, got nil")
	}
}

func TestLoadSquadPinsParsed(t *testing.T) {
	writeTestConfig(t, `
[squad.pins]
architect = "glm-5.3"
backend = "deepseek-v3"
frontend = "claude-sonnet"
`)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Squad.Pins) != 3 {
		t.Fatalf("Pins len = %d, want 3", len(cfg.Squad.Pins))
	}
	want := map[string]string{
		"architect": "glm-5.3",
		"backend":   "deepseek-v3",
		"frontend":  "claude-sonnet",
	}
	for k, v := range want {
		if cfg.Squad.Pins[k] != v {
			t.Errorf("Pins[%s] = %q, want %q", k, cfg.Squad.Pins[k], v)
		}
	}
}

func TestLoadSquadExistingSectionsUntouched(t *testing.T) {
	writeTestConfig(t, `
[llm]
base_url = "http://localhost:8080"
api_key = "secret"
skip_tls_verify = true

[typesafe]
api_key = "ts-key"
`)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.BaseURL != "http://localhost:8080" {
		t.Errorf("LLM.BaseURL = %q, want \"http://localhost:8080\"", cfg.LLM.BaseURL)
	}
	if cfg.LLM.APIKey != "secret" {
		t.Errorf("LLM.APIKey = %q, want \"secret\"", cfg.LLM.APIKey)
	}
	if !cfg.LLM.SkipTLSVerify {
		t.Error("LLM.SkipTLSVerify = false, want true")
	}
	if cfg.Typesafe.APIKey != "ts-key" {
		t.Errorf("Typesafe.APIKey = %q, want \"ts-key\"", cfg.Typesafe.APIKey)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	writeTestConfig(t, "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Squad.DefaultMode = "squad"
	cfg.Squad.MaxConvocations = 12
	cfg.Squad.TokenBudget = 300000
	cfg.Squad.Pins = map[string]string{
		"architect": "glm-5.3",
		"qa":        "gpt-4o-mini",
	}
	cfg.LLM.BaseURL = "http://localhost"
	cfg.LLM.APIKey = "key"
	cfg.Typesafe.APIKey = "ts-key"

	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Squad.DefaultMode != "squad" {
		t.Errorf("DefaultMode = %q, want \"squad\"", loaded.Squad.DefaultMode)
	}
	if loaded.Squad.MaxConvocations != 12 {
		t.Errorf("MaxConvocations = %d, want 12", loaded.Squad.MaxConvocations)
	}
	if loaded.Squad.TokenBudget != 300000 {
		t.Errorf("TokenBudget = %d, want 300000", loaded.Squad.TokenBudget)
	}
	if loaded.Squad.Pins["architect"] != "glm-5.3" {
		t.Errorf("Pins[architect] = %q, want \"glm-5.3\"", loaded.Squad.Pins["architect"])
	}
	if loaded.Squad.Pins["qa"] != "gpt-4o-mini" {
		t.Errorf("Pins[qa] = %q, want \"gpt-4o-mini\"", loaded.Squad.Pins["qa"])
	}
	if loaded.LLM.BaseURL != "http://localhost" {
		t.Errorf("LLM.BaseURL = %q, want \"http://localhost\"", loaded.LLM.BaseURL)
	}
	if loaded.LLM.APIKey != "key" {
		t.Errorf("LLM.APIKey = %q, want \"key\"", loaded.LLM.APIKey)
	}
	if loaded.Typesafe.APIKey != "ts-key" {
		t.Errorf("Typesafe.APIKey = %q, want \"ts-key\"", loaded.Typesafe.APIKey)
	}
}

func TestSaveLoadRoundTripDefaults(t *testing.T) {
	writeTestConfig(t, "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Squad.DefaultMode != "sdd" {
		t.Errorf("DefaultMode = %q, want \"sdd\"", loaded.Squad.DefaultMode)
	}
	if loaded.Squad.MaxConvocations != 8 {
		t.Errorf("MaxConvocations = %d, want 8", loaded.Squad.MaxConvocations)
	}
	if loaded.Squad.TokenBudget != 200000 {
		t.Errorf("TokenBudget = %d, want 200000", loaded.Squad.TokenBudget)
	}
}

func TestSavePreservesSquadSection(t *testing.T) {
	writeTestConfig(t, "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Squad.DefaultMode = "squad"
	cfg.Squad.MaxConvocations = 6
	cfg.Squad.TokenBudget = 100000
	cfg.Squad.Pins = map[string]string{"architect": "glm-5.3"}

	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "[squad]") {
		t.Error("saved config missing [squad] section")
	}
	if !strings.Contains(content, "default_mode = \"squad\"") {
		t.Error("saved config missing default_mode")
	}
	if !strings.Contains(content, "max_convocations = 6") {
		t.Error("saved config missing max_convocations")
	}
	if !strings.Contains(content, "token_budget = 100000") {
		t.Error("saved config missing token_budget")
	}
	if !strings.Contains(content, "[squad.pins]") {
		t.Error("saved config missing [squad.pins] section")
	}
	if !strings.Contains(content, "architect = \"glm-5.3\"") {
		t.Error("saved config missing pin entry")
	}
}

func TestLoadResilienceDefaults(t *testing.T) {
	writeTestConfig(t, "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.RequestTimeoutDuration != 10*time.Minute {
		t.Errorf("RequestTimeoutDuration = %v, want 10m", cfg.LLM.RequestTimeoutDuration)
	}
	if cfg.LLM.IdleTimeoutDuration != 5*time.Minute {
		t.Errorf("IdleTimeoutDuration = %v, want 5m", cfg.LLM.IdleTimeoutDuration)
	}
	if cfg.LLM.FirstByteTimeoutDuration != 60*time.Second {
		t.Errorf("FirstByteTimeoutDuration = %v, want 60s", cfg.LLM.FirstByteTimeoutDuration)
	}
	if cfg.LLM.MaxRetries != 2 {
		t.Errorf("MaxRetries = %d, want 2", cfg.LLM.MaxRetries)
	}
	if cfg.Agent.SubagentTimeoutDuration != 10*time.Minute {
		t.Errorf("SubagentTimeoutDuration = %v, want 10m", cfg.Agent.SubagentTimeoutDuration)
	}
}

func TestLoadResilienceDurationsParsed(t *testing.T) {
	writeTestConfig(t, `
[llm]
request_timeout = "90s"
idle_timeout = "10m"
first_byte_timeout = "0s"
max_retries = 5

[agent]
subagent_timeout = "90s"
`)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.RequestTimeout != "90s" {
		t.Errorf("RequestTimeout = %q, want \"90s\"", cfg.LLM.RequestTimeout)
	}
	if cfg.LLM.RequestTimeoutDuration != 90*time.Second {
		t.Errorf("RequestTimeoutDuration = %v, want 90s", cfg.LLM.RequestTimeoutDuration)
	}
	if cfg.LLM.IdleTimeoutDuration != 10*time.Minute {
		t.Errorf("IdleTimeoutDuration = %v, want 10m", cfg.LLM.IdleTimeoutDuration)
	}
	if cfg.LLM.FirstByteTimeoutDuration != 0 {
		t.Errorf("FirstByteTimeoutDuration = %v, want 0s", cfg.LLM.FirstByteTimeoutDuration)
	}
	if cfg.LLM.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want 5", cfg.LLM.MaxRetries)
	}
	if cfg.Agent.SubagentTimeout != "90s" {
		t.Errorf("SubagentTimeout = %q, want \"90s\"", cfg.Agent.SubagentTimeout)
	}
	if cfg.Agent.SubagentTimeoutDuration != 90*time.Second {
		t.Errorf("SubagentTimeoutDuration = %v, want 90s", cfg.Agent.SubagentTimeoutDuration)
	}
}

func TestLoadInvalidDuration(t *testing.T) {
	writeTestConfig(t, `
[llm]
idle_timeout = "5min"
`)
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid idle_timeout, got nil")
	}
	if !strings.Contains(err.Error(), "llm.idle_timeout") {
		t.Errorf("error %q missing key llm.idle_timeout", err)
	}
	if !strings.Contains(err.Error(), "5min") {
		t.Errorf("error %q missing value 5min", err)
	}
}

func TestLoadEnvOverridesFile(t *testing.T) {
	writeTestConfig(t, `
[llm]
request_timeout = "10m"
idle_timeout = "10m"
first_byte_timeout = "90s"
max_retries = 5

[agent]
subagent_timeout = "10m"
`)
	t.Setenv("KTERMINAL_LLM_REQUEST_TIMEOUT", "90s")
	t.Setenv("KTERMINAL_LLM_IDLE_TIMEOUT", "90s")
	t.Setenv("KTERMINAL_LLM_FIRST_BYTE_TIMEOUT", "45s")
	t.Setenv("KTERMINAL_LLM_MAX_RETRIES", "7")
	t.Setenv("KTERMINAL_AGENT_SUBAGENT_TIMEOUT", "2m")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.RequestTimeoutDuration != 90*time.Second {
		t.Errorf("RequestTimeoutDuration = %v, want 90s", cfg.LLM.RequestTimeoutDuration)
	}
	if cfg.LLM.IdleTimeoutDuration != 90*time.Second {
		t.Errorf("IdleTimeoutDuration = %v, want 90s", cfg.LLM.IdleTimeoutDuration)
	}
	if cfg.LLM.FirstByteTimeoutDuration != 45*time.Second {
		t.Errorf("FirstByteTimeoutDuration = %v, want 45s", cfg.LLM.FirstByteTimeoutDuration)
	}
	if cfg.LLM.MaxRetries != 7 {
		t.Errorf("MaxRetries = %d, want 7", cfg.LLM.MaxRetries)
	}
	if cfg.Agent.SubagentTimeoutDuration != 2*time.Minute {
		t.Errorf("SubagentTimeoutDuration = %v, want 2m", cfg.Agent.SubagentTimeoutDuration)
	}
}

func TestLoadNegativeMaxRetriesRejected(t *testing.T) {
	writeTestConfig(t, `
[llm]
max_retries = -1
`)
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for negative max_retries, got nil")
	}
	if !strings.Contains(err.Error(), "llm.max_retries") {
		t.Errorf("error %q missing key llm.max_retries", err)
	}
}

func TestLoadEnvNegativeMaxRetriesRejected(t *testing.T) {
	writeTestConfig(t, "")
	t.Setenv("KTERMINAL_LLM_MAX_RETRIES", "-2")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for negative KTERMINAL_LLM_MAX_RETRIES, got nil")
	}
	if !strings.Contains(err.Error(), "llm.max_retries") {
		t.Errorf("error %q missing key llm.max_retries", err)
	}
}

func TestLoadEnvInvalidDurationRejected(t *testing.T) {
	writeTestConfig(t, "")
	t.Setenv("KTERMINAL_LLM_IDLE_TIMEOUT", "5min")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid KTERMINAL_LLM_IDLE_TIMEOUT, got nil")
	}
	if !strings.Contains(err.Error(), "llm.idle_timeout") {
		t.Errorf("error %q missing key llm.idle_timeout", err)
	}
}

func TestLoadEnvInvalidMaxRetriesRejected(t *testing.T) {
	writeTestConfig(t, "")
	t.Setenv("KTERMINAL_LLM_MAX_RETRIES", "abc")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for non-numeric KTERMINAL_LLM_MAX_RETRIES, got nil")
	}
	if !strings.Contains(err.Error(), "llm.max_retries") {
		t.Errorf("error %q missing key llm.max_retries", err)
	}
}

func TestSaveOmitsResolvedDurationFields(t *testing.T) {
	writeTestConfig(t, "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, field := range []string{"RequestTimeoutDuration", "IdleTimeoutDuration", "FirstByteTimeoutDuration", "SubagentTimeoutDuration"} {
		if strings.Contains(content, field) {
			t.Errorf("saved config contains resolved field %s", field)
		}
	}
}
