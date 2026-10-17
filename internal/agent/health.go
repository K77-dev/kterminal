package agent

import (
	"sync"

	"kterminal/internal/catalog"
)

type modelHealth struct {
	mu      sync.Mutex
	blocked map[string]bool
}

func newModelHealth() *modelHealth {
	return &modelHealth{blocked: map[string]bool{}}
}

func (h *modelHealth) block(model string) {
	if h == nil || model == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.blocked[model] = true
}

func (h *modelHealth) isBlocked(model string) bool {
	if h == nil {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.blocked[model]
}

func (h *modelHealth) filter(models []catalog.Model) []catalog.Model {
	if h == nil {
		return models
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.blocked) == 0 {
		return models
	}
	out := make([]catalog.Model, 0, len(models))
	for _, m := range models {
		if !h.blocked[m.Name] {
			out = append(out, m)
		}
	}
	return out
}
