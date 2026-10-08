package telemetry

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func almostEq(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func persistedStats(t *testing.T, path, model string) *Stats {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read telemetry file: %v", err)
	}
	var d diskStore
	if err := json.Unmarshal(data, &d); err != nil {
		t.Fatalf("unmarshal telemetry file: %v", err)
	}
	return d.Models[model]
}

func TestWelfordMeanKnownSequence(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "telemetry.json"))
	for _, tps := range []float64{10, 20, 30} {
		s.Record("sonnet", tps)
	}
	mean, samples := s.GetMean("sonnet")
	if samples != 3 {
		t.Fatalf("samples = %d, want 3", samples)
	}
	if mean != 20 {
		t.Fatalf("mean = %v, want 20", mean)
	}
	if m2 := s.models["sonnet"].M2; m2 != 200 {
		t.Fatalf("m2 = %v, want 200", m2)
	}
}

func TestOutliersDiscarded(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "telemetry.json"))
	for _, tps := range []float64{0, -5, 20000} {
		s.Record("sonnet", tps)
	}
	if _, samples := s.GetMean("sonnet"); samples != 0 {
		t.Fatalf("samples = %d, want 0 after outliers only", samples)
	}
	s.Record("sonnet", 50)
	mean, samples := s.GetMean("sonnet")
	if samples != 1 || mean != 50 {
		t.Fatalf("got (%v, %d), want (50, 1) after valid sample", mean, samples)
	}
	s.Record("sonnet", 10000)
	if _, samples := s.GetMean("sonnet"); samples != 2 {
		t.Fatalf("samples = %d, want 2 (10000 is the valid boundary)", samples)
	}
	s.Record("sonnet", 10000.0001)
	if _, samples := s.GetMean("sonnet"); samples != 2 {
		t.Fatalf("samples = %d, want 2 (above 10000 must be discarded)", samples)
	}
	if mean, _ := s.GetMean("sonnet"); mean != 5025 {
		t.Fatalf("mean = %v, want 5025 ((50+10000)/2)", mean)
	}
}

func TestEWMAConverges(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "telemetry.json"))
	s.Record("sonnet", 10)
	s.Record("sonnet", 20)
	if got := s.models["sonnet"].EWMA; !almostEq(got, 13) {
		t.Fatalf("ewma after [10, 20] = %v, want 13 (10*0.7 + 20*0.3)", got)
	}
	for i := 0; i < 10; i++ {
		s.Record("haiku", 100)
	}
	ewma, samples := s.Get("haiku")
	if samples != 10 {
		t.Fatalf("samples = %d, want 10", samples)
	}
	if !almostEq(ewma, 100) {
		t.Fatalf("ewma = %v, want ~100 after constant sequence", ewma)
	}
}

func TestGetReturnsZeroBelowFiveSamples(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "telemetry.json"))
	if ewma, samples := s.Get("sonnet"); ewma != 0 || samples != 0 {
		t.Fatalf("get on unknown model = (%v, %d), want (0, 0)", ewma, samples)
	}
	for _, tps := range []float64{10, 20, 30, 40} {
		s.Record("sonnet", tps)
	}
	if ewma, samples := s.Get("sonnet"); ewma != 0 || samples != 4 {
		t.Fatalf("get below threshold = (%v, %d), want (0, 4)", ewma, samples)
	}
	s.Record("sonnet", 50)
	ewma, samples := s.Get("sonnet")
	if samples != 5 {
		t.Fatalf("samples = %d, want 5", samples)
	}
	if !almostEq(ewma, 32.269) {
		t.Fatalf("ewma = %v, want 32.269 (hand-computed chain with alpha 0.3)", ewma)
	}
}

func TestPersistenceRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telemetry.json")
	s := New(path)
	for _, tps := range []float64{10, 20, 30} {
		s.Record("sonnet", tps)
	}
	s.Record("haiku", 42.5)
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat telemetry file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %v, want 0600", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read telemetry file: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal telemetry file: %v", err)
	}
	if _, ok := raw["models"]; !ok {
		t.Fatalf("telemetry file missing top-level models key: %s", data)
	}
	loaded := Load(path)
	got := loaded.models["sonnet"]
	want := s.models["sonnet"]
	if got == nil || got.Samples != want.Samples || !almostEq(got.Mean, want.Mean) || !almostEq(got.M2, want.M2) || !almostEq(got.EWMA, want.EWMA) {
		t.Fatalf("sonnet stats mismatch after reload: got %+v want %+v", got, want)
	}
	mean, samples := loaded.GetMean("sonnet")
	if samples != 3 || !almostEq(mean, 20) {
		t.Fatalf("loaded mean = (%v, %d), want (20, 3)", mean, samples)
	}
	if got := loaded.models["haiku"]; got == nil || !almostEq(got.EWMA, 42.5) {
		t.Fatalf("haiku stats lost after reload: %+v", got)
	}
	loaded.Record("sonnet", 40)
	mean, samples = loaded.GetMean("sonnet")
	if samples != 4 || !almostEq(mean, 25) {
		t.Fatalf("record after reload = (%v, %d), want (25, 4)", mean, samples)
	}
}

func TestSaveDebounce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telemetry.json")
	s := New(path)
	current := time.Unix(1700000000, 0)
	s.now = func() time.Time { return current }

	s.Record("sonnet", 10)
	s.Record("sonnet", 20)
	if st := persistedStats(t, path, "sonnet"); st == nil || st.Samples != 1 {
		t.Fatalf("persisted samples after two records at same instant = %+v, want 1", st)
	}

	current = current.Add(11 * time.Second)
	s.Record("sonnet", 30)
	if st := persistedStats(t, path, "sonnet"); st == nil || st.Samples != 3 {
		t.Fatalf("persisted samples after debounce window = %+v, want 3", st)
	}

	s.Record("sonnet", 40)
	if st := persistedStats(t, path, "sonnet"); st == nil || st.Samples != 3 {
		t.Fatalf("persisted samples within debounce window = %+v, want 3 (no save)", st)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if st := persistedStats(t, path, "sonnet"); st == nil || st.Samples != 4 {
		t.Fatalf("persisted samples after close = %+v, want 4 (close always saves)", st)
	}
}

func TestLoadMissingFileStartsFresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "telemetry.json")
	s := Load(path)
	if mean, samples := s.GetMean("sonnet"); mean != 0 || samples != 0 {
		t.Fatalf("fresh store mean = (%v, %d), want (0, 0)", mean, samples)
	}
	s.Record("sonnet", 33)
	mean, samples := s.GetMean("sonnet")
	if samples != 1 || !almostEq(mean, 33) {
		t.Fatalf("record on fresh store = (%v, %d), want (33, 1)", mean, samples)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("telemetry file not created by fresh store: %v", err)
	}
}

func TestLoadCorruptJSONStartsFresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telemetry.json")
	if err := os.WriteFile(path, []byte(`{"models": "not a map"`), 0o600); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}
	s := Load(path)
	if mean, samples := s.GetMean("sonnet"); mean != 0 || samples != 0 {
		t.Fatalf("corrupt load mean = (%v, %d), want (0, 0)", mean, samples)
	}
	s.Record("sonnet", 10)
	if mean, samples := s.GetMean("sonnet"); samples != 1 || !almostEq(mean, 10) {
		t.Fatalf("record after corrupt load = (%v, %d), want (10, 1)", mean, samples)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	loaded := Load(path)
	if mean, samples := loaded.GetMean("sonnet"); samples != 1 || !almostEq(mean, 10) {
		t.Fatalf("reload after recovery = (%v, %d), want (10, 1)", mean, samples)
	}
}

func TestConcurrentAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telemetry.json")
	s := New(path)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				s.Record("sonnet", float64(50+g))
				s.Get("sonnet")
				s.GetMean("sonnet")
			}
		}(g)
	}
	wg.Wait()
	if _, samples := s.GetMean("sonnet"); samples != 200 {
		t.Fatalf("samples = %d, want 200", samples)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if st := persistedStats(t, path, "sonnet"); st == nil || st.Samples != 200 {
		t.Fatalf("persisted samples = %+v, want 200", st)
	}
}

func TestNilStoreIsSafe(t *testing.T) {
	var s *Store
	s.Record("sonnet", 10)
	if ewma, samples := s.Get("sonnet"); ewma != 0 || samples != 0 {
		t.Fatalf("get on nil store = (%v, %d), want (0, 0)", ewma, samples)
	}
	if mean, samples := s.GetMean("sonnet"); mean != 0 || samples != 0 {
		t.Fatalf("getMean on nil store = (%v, %d), want (0, 0)", mean, samples)
	}
	if err := s.Save(); err != nil {
		t.Fatalf("save on nil store: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close on nil store: %v", err)
	}
}

func TestPersistFailureIsBestEffort(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	s := New(filepath.Join(blocker, "telemetry.json"))
	s.Record("sonnet", 10)
	s.Record("sonnet", 20)
	mean, samples := s.GetMean("sonnet")
	if samples != 2 || !almostEq(mean, 15) {
		t.Fatalf("in-memory stats after persist failure = (%v, %d), want (15, 2)", mean, samples)
	}
	if err := s.Close(); err == nil {
		t.Fatalf("want error from close when path is unwritable")
	}
}
