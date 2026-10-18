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
	if cfg.LLM.FirstByteTimeoutDuration != 120*time.Second {
		t.Errorf("FirstByteTimeoutDuration = %v, want 120s", cfg.LLM.FirstByteTimeoutDuration)
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

func TestJevEnabledAccessorTriState(t *testing.T) {
	var ts Typesafe
	if !ts.JevEnabled() {
		t.Error("JevEnabled() = false, want true for nil Enabled")
	}
	explicitTrue := true
	ts.Enabled = &explicitTrue
	if !ts.JevEnabled() {
		t.Error("JevEnabled() = false, want true for explicit true")
	}
	explicitFalse := false
	ts.Enabled = &explicitFalse
	if ts.JevEnabled() {
		t.Error("JevEnabled() = true, want false for explicit false")
	}
}

func TestJevEnabledNilDefaultsTrue(t *testing.T) {
	writeTestConfig(t, "")
	t.Setenv("KTERMINAL_TYPESAFE_ENABLED", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Typesafe.Enabled != nil {
		t.Errorf("Enabled = %v, want nil", *cfg.Typesafe.Enabled)
	}
	if !cfg.Typesafe.JevEnabled() {
		t.Error("JevEnabled() = false, want true for config without enabled")
	}
}

func TestJevEnabledExplicitTOML(t *testing.T) {
	writeTestConfig(t, `
[typesafe]
api_key = "ts-key"
enabled = true
`)
	t.Setenv("KTERMINAL_TYPESAFE_ENABLED", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Typesafe.Enabled == nil {
		t.Fatal("Enabled = nil, want explicit true")
	}
	if !*cfg.Typesafe.Enabled {
		t.Error("*Enabled = false, want true")
	}
	if !cfg.Typesafe.JevEnabled() {
		t.Error("JevEnabled() = false, want true for enabled = true")
	}

	writeTestConfig(t, `
[typesafe]
api_key = "ts-key"
enabled = false
`)
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Typesafe.Enabled == nil {
		t.Fatal("Enabled = nil, want explicit false")
	}
	if *cfg.Typesafe.Enabled {
		t.Error("*Enabled = true, want false")
	}
	if cfg.Typesafe.JevEnabled() {
		t.Error("JevEnabled() = true, want false for enabled = false")
	}
}

func TestLoadTypesafeEnabledEnvOverridesFile(t *testing.T) {
	cases := []struct {
		name        string
		fileValue   string
		envValue    string
		wantEnabled bool
	}{
		{"env false over file true", "true", "false", false},
		{"env true over file false", "false", "true", true},
		{"env 1 over file false", "false", "1", true},
		{"env 0 over file true", "true", "0", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writeTestConfig(t, "[typesafe]\nenabled = "+tc.fileValue+"\n")
			t.Setenv("KTERMINAL_TYPESAFE_ENABLED", tc.envValue)
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Typesafe.Enabled == nil {
				t.Fatal("Enabled = nil, want explicit value from env")
			}
			if *cfg.Typesafe.Enabled != tc.wantEnabled {
				t.Errorf("*Enabled = %v, want %v", *cfg.Typesafe.Enabled, tc.wantEnabled)
			}
			if cfg.Typesafe.JevEnabled() != tc.wantEnabled {
				t.Errorf("JevEnabled() = %v, want %v", cfg.Typesafe.JevEnabled(), tc.wantEnabled)
			}
		})
	}
}

func TestLoadTypesafeEnabledEnvInvalidRejected(t *testing.T) {
	writeTestConfig(t, "")
	t.Setenv("KTERMINAL_TYPESAFE_ENABLED", "maybe")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid KTERMINAL_TYPESAFE_ENABLED, got nil")
	}
	if !strings.Contains(err.Error(), "typesafe.enabled") {
		t.Errorf("error %q missing key typesafe.enabled", err)
	}
	if !strings.Contains(err.Error(), "maybe") {
		t.Errorf("error %q missing value maybe", err)
	}
}

func TestLoadModelEnvOverridesFile(t *testing.T) {
	writeTestConfig(t, "model = \"from-file\"\n")
	t.Setenv("KTERMINAL_MODEL", "from-env")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "from-env" {
		t.Errorf("Model = %q, want \"from-env\"", cfg.Model)
	}

	writeTestConfig(t, "model = \"from-file\"\n")
	t.Setenv("KTERMINAL_MODEL", "")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "from-file" {
		t.Errorf("Model = %q, want \"from-file\" with empty env", cfg.Model)
	}
}

func TestSaveNormalizesNilEnabledToExplicitTrue(t *testing.T) {
	writeTestConfig(t, `
[typesafe]
api_key = "ts-key"
`)
	t.Setenv("KTERMINAL_TYPESAFE_ENABLED", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Typesafe.Enabled != nil {
		t.Fatal("Enabled != nil before Save, want nil")
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "enabled = true") {
		t.Errorf("saved config missing materialized enabled = true:\n%s", content)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Typesafe.Enabled == nil {
		t.Fatal("Enabled = nil after round-trip, want explicit true")
	}
	if !*loaded.Typesafe.Enabled {
		t.Error("*Enabled = false after round-trip, want true")
	}
	if !loaded.Typesafe.JevEnabled() {
		t.Error("JevEnabled() = false after round-trip, want true")
	}
}

func TestSaveRoundTripPreservesExplicitFalse(t *testing.T) {
	writeTestConfig(t, `
[typesafe]
api_key = "ts-key"
enabled = false
`)
	t.Setenv("KTERMINAL_TYPESAFE_ENABLED", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Typesafe.JevEnabled() {
		t.Fatal("JevEnabled() = true before Save, want false")
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "enabled = false") {
		t.Errorf("saved config missing enabled = false:\n%s", string(data))
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Typesafe.JevEnabled() {
		t.Error("JevEnabled() = true after round-trip, want false")
	}
}

func TestSaveRoundTripModel(t *testing.T) {
	writeTestConfig(t, "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Model = "glm-5.3"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "model = \"glm-5.3\"") {
		t.Errorf("saved config missing model entry:\n%s", string(data))
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Model != "glm-5.3" {
		t.Errorf("Model = %q after round-trip, want \"glm-5.3\"", loaded.Model)
	}
}

func TestLoadLegacyConfigDefaults(t *testing.T) {
	writeTestConfig(t, `
[llm]
base_url = "http://localhost"
api_key = "key"

[typesafe]
api_key = "ts-key"

[squad]
default_mode = "sdd"
max_convocations = 8
token_budget = 200000
`)
	t.Setenv("KTERMINAL_TYPESAFE_ENABLED", "")
	t.Setenv("KTERMINAL_MODEL", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "" {
		t.Errorf("Model = %q, want \"\" for legacy config", cfg.Model)
	}
	if cfg.Typesafe.Enabled != nil {
		t.Errorf("Enabled = %v, want nil for legacy config", *cfg.Typesafe.Enabled)
	}
	if !cfg.Typesafe.JevEnabled() {
		t.Error("JevEnabled() = false, want true for legacy config")
	}
	if cfg.Typesafe.APIKey != "ts-key" {
		t.Errorf("Typesafe.APIKey = %q, want \"ts-key\"", cfg.Typesafe.APIKey)
	}
}

func TestLoadRouterDefaultsToJev(t *testing.T) {
	writeTestConfig(t, "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Router.Engine != "jev" {
		t.Fatalf("engine = %q, want jev default", cfg.Router.Engine)
	}
	if !cfg.Router.RoutingEnabled() {
		t.Fatal("routing must default to enabled")
	}
	if cfg.Router.Shadow {
		t.Fatal("shadow must default to false")
	}
}

func TestLoadRouterExplicitEngine(t *testing.T) {
	writeTestConfig(t, "[router]\nengine = \"laya\"\nshadow = true\n\n[typesafe]\nenabled = false\n")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Router.Engine != "laya" {
		t.Fatalf("engine = %q, want laya", cfg.Router.Engine)
	}
	if !cfg.Router.Shadow {
		t.Fatal("shadow = true must parse")
	}
	if !cfg.Router.RoutingEnabled() {
		t.Fatal("routing enabled defaults to true with nil")
	}
	if cfg.Typesafe.JevEnabled() {
		t.Fatal("typesafe.enabled stays an independent jev gate")
	}
}

func TestLoadRouterInvalidEngine(t *testing.T) {
	writeTestConfig(t, "[router]\nengine = \"grok\"\n")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "invalid router.engine") {
		t.Fatalf("err = %v, want invalid router.engine", err)
	}
}

func TestLoadRouterEnvOverride(t *testing.T) {
	writeTestConfig(t, "[router]\nengine = \"jev\"\n")
	t.Setenv("KTERMINAL_ROUTER", "laya")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Router.Engine != "laya" {
		t.Fatalf("engine = %q, want laya from KTERMINAL_ROUTER", cfg.Router.Engine)
	}
}

func TestSaveMaterializesRouterSection(t *testing.T) {
	path := writeTestConfig(t, "[llm]\nbase_url = \"http://localhost:4000\"\n")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(raw), "[router]") || !strings.Contains(string(raw), "engine = \"jev\"") || !strings.Contains(string(raw), "enabled = true") {
		t.Fatalf("saved config must materialize the router section:\n%s", string(raw))
	}
	reloaded, err := Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Router.Engine != "jev" || !reloaded.Router.RoutingEnabled() {
		t.Fatalf("reloaded router = %+v, want jev enabled", reloaded.Router)
	}
}
