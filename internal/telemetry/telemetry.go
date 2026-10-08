package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	ewmaAlpha          = 0.3
	measuredMinSamples = 5
	saveDebounce       = 10 * time.Second
	outlierMaxTPS      = 10000
)

type Stats struct {
	Samples int     `json:"samples"`
	Mean    float64 `json:"mean"`
	M2      float64 `json:"m2"`
	EWMA    float64 `json:"ewma"`
}

type diskStore struct {
	Models map[string]*Stats `json:"models"`
}

type Store struct {
	mu       sync.Mutex
	models   map[string]*Stats
	path     string
	lastSave time.Time
	now      func() time.Time
}

func New(path string) *Store {
	return &Store{
		models: map[string]*Stats{},
		path:   path,
		now:    time.Now,
	}
}

func Load(path string) *Store {
	s := New(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	var d diskStore
	if err := json.Unmarshal(data, &d); err != nil {
		return s
	}
	for name, st := range d.Models {
		if st != nil {
			s.models[name] = st
		}
	}
	return s
}

func (s *Store) Record(model string, tps float64) {
	if s == nil || tps <= 0 || tps > outlierMaxTPS {
		return
	}
	s.mu.Lock()
	st := s.statsFor(model)
	st.Samples++
	delta := tps - st.Mean
	st.Mean += delta / float64(st.Samples)
	st.M2 += delta * (tps - st.Mean)
	if st.Samples == 1 {
		st.EWMA = tps
	} else {
		st.EWMA = st.EWMA*(1-ewmaAlpha) + tps*ewmaAlpha
	}
	s.mu.Unlock()
	_ = s.Save()
}

func (s *Store) Get(model string) (float64, int) {
	if s == nil {
		return 0, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.models[model]
	if st == nil {
		return 0, 0
	}
	if st.Samples < measuredMinSamples {
		return 0, st.Samples
	}
	return st.EWMA, st.Samples
}

func (s *Store) GetMean(model string) (float64, int) {
	if s == nil {
		return 0, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.models[model]
	if st == nil {
		return 0, 0
	}
	return st.Mean, st.Samples
}

func (s *Store) Save() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.now().Sub(s.lastSave) < saveDebounce {
		return nil
	}
	return s.persist()
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.persist()
}

func (s *Store) statsFor(model string) *Stats {
	st := s.models[model]
	if st == nil {
		st = &Stats{}
		s.models[model] = st
	}
	return st
}

func (s *Store) persist() error {
	data, err := json.Marshal(diskStore{Models: s.models})
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := s.writeAtomic(dir, data); err != nil {
		return err
	}
	s.lastSave = s.now()
	return nil
}

func (s *Store) writeAtomic(dir string, data []byte) error {
	tmp, err := os.CreateTemp(dir, ".telemetry-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}
