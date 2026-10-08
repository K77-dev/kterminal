package catalog

import (
	"strings"
	"testing"
)

func TestCriteriaUsesEstimateByDefault(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	m, ok := c.Get("glm-5.2")
	if !ok {
		t.Fatalf("glm-5.2 not in catalog")
	}
	if m.TPSEstimate != 70 {
		t.Fatalf("tps estimate = %v, want 70", m.TPSEstimate)
	}
	got := m.Criteria()
	if !strings.Contains(got, "~70 tok/s") {
		t.Fatalf("criteria missing estimate speed: %q", got)
	}
	if strings.Contains(got, "measured") {
		t.Fatalf("criteria must not mention measured without samples: %q", got)
	}
}

func TestCriteriaUsesMeasuredWhenAvailable(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	m, ok := c.Get("glm-5.2")
	if !ok {
		t.Fatalf("glm-5.2 not in catalog")
	}
	m.MeasuredTPS = 92.4
	m.MeasuredSamples = 7
	got := m.Criteria()
	if !strings.Contains(got, "~92 tok/s") {
		t.Fatalf("criteria missing measured speed: %q", got)
	}
	if !strings.Contains(got, "measured over 7 calls") {
		t.Fatalf("criteria missing measured sample count: %q", got)
	}
	if strings.Contains(got, "~70 tok/s") {
		t.Fatalf("criteria must not cite estimate when measured is set: %q", got)
	}
}

func TestVisionParsedFromYAML(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	m, ok := c.Get("glm-5.3")
	if !ok {
		t.Fatalf("glm-5.3 not in catalog")
	}
	if !m.Vision {
		t.Fatalf("glm-5.3 vision = false, want true")
	}
	for _, m := range c.Models {
		if m.Name == "glm-5.3" {
			continue
		}
		if m.Vision {
			t.Fatalf("%s vision = true, want false", m.Name)
		}
	}
}

func TestParseIgnoresMeasuredFields(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	if len(c.Models) == 0 {
		t.Fatalf("catalog has no models")
	}
	for _, m := range c.Models {
		if m.MeasuredTPS != 0 || m.MeasuredSamples != 0 {
			t.Fatalf("%s parsed with measured fields (%v, %d), want zero values", m.Name, m.MeasuredTPS, m.MeasuredSamples)
		}
	}
}
