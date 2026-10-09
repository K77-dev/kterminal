package catalog

import (
	_ "embed"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed models.yaml
var modelsYAML []byte

type Model struct {
	Name            string   `yaml:"name" json:"name"`
	Description     string   `yaml:"description" json:"description"`
	Strengths       string   `yaml:"strengths" json:"strengths"`
	InputPricePerM  float64  `yaml:"input_price_per_m" json:"input_price_per_m"`
	OutputPricePerM float64  `yaml:"output_price_per_m" json:"output_price_per_m"`
	TPSEstimate     float64  `yaml:"tps_estimate" json:"tps_estimate"`
	ContextWindow   int      `yaml:"context_window" json:"context_window"`
	Tags            []string `yaml:"tags" json:"tags"`
	Vision          bool     `yaml:"vision"`
	MeasuredTPS     float64
	MeasuredSamples int
}

type fileCatalog struct {
	Models       []Model `yaml:"models"`
	DefaultModel string  `yaml:"default_model"`
}

type Catalog struct {
	Models       []Model
	DefaultModel string
	byName       map[string]Model
}

func Load() (*Catalog, error) {
	return parse(modelsYAML)
}

func Parse(data []byte) (*Catalog, error) {
	return parse(data)
}

func parse(data []byte) (*Catalog, error) {
	var fc fileCatalog
	if err := yaml.Unmarshal(data, &fc); err != nil {
		return nil, fmt.Errorf("parse models.yaml: %w", err)
	}
	if len(fc.Models) == 0 {
		return nil, fmt.Errorf("models.yaml: no models")
	}
	def := fc.DefaultModel
	if def == "" {
		def = fc.Models[0].Name
	}
	c := &Catalog{Models: fc.Models, DefaultModel: def, byName: map[string]Model{}}
	for _, m := range fc.Models {
		c.byName[m.Name] = m
	}
	if _, ok := c.byName[def]; !ok {
		return nil, fmt.Errorf("models.yaml: default_model %q not in models", def)
	}
	return c, nil
}

func (c *Catalog) Get(name string) (Model, bool) {
	m, ok := c.byName[name]
	return m, ok
}

func (c *Catalog) Available(remote []string) []Model {
	remoteSet := map[string]bool{}
	for _, r := range remote {
		remoteSet[r] = true
	}
	var out []Model
	for _, m := range c.Models {
		if remoteSet[m.Name] {
			out = append(out, m)
		}
	}
	return out
}

func (m Model) Cost(inputTokens, outputTokens int64) float64 {
	return float64(inputTokens)/1e6*m.InputPricePerM + float64(outputTokens)/1e6*m.OutputPricePerM
}

func (m Model) Criteria() string {
	speed := fmt.Sprintf("Speed: ~%.0f tok/s.", m.TPSEstimate)
	if m.MeasuredTPS > 0 {
		speed = fmt.Sprintf("Speed: ~%.0f tok/s (measured over %d calls).", m.MeasuredTPS, m.MeasuredSamples)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %s. Strengths: %s. Price: $%.2f/M input, $%.2f/M output. %s Context: %d tokens.",
		m.Name, m.Description, m.Strengths, m.InputPricePerM, m.OutputPricePerM, speed, m.ContextWindow)
	if len(m.Tags) > 0 {
		fmt.Fprintf(&b, " Tags: %s.", strings.Join(m.Tags, ", "))
	}
	return b.String()
}
