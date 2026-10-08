package tools

import (
	"context"
	"fmt"
	"strings"

	"kterminal/internal/llm"
)

const AskUserToolName = "ask_user"

type AskOption struct {
	Label       string
	Description string
}

type AskQuestion struct {
	Question string
	Header   string
	Options  []AskOption
	Multiple bool
}

func AskUserTool(ask func(questions []AskQuestion, answerCh chan []string)) Tool {
	return Tool{
		Name: AskUserToolName,
		Schema: llm.Tool{
			Type: "function",
			Function: llm.Function{
				Name:        AskUserToolName,
				Description: "Ask the user structured questions and wait for the answers. Questions are shown one per screen with keyboard-navigable options, optional multi-select and a free-text fallback. Use it whenever you need clarification, a decision or a preference instead of guessing.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"questions": map[string]any{
							"type":        "array",
							"description": "Questions to ask, presented sequentially (one per screen)",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"question": map[string]any{"type": "string", "description": "Question text shown to the user"},
									"header":   map[string]any{"type": "string", "description": "Short header shown above the question (optional)"},
									"multiple": map[string]any{"type": "boolean", "description": "Allow selecting multiple options (default false)"},
									"options": map[string]any{
										"type":        "array",
										"description": "Navigable answer options (optional — free text is always available)",
										"items": map[string]any{
											"type": "object",
											"properties": map[string]any{
												"label":       map[string]any{"type": "string", "description": "Option label"},
												"description": map[string]any{"type": "string", "description": "Optional explanation shown under the label"},
											},
											"required": []string{"label"},
										},
									},
								},
								"required": []string{"question"},
							},
						},
					},
					"required": []string{"questions"},
				},
			},
		},
		Execute: func(ctx context.Context, args map[string]any) (Result, error) {
			questions, err := ParseAskQuestions(args)
			if err != nil {
				return Result{}, err
			}
			answerCh := make(chan []string, 1)
			ask(questions, answerCh)
			select {
			case answers := <-answerCh:
				if answers == nil {
					return Result{Output: "user declined to answer"}, nil
				}
				return Result{Output: FormatAskResult(questions, answers)}, nil
			case <-ctx.Done():
				return Result{}, ctx.Err()
			}
		},
	}
}

func ParseAskQuestions(args map[string]any) ([]AskQuestion, error) {
	raw, ok := args["questions"]
	if !ok {
		return nil, fmt.Errorf("missing argument %q", "questions")
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("argument %q must be an array", "questions")
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("argument %q must not be empty", "questions")
	}
	questions := make([]AskQuestion, 0, len(list))
	for i, item := range list {
		qmap, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("questions[%d] must be an object", i)
		}
		question, err := str(qmap, "question")
		if err != nil {
			return nil, fmt.Errorf("questions[%d]: %w", i, err)
		}
		q := AskQuestion{
			Question: question,
			Header:   optStr(qmap, "header"),
			Multiple: optBool(qmap, "multiple"),
		}
		optsRaw, ok := qmap["options"]
		if ok && optsRaw != nil {
			optsList, ok := optsRaw.([]any)
			if !ok {
				return nil, fmt.Errorf("questions[%d]: options must be an array", i)
			}
			for j, optItem := range optsList {
				omap, ok := optItem.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("questions[%d].options[%d] must be an object", i, j)
				}
				label, err := str(omap, "label")
				if err != nil {
					return nil, fmt.Errorf("questions[%d].options[%d]: %w", i, j, err)
				}
				q.Options = append(q.Options, AskOption{Label: label, Description: optStr(omap, "description")})
			}
		}
		questions = append(questions, q)
	}
	return questions, nil
}

func FormatAskResult(questions []AskQuestion, answers []string) string {
	var b strings.Builder
	for i, q := range questions {
		if i >= len(answers) {
			break
		}
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("Q: ")
		b.WriteString(q.Question)
		b.WriteString("\nA: ")
		b.WriteString(answers[i])
	}
	return b.String()
}

func optBool(args map[string]any, key string) bool {
	v, _ := args[key].(bool)
	return v
}
