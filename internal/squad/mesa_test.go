package squad

import (
	"encoding/json"
	"reflect"
	"sync"
	"testing"
)

func mesaTestKickoff() Kickoff {
	return Kickoff{
		Roles:           []string{"architect", "backend", "qa"},
		MaxConvocations: 6,
		TokenBudget:     150000,
	}
}

func mesaTestDisciplines() map[string]string {
	return map[string]string{
		"architect": "architecture",
		"backend":   "backend",
		"qa":        "quality",
	}
}

func TestResetBuildsMesaFromKickoff(t *testing.T) {
	var m Mesa
	disciplines := mesaTestDisciplines()
	m.Reset(mesaTestKickoff(), disciplines)
	wantRoles := []string{"architect", "backend", "qa"}
	if len(m.Roles) != len(wantRoles) {
		t.Fatalf("expected %d roles, got %d", len(wantRoles), len(m.Roles))
	}
	for i, want := range wantRoles {
		if m.Roles[i] != want {
			t.Errorf("Roles[%d] = %q, want %q", i, m.Roles[i], want)
		}
	}
	if m.MaxConvocations != 6 {
		t.Errorf("MaxConvocations = %d, want 6", m.MaxConvocations)
	}
	if m.TokenBudget != 150000 {
		t.Errorf("TokenBudget = %d, want 150000", m.TokenBudget)
	}
	if m.Convocations != 0 {
		t.Errorf("Convocations = %d, want 0", m.Convocations)
	}
	if m.Tokens != 0 {
		t.Errorf("Tokens = %d, want 0", m.Tokens)
	}
	if len(m.Entries) != len(wantRoles) {
		t.Fatalf("expected %d entries, got %d", len(wantRoles), len(m.Entries))
	}
	for i, want := range wantRoles {
		e := m.Entries[i]
		if e.Name != want {
			t.Errorf("Entries[%d].Name = %q, want %q", i, e.Name, want)
		}
		if e.Discipline != disciplines[want] {
			t.Errorf("Entries[%d].Discipline = %q, want %q", i, e.Discipline, disciplines[want])
		}
		if e.Status != StatusWaiting {
			t.Errorf("Entries[%d].Status = %q, want %q", i, e.Status, StatusWaiting)
		}
		if e.Model != "" {
			t.Errorf("Entries[%d].Model = %q, want empty", i, e.Model)
		}
		if e.Tokens != 0 {
			t.Errorf("Entries[%d].Tokens = %d, want 0", i, e.Tokens)
		}
		if e.Cost != 0 {
			t.Errorf("Entries[%d].Cost = %v, want 0", i, e.Cost)
		}
	}
}

func TestResetRebuildsDirtyMesa(t *testing.T) {
	var m Mesa
	m.Reset(mesaTestKickoff(), mesaTestDisciplines())
	m.AddTokens(500)
	m.AddConvocation()
	m.StartDeliberation("architect")
	m.ObservePersona("architect", "model-a", 100, 0.5)
	m.Reset(mesaTestKickoff(), mesaTestDisciplines())
	if m.Tokens != 0 {
		t.Errorf("Tokens = %d, want 0 after Reset", m.Tokens)
	}
	if m.Convocations != 0 {
		t.Errorf("Convocations = %d, want 0 after Reset", m.Convocations)
	}
	if len(m.Entries) != 3 {
		t.Fatalf("expected 3 entries after Reset, got %d", len(m.Entries))
	}
	for i, e := range m.Entries {
		if e.Status != StatusWaiting {
			t.Errorf("Entries[%d].Status = %q, want %q", i, e.Status, StatusWaiting)
		}
		if e.Tokens != 0 || e.Cost != 0 || e.Model != "" {
			t.Errorf("Entries[%d] = %+v, want zeroed metrics", i, e)
		}
	}
}

func TestResetCopiesRolesSlice(t *testing.T) {
	roles := []string{"architect", "backend"}
	var m Mesa
	m.Reset(Kickoff{Roles: roles, MaxConvocations: 4, TokenBudget: 100000}, nil)
	roles[0] = "mutated"
	if m.Roles[0] != "architect" {
		t.Errorf("Roles[0] = %q, want architect (Reset must copy the slice)", m.Roles[0])
	}
}

func TestResetTurnZeroesCountersKeepsEntries(t *testing.T) {
	var m Mesa
	m.Reset(mesaTestKickoff(), mesaTestDisciplines())
	m.AddTokens(4200)
	m.AddConvocation()
	m.AddConvocation()
	m.StartDeliberation("backend")
	m.ObservePersona("backend", "model-b", 300, 0.75)
	before := append([]MesaEntry(nil), m.Entries...)
	m.ResetTurn()
	if m.Convocations != 0 {
		t.Errorf("Convocations = %d, want 0 after ResetTurn", m.Convocations)
	}
	if m.Tokens != 0 {
		t.Errorf("Tokens = %d, want 0 after ResetTurn", m.Tokens)
	}
	if !reflect.DeepEqual(m.Entries, before) {
		t.Errorf("Entries changed after ResetTurn: %+v, want %+v", m.Entries, before)
	}
}

func TestDeliberationTransitions(t *testing.T) {
	var m Mesa
	m.Reset(mesaTestKickoff(), mesaTestDisciplines())
	m.StartDeliberation("architect")
	if m.Entries[0].Status != StatusDeliberating {
		t.Errorf("Entries[0].Status = %q, want %q", m.Entries[0].Status, StatusDeliberating)
	}
	m.FinishDeliberation("architect")
	if m.Entries[0].Status != StatusDone {
		t.Errorf("Entries[0].Status = %q, want %q", m.Entries[0].Status, StatusDone)
	}
	for i, e := range m.Entries[1:] {
		if e.Status != StatusWaiting {
			t.Errorf("Entries[%d].Status = %q, want %q", i+1, e.Status, StatusWaiting)
		}
	}
}

func TestDeliberationUnknownPersonaIsNoOp(t *testing.T) {
	var m Mesa
	m.StartDeliberation("architect")
	m.FinishDeliberation("architect")
	m.ObservePersona("architect", "model-x", 10, 0.5)
	m.Reset(mesaTestKickoff(), mesaTestDisciplines())
	m.StartDeliberation("ghost")
	m.FinishDeliberation("ghost")
	m.ObservePersona("ghost", "model-x", 10, 0.5)
	for i, e := range m.Entries {
		if e.Status != StatusWaiting {
			t.Errorf("Entries[%d].Status = %q, want %q", i, e.Status, StatusWaiting)
		}
		if e.Model != "" || e.Tokens != 0 || e.Cost != 0 {
			t.Errorf("Entries[%d] = %+v, want untouched metrics", i, e)
		}
	}
}

func TestObservePersonaAccumulatesAcrossConvocations(t *testing.T) {
	var m Mesa
	m.Reset(mesaTestKickoff(), mesaTestDisciplines())
	m.StartDeliberation("architect")
	m.ObservePersona("architect", "model-a", 100, 0.5)
	m.ObservePersona("architect", "model-a", 150, 0.75)
	e := m.Entries[0]
	if e.Tokens != 150 {
		t.Errorf("Tokens = %d, want 150 (cumulative over baseline 0)", e.Tokens)
	}
	if e.Cost != 0.75 {
		t.Errorf("Cost = %v, want 0.75 (cumulative over baseline 0)", e.Cost)
	}
	if e.Model != "model-a" {
		t.Errorf("Model = %q, want model-a", e.Model)
	}
	m.FinishDeliberation("architect")
	m.StartDeliberation("architect")
	m.ObservePersona("architect", "model-b", 200, 1.0)
	e = m.Entries[0]
	if e.Tokens != 350 {
		t.Errorf("Tokens = %d, want 350 (150 baseline + 200)", e.Tokens)
	}
	if e.Cost != 1.75 {
		t.Errorf("Cost = %v, want 1.75 (0.75 baseline + 1.0)", e.Cost)
	}
	if e.Model != "model-b" {
		t.Errorf("Model = %q, want model-b", e.Model)
	}
}

func TestFinishTurnMarksDeliberatingDone(t *testing.T) {
	var m Mesa
	m.Reset(mesaTestKickoff(), mesaTestDisciplines())
	m.StartDeliberation("backend")
	m.FinishDeliberation("qa")
	m.FinishTurn()
	want := []string{StatusWaiting, StatusDone, StatusDone}
	for i, e := range m.Entries {
		if e.Status != want[i] {
			t.Errorf("Entries[%d].Status = %q, want %q", i, e.Status, want[i])
		}
	}
}

func TestMesaFullCycle(t *testing.T) {
	var m Mesa
	m.Reset(
		Kickoff{Roles: []string{"architect", "backend"}, MaxConvocations: 4, TokenBudget: 100000},
		map[string]string{"architect": "architecture", "backend": "backend"},
	)
	m.ResetTurn()
	m.AddConvocation()
	m.AddTokens(120)
	m.StartDeliberation("architect")
	m.ObservePersona("architect", "model-a", 80, 0.25)
	m.ObservePersona("architect", "model-a", 95, 0.5)
	m.FinishDeliberation("architect")
	m.StartDeliberation("backend")
	m.ObservePersona("backend", "model-b", 60, 0.125)
	m.FinishTurn()
	if m.Convocations != 1 {
		t.Errorf("Convocations = %d, want 1", m.Convocations)
	}
	if m.Tokens != 120 {
		t.Errorf("Tokens = %d, want 120", m.Tokens)
	}
	architect := m.Entries[0]
	if architect.Status != StatusDone {
		t.Errorf("architect status = %q, want %q", architect.Status, StatusDone)
	}
	if architect.Model != "model-a" {
		t.Errorf("architect model = %q, want model-a", architect.Model)
	}
	if architect.Tokens != 95 {
		t.Errorf("architect tokens = %d, want 95", architect.Tokens)
	}
	if architect.Cost != 0.5 {
		t.Errorf("architect cost = %v, want 0.5", architect.Cost)
	}
	backend := m.Entries[1]
	if backend.Status != StatusDone {
		t.Errorf("backend status = %q, want %q", backend.Status, StatusDone)
	}
	if backend.Model != "model-b" {
		t.Errorf("backend model = %q, want model-b", backend.Model)
	}
	if backend.Tokens != 60 {
		t.Errorf("backend tokens = %d, want 60", backend.Tokens)
	}
	if backend.Cost != 0.125 {
		t.Errorf("backend cost = %v, want 0.125", backend.Cost)
	}
}

func TestMesaJSONRoundTripPopulated(t *testing.T) {
	var m Mesa
	m.Reset(
		Kickoff{Roles: []string{"architect", "backend"}, MaxConvocations: 5, TokenBudget: 200000},
		map[string]string{"architect": "architecture"},
	)
	m.AddConvocation()
	m.AddTokens(1234)
	m.StartDeliberation("architect")
	m.ObservePersona("architect", "model-a", 900, 0.5)
	m.FinishDeliberation("architect")
	data, err := json.Marshal(&m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var restored Mesa
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	restoredData, err := json.Marshal(&restored)
	if err != nil {
		t.Fatalf("Marshal restored: %v", err)
	}
	if string(data) != string(restoredData) {
		t.Errorf("round-trip JSON mismatch: %s vs %s", data, restoredData)
	}
	if !reflect.DeepEqual(m.Roles, restored.Roles) {
		t.Errorf("Roles = %v, want %v", restored.Roles, m.Roles)
	}
	if !reflect.DeepEqual(m.Entries, restored.Entries) {
		t.Errorf("Entries = %+v, want %+v", restored.Entries, m.Entries)
	}
	if restored.MaxConvocations != m.MaxConvocations {
		t.Errorf("MaxConvocations = %d, want %d", restored.MaxConvocations, m.MaxConvocations)
	}
	if restored.TokenBudget != m.TokenBudget {
		t.Errorf("TokenBudget = %d, want %d", restored.TokenBudget, m.TokenBudget)
	}
	if restored.Convocations != m.Convocations {
		t.Errorf("Convocations = %d, want %d", restored.Convocations, m.Convocations)
	}
	if restored.Tokens != m.Tokens {
		t.Errorf("Tokens = %d, want %d", restored.Tokens, m.Tokens)
	}
}

func TestMesaJSONOmitEmpty(t *testing.T) {
	var zero Mesa
	data, err := json.Marshal(&zero)
	if err != nil {
		t.Fatalf("Marshal zero: %v", err)
	}
	if string(data) != "{}" {
		t.Errorf("zero Mesa JSON = %s, want {}", data)
	}
	var m Mesa
	m.Reset(Kickoff{Roles: []string{"qa"}, MaxConvocations: 3, TokenBudget: 50000}, nil)
	data, err = json.Marshal(&m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatalf("Unmarshal top: %v", err)
	}
	for _, key := range []string{"roles", "max_convocations", "token_budget", "entries"} {
		if _, ok := top[key]; !ok {
			t.Errorf("expected key %q in %s", key, data)
		}
	}
	for _, key := range []string{"convocations", "tokens"} {
		if _, ok := top[key]; ok {
			t.Errorf("unexpected key %q in %s", key, data)
		}
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(top["entries"], &entries); err != nil {
		t.Fatalf("Unmarshal entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	entry := entries[0]
	for _, key := range []string{"name", "status"} {
		if _, ok := entry[key]; !ok {
			t.Errorf("expected entry key %q in %s", key, data)
		}
	}
	for _, key := range []string{"discipline", "model", "tokens", "cost"} {
		if _, ok := entry[key]; ok {
			t.Errorf("unexpected entry key %q in %s", key, data)
		}
	}
}

func TestMesaCloneIsolatesState(t *testing.T) {
	var nilMesa *Mesa
	if nilMesa.Clone() != nil {
		t.Fatal("Clone of nil Mesa must return nil")
	}

	var m Mesa
	m.Reset(mesaTestKickoff(), mesaTestDisciplines())
	m.AddConvocation()
	m.AddTokens(4200)
	m.StartDeliberation("architect")
	m.ObservePersona("architect", "model-a", 900, 0.5)

	clone := m.Clone()
	if clone == &m {
		t.Fatal("Clone returned the receiver pointer")
	}
	if !reflect.DeepEqual(clone.Roles, m.Roles) {
		t.Errorf("clone Roles = %v, want %v", clone.Roles, m.Roles)
	}
	if clone.MaxConvocations != m.MaxConvocations || clone.TokenBudget != m.TokenBudget {
		t.Errorf("clone ceilings = %d/%d, want %d/%d", clone.MaxConvocations, clone.TokenBudget, m.MaxConvocations, m.TokenBudget)
	}
	if clone.Convocations != m.Convocations || clone.Tokens != m.Tokens {
		t.Errorf("clone counters = %d/%d, want %d/%d", clone.Convocations, clone.Tokens, m.Convocations, m.Tokens)
	}
	if !reflect.DeepEqual(clone.Entries, m.Entries) {
		t.Errorf("clone Entries = %+v, want %+v", clone.Entries, m.Entries)
	}

	m.Entries[0].Status = StatusDone
	m.AddTokens(1)
	if clone.Entries[0].Status != StatusDeliberating {
		t.Errorf("clone entry status = %q, want deliberating (entries must not alias)", clone.Entries[0].Status)
	}
	if clone.Tokens != 4200 {
		t.Errorf("clone Tokens = %d, want 4200 (counters must not alias)", clone.Tokens)
	}

	var empty Mesa
	emptyClone := empty.Clone()
	if emptyClone == nil {
		t.Fatal("Clone of zero Mesa must return a non-nil copy")
	}
	if len(emptyClone.Entries) != 0 || emptyClone.Tokens != 0 {
		t.Errorf("clone of zero Mesa = %+v, want zeroed copy", emptyClone)
	}
}

func TestMesaConcurrentAccess(t *testing.T) {
	var m Mesa
	m.Reset(mesaTestKickoff(), mesaTestDisciplines())
	const goroutines = 100
	const iterations = 50
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				m.AddTokens(2)
				m.AddConvocation()
				m.StartDeliberation("architect")
				m.ObservePersona("architect", "model-a", 10, 0.25)
				m.FinishDeliberation("architect")
				m.StartDeliberation("backend")
				m.ObservePersona("backend", "model-b", 20, 0.5)
				m.FinishTurn()
				m.StartDeliberation("ghost")
				m.FinishDeliberation("ghost")
				m.ObservePersona("ghost", "model-x", 1, 0.125)
			}
		}()
	}
	wg.Wait()
	if m.Tokens != int64(goroutines*iterations*2) {
		t.Errorf("Tokens = %d, want %d", m.Tokens, goroutines*iterations*2)
	}
	if m.Convocations != goroutines*iterations {
		t.Errorf("Convocations = %d, want %d", m.Convocations, goroutines*iterations)
	}
}
