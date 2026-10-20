package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type modelsCache struct {
	TS     time.Time `json:"ts"`
	Models []string  `json:"models"`
}

// LoadModelsCache returns the cached gateway model list and its timestamp.
// A missing, corrupt or empty cache is not a cache.
func LoadModelsCache(path string) ([]string, time.Time, bool) {
	if path == "" {
		return nil, time.Time{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, false
	}
	var c modelsCache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, time.Time{}, false
	}
	if len(c.Models) == 0 || c.TS.IsZero() {
		return nil, time.Time{}, false
	}
	return c.Models, c.TS, true
}

// SaveModelsCache persists the gateway model list with the current timestamp.
func SaveModelsCache(path string, models []string) error {
	if path == "" || len(models) == 0 {
		return nil
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	data, err := json.Marshal(modelsCache{TS: time.Now().UTC(), Models: models})
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
