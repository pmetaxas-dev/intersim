package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"intersim/internal/app"
)

func TestLoadQuestions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	validPath := filepath.Join(dir, "questions.json")
	validJSON := `[
		{"id":1,"category":"Go","question":"Q1"},
		{"id":2,"category":"Go","question":"Q2"},
		{"id":3,"category":"Go","question":"Q3"},
		{"id":4,"category":"Go","question":"Q4"},
		{"id":5,"category":"Go","question":"Q5"}
	]`
	if err := os.WriteFile(validPath, []byte(validJSON), 0o600); err != nil {
		t.Fatal(err)
	}

	questions, err := app.LoadQuestions(validPath)
	if err != nil {
		t.Fatalf("LoadQuestions() error = %v", err)
	}
	if len(questions) != 5 {
		t.Fatalf("LoadQuestions() count = %d, want 5", len(questions))
	}

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "malformed JSON", content: `[`, want: "decode"},
		{name: "too few", content: `[{"id":1,"category":"Go","question":"Q1"}]`, want: "at least 5"},
		{name: "duplicate ID", content: strings.Replace(validJSON, `"id":5`, `"id":1`, 1), want: "duplicate"},
		{name: "empty category", content: strings.Replace(validJSON, `"category":"Go"`, `"category":""`, 1), want: "category"},
		{name: "empty question", content: strings.Replace(validJSON, `"question":"Q1"`, `"question":""`, 1), want: "question"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(tc.name, " ", "_")+".json")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := app.LoadQuestions(path)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("LoadQuestions() error = %v, want containing %q", err, tc.want)
			}
		})
	}

	if _, err := app.LoadQuestions(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("LoadQuestions() missing file error = nil")
	}
}

func TestNewRejectsInvalidQuestionPool(t *testing.T) {
	t.Parallel()

	_, err := app.New(app.Config{Questions: testQuestions()[:4]})
	if err == nil {
		t.Fatal("New() error = nil, want invalid question pool error")
	}
}
