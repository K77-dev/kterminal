package router

import (
	"math"
	"testing"
)

func TestMinConfidenceScalesWithCandidates(t *testing.T) {
	cases := []struct {
		candidates int
		want       float64
	}{
		{1, 0.5},
		{2, 0.5},
		{4, 0.5},
		{5, 0.4},
		{6, 2.0 / 6.0},
		{7, 0.3},
		{20, 0.3},
		{0, 0.5},
	}
	for _, c := range cases {
		if got := MinConfidence(c.candidates); math.Abs(got-c.want) > 1e-9 {
			t.Fatalf("MinConfidence(%d) = %.4f, want %.4f", c.candidates, got, c.want)
		}
	}
}
