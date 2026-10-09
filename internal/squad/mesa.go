package squad

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
)

const (
	StatusWaiting      = "waiting"
	StatusDeliberating = "deliberating"
	StatusDone         = "done"
)

type MesaEntry struct {
	Name       string  `json:"name"`
	Discipline string  `json:"discipline,omitempty"`
	Status     string  `json:"status"`
	Model      string  `json:"model,omitempty"`
	Tokens     int64   `json:"tokens,omitempty"`
	Cost       float64 `json:"cost,omitempty"`
}

type mesaBaseline struct {
	tokens int64
	cost   float64
}

type Mesa struct {
	mu        sync.Mutex
	baselines map[string]mesaBaseline

	Roles           []string    `json:"roles,omitempty"`
	MaxConvocations int         `json:"max_convocations,omitempty"`
	TokenBudget     int64       `json:"token_budget,omitempty"`
	Convocations    int         `json:"convocations,omitempty"`
	Tokens          int64       `json:"tokens,omitempty"`
	Entries         []MesaEntry `json:"entries,omitempty"`
}

func (m *Mesa) Reset(k Kickoff, disciplines map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Roles = append([]string(nil), k.Roles...)
	m.MaxConvocations = k.MaxConvocations
	m.TokenBudget = k.TokenBudget
	m.Convocations = 0
	m.Tokens = 0
	m.Entries = make([]MesaEntry, len(k.Roles))
	for i, role := range k.Roles {
		m.Entries[i] = MesaEntry{Name: role, Discipline: disciplines[role], Status: StatusWaiting}
	}
	m.baselines = nil
}

func (m *Mesa) ResetTurn() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Convocations = 0
	m.Tokens = 0
}

func (m *Mesa) AddTokens(n int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Tokens += n
}

func (m *Mesa) StatusLine() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var b strings.Builder
	b.WriteString("[mesa: ")
	b.WriteString(strconv.Itoa(m.Convocations))
	if m.MaxConvocations > 0 {
		b.WriteString("/")
		b.WriteString(strconv.Itoa(m.MaxConvocations))
	}
	b.WriteString(" convocations · ")
	b.WriteString(formatTokensK(m.Tokens))
	if m.TokenBudget > 0 {
		b.WriteString("/")
		b.WriteString(formatTokensK(m.TokenBudget))
	}
	b.WriteString(" tokens")
	if m.TokenBudget > 0 && m.Tokens > m.TokenBudget {
		b.WriteString(" · over budget")
	}
	b.WriteString("]")
	return b.String()
}

func formatTokensK(n int64) string {
	if n < 1000 {
		return strconv.FormatInt(n, 10)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}

func (m *Mesa) AddConvocation() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Convocations++
}

func (m *Mesa) StartDeliberation(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := m.entryIndexLocked(name)
	if i < 0 {
		return
	}
	m.Entries[i].Status = StatusDeliberating
	if m.baselines == nil {
		m.baselines = make(map[string]mesaBaseline)
	}
	m.baselines[name] = mesaBaseline{tokens: m.Entries[i].Tokens, cost: m.Entries[i].Cost}
}

func (m *Mesa) FinishDeliberation(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := m.entryIndexLocked(name)
	if i < 0 {
		return
	}
	m.Entries[i].Status = StatusDone
}

func (m *Mesa) FinishTurn() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.Entries {
		if m.Entries[i].Status == StatusDeliberating {
			m.Entries[i].Status = StatusDone
		}
	}
}

func (m *Mesa) ObservePersona(name, model string, tokens int64, cost float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := m.entryIndexLocked(name)
	if i < 0 {
		return
	}
	base := m.baselines[name]
	m.Entries[i].Model = model
	m.Entries[i].Tokens = base.tokens + tokens
	m.Entries[i].Cost = base.cost + cost
}

func (m *Mesa) Clone() *Mesa {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return &Mesa{
		Roles:           append([]string(nil), m.Roles...),
		MaxConvocations: m.MaxConvocations,
		TokenBudget:     m.TokenBudget,
		Convocations:    m.Convocations,
		Tokens:          m.Tokens,
		Entries:         append([]MesaEntry(nil), m.Entries...),
	}
}

func (m *Mesa) entryIndexLocked(name string) int {
	for i := range m.Entries {
		if m.Entries[i].Name == name {
			return i
		}
	}
	return -1
}
