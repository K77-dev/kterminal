package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultBaseURL = "https://api.typesafe.ai"
	DefaultModel   = "jev-latest"
)

type choiceQuestion struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

type request struct {
	State     string                    `json:"state"`
	Model     string                    `json:"model"`
	Questions map[string]choiceQuestion `json:"questions"`
}

type choiceAnswer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}

type response struct {
	Model   string                  `json:"model"`
	Answers map[string]choiceAnswer `json:"answers"`
}

type Client struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

func New(apiKey string) *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		APIKey:  apiKey,
		Model:   DefaultModel,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) Decide(ctx context.Context, state, instructions string, criteria map[string]string) (choice string, confidence float64, probabilities map[string]float64, err error) {
	body := request{
		State: state,
		Model: c.Model,
		Questions: map[string]choiceQuestion{
			"model_choice": {
				Type:         "choice",
				Instructions: instructions,
				Criteria:     criteria,
			},
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", 0, nil, err
	}
	url := strings.TrimRight(c.BaseURL, "/") + "/v1/systemone"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", 0, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", 0, nil, fmt.Errorf("typesafe systemone: %s: %s", resp.Status, string(b))
	}
	var out response
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", 0, nil, fmt.Errorf("decode typesafe response: %w", err)
	}
	ans, ok := out.Answers["model_choice"]
	if !ok {
		return "", 0, nil, fmt.Errorf("typesafe response missing model_choice answer")
	}
	if ans.Choice == "" {
		return "", 0, nil, fmt.Errorf("typesafe returned empty choice")
	}
	return ans.Choice, ans.Confidence, ans.Probabilities, nil
}
