package llm

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestModelsCacheRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "models-cache.json")
	if _, _, ok := LoadModelsCache(path); ok {
		t.Fatal("LoadModelsCache ok = true for missing file")
	}
	models := []string{"glm-5.3", "deepseek-v4-flash"}
	if err := SaveModelsCache(path, models); err != nil {
		t.Fatalf("SaveModelsCache: %v", err)
	}
	loaded, ts, ok := LoadModelsCache(path)
	if !ok {
		t.Fatal("LoadModelsCache ok = false after save")
	}
	if len(loaded) != 2 || loaded[0] != "glm-5.3" || loaded[1] != "deepseek-v4-flash" {
		t.Fatalf("loaded = %v, want %v", loaded, models)
	}
	if time.Since(ts) > time.Minute {
		t.Fatalf("ts = %v, want recent", ts)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %v, want 0600", info.Mode().Perm())
	}
}

func TestModelsCacheRejectsCorruptAndEmpty(t *testing.T) {
	dir := t.TempDir()

	corrupt := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(corrupt, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}
	if _, _, ok := LoadModelsCache(corrupt); ok {
		t.Fatal("LoadModelsCache ok = true for corrupt file")
	}

	empty := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(empty, []byte(`{"ts":"2026-01-01T00:00:00Z","models":[]}`), 0o600); err != nil {
		t.Fatalf("write empty: %v", err)
	}
	if _, _, ok := LoadModelsCache(empty); ok {
		t.Fatal("LoadModelsCache ok = true for empty model list")
	}

	noTS := filepath.Join(dir, "nots.json")
	if err := os.WriteFile(noTS, []byte(`{"models":["a"]}`), 0o600); err != nil {
		t.Fatalf("write nots: %v", err)
	}
	if _, _, ok := LoadModelsCache(noTS); ok {
		t.Fatal("LoadModelsCache ok = true without timestamp")
	}

	if _, _, ok := LoadModelsCache(""); ok {
		t.Fatal("LoadModelsCache ok = true for empty path")
	}
}

func TestSaveModelsCacheNoop(t *testing.T) {
	if err := SaveModelsCache("", []string{"a"}); err != nil {
		t.Fatalf("SaveModelsCache(\"\") = %v, want nil", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "models-cache.json")
	if err := SaveModelsCache(path, nil); err != nil {
		t.Fatalf("SaveModelsCache(nil models) = %v, want nil", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("cache file written with no models: %v", err)
	}
}
