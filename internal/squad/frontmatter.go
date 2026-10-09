package squad

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

type frontmatter struct {
	Name       string   `yaml:"name"`
	Discipline string   `yaml:"discipline"`
	ModelTags  []string `yaml:"model-tags"`
	Rules      []string `yaml:"rules"`
}

func parseFrontmatter(data []byte) (frontmatter, error) {
	var fm frontmatter
	block, _, err := splitFrontmatter(data)
	if err != nil {
		return fm, err
	}
	if err := yaml.Unmarshal(block, &fm); err != nil {
		return fm, fmt.Errorf("parse frontmatter: %w", err)
	}
	if fm.Name == "" {
		return fm, fmt.Errorf("parse frontmatter: missing name")
	}
	return fm, nil
}

func frontmatterBody(data []byte) ([]byte, error) {
	_, body, err := splitFrontmatter(data)
	return body, err
}

func splitFrontmatter(data []byte) (block, body []byte, err error) {
	lines := bytes.Split(data, []byte("\n"))
	if len(lines) < 2 || !isDelimiter(lines[0]) {
		return nil, nil, fmt.Errorf("parse frontmatter: missing opening delimiter")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if isDelimiter(lines[i]) {
			end = i
			break
		}
	}
	if end < 0 {
		return nil, nil, fmt.Errorf("parse frontmatter: missing closing delimiter")
	}
	return bytes.Join(lines[1:end], []byte("\n")), bytes.Join(lines[end+1:], []byte("\n")), nil
}

func isDelimiter(line []byte) bool {
	return string(bytes.TrimSuffix(line, []byte("\r"))) == "---"
}
