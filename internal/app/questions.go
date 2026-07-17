package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const interviewQuestionCount = 5

// LoadQuestions reads and validates a JSON question pool.
func LoadQuestions(path string) ([]Question, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read questions: %w", err)
	}

	var questions []Question
	if err := json.Unmarshal(data, &questions); err != nil {
		return nil, fmt.Errorf("decode questions: %w", err)
	}
	if err := validateQuestions(questions); err != nil {
		return nil, err
	}
	return questions, nil
}

// validateQuestions checks that questions are complete and uniquely identified.
func validateQuestions(questions []Question) error {
	if len(questions) < interviewQuestionCount {
		return fmt.Errorf("question pool must contain at least %d questions", interviewQuestionCount)
	}

	ids := make(map[int]struct{}, len(questions))
	for index, question := range questions {
		if question.ID <= 0 {
			return fmt.Errorf("question %d must have a positive id", index+1)
		}
		if _, exists := ids[question.ID]; exists {
			return fmt.Errorf("duplicate question id %d", question.ID)
		}
		ids[question.ID] = struct{}{}
		if strings.TrimSpace(question.Category) == "" {
			return fmt.Errorf("question %d category must not be empty", question.ID)
		}
		if strings.TrimSpace(question.Question) == "" {
			return fmt.Errorf("question %d question text must not be empty", question.ID)
		}
	}
	return nil
}
