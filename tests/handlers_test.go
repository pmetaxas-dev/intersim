package tests

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"intersim/internal/app"
)

func evaluationResponse(followUp string) string {
	content := fmt.Sprintf(`{"score":90,"feedbackGood":"Accurate","feedbackBad":"Add detail","followUpQuestion":%q}`, followUp)
	return fmt.Sprintf(`{"choices":[{"message":{"content":%q}}]}`, content)
}

func TestConfigPreflight(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		fallback    string
		requiresKey bool
	}{
		{name: "key required", fallback: "", requiresKey: true},
		{name: "environment fallback", fallback: "env-key", requiresKey: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			application, _ := newTestApp(t, tc.fallback, func(http.ResponseWriter, *http.Request) {})
			response := request(t, application.Handler(), http.MethodGet, "/api/config", "")
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			payload := decodeJSON[struct {
				RequiresAPIKey bool `json:"requiresApiKey"`
			}](t, response)
			if payload.RequiresAPIKey != tc.requiresKey {
				t.Fatalf("requiresApiKey = %v, want %v", payload.RequiresAPIKey, tc.requiresKey)
			}
		})
	}
}

func TestStartInterviewValidationAndSelection(t *testing.T) {
	t.Parallel()

	application, _ := newTestApp(t, "", func(http.ResponseWriter, *http.Request) {})
	handler := application.Handler()

	tests := []struct {
		name   string
		method string
		body   string
		status int
	}{
		{name: "wrong method", method: http.MethodGet, status: http.StatusMethodNotAllowed},
		{name: "malformed JSON", method: http.MethodPost, body: `{`, status: http.StatusBadRequest},
		{name: "unknown field", method: http.MethodPost, body: `{"unexpected":true}`, status: http.StatusBadRequest},
		{name: "multiple objects", method: http.MethodPost, body: `{} {}`, status: http.StatusBadRequest},
		{name: "oversized body", method: http.MethodPost, body: `{"apiKey":"` + strings.Repeat("a", (1<<20)+1) + `"}`, status: http.StatusBadRequest},
		{name: "missing required key", method: http.MethodPost, body: `{}`, status: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			response := request(t, handler, tc.method, "/api/start", tc.body)
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d; body = %q", response.Code, tc.status, response.Body.String())
			}
		})
	}

	response := request(t, handler, http.MethodPost, "/api/start", `{"apiKey":"browser-key"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("valid start status = %d, want %d; body = %q", response.Code, http.StatusOK, response.Body.String())
	}
	question := decodeJSON[app.Question](t, response)
	if question.ID < 1 || question.ID > 6 {
		t.Fatalf("question ID = %d, want an ID from the configured pool", question.ID)
	}
}

func TestFallbackKeyAndSubmittedKeyPrecedence(t *testing.T) {
	t.Parallel()

	var authorization atomic.Value
	application, _ := newTestApp(t, "env-key", func(w http.ResponseWriter, r *http.Request) {
		authorization.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, evaluationResponse("Follow-up"))
	})
	handler := application.Handler()

	response := request(t, handler, http.MethodPost, "/api/start", `{}`)
	if response.Code != http.StatusOK {
		t.Fatalf("fallback start status = %d, want 200", response.Code)
	}
	question := decodeJSON[app.Question](t, response)
	response = request(t, handler, http.MethodPost, "/api/answer", fmt.Sprintf(`{"questionId":%d,"answer":"Answer"}`, question.ID))
	if response.Code != http.StatusOK {
		t.Fatalf("fallback answer status = %d, want 200; body = %q", response.Code, response.Body.String())
	}
	if got := authorization.Load(); got != "Bearer env-key" {
		t.Fatalf("Authorization = %q, want fallback key", got)
	}

	response = request(t, handler, http.MethodPost, "/api/start", `{"apiKey":"browser-key"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("submitted-key start status = %d, want 200", response.Code)
	}
	question = decodeJSON[app.Question](t, response)
	response = request(t, handler, http.MethodPost, "/api/answer", fmt.Sprintf(`{"questionId":%d,"answer":"Answer"}`, question.ID))
	if response.Code != http.StatusOK {
		t.Fatalf("submitted-key answer status = %d, want 200", response.Code)
	}
	if got := authorization.Load(); got != "Bearer browser-key" {
		t.Fatalf("Authorization = %q, want submitted key", got)
	}
}

func TestAnswerStateTransitionsAndCompletion(t *testing.T) {
	t.Parallel()

	application, _ := newTestApp(t, "env-key", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		userMessage, err := readGroqUserMessage(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.Contains(userMessage, "Context: Follow-up") {
			fmt.Fprint(w, evaluationResponse(""))
			return
		}
		fmt.Fprint(w, evaluationResponse("Explain further"))
	})
	handler := application.Handler()

	beforeStart := request(t, handler, http.MethodPost, "/api/answer", `{"questionId":1,"answer":"Answer"}`)
	if beforeStart.Code != http.StatusConflict {
		t.Fatalf("answer before start status = %d, want %d", beforeStart.Code, http.StatusConflict)
	}
	start := request(t, handler, http.MethodPost, "/api/start", `{}`)
	currentQuestion := decodeJSON[app.Question](t, start)

	for answerNumber := 1; answerNumber <= 10; answerNumber++ {
		response := request(t, handler, http.MethodPost, "/api/answer", fmt.Sprintf(`{"questionId":%d,"answer":"A useful answer"}`, currentQuestion.ID))
		if response.Code != http.StatusOK {
			t.Fatalf("answer %d status = %d, want 200; body = %q", answerNumber, response.Code, response.Body.String())
		}
		payload := decodeJSON[struct {
			NextQuestion *app.Question `json:"nextQuestion"`
			IsFinished   bool          `json:"isFinished"`
		}](t, response)
		if answerNumber == 10 {
			if !payload.IsFinished || payload.NextQuestion != nil {
				t.Fatalf("final answer = finished %v, next %#v", payload.IsFinished, payload.NextQuestion)
			}
			continue
		}
		if payload.IsFinished || payload.NextQuestion == nil {
			t.Fatalf("answer %d ended interview unexpectedly", answerNumber)
		}
		if answerNumber%2 == 1 && payload.NextQuestion.Category != "Follow-up" {
			t.Fatalf("answer %d next category = %q, want Follow-up", answerNumber, payload.NextQuestion.Category)
		}
		if answerNumber%2 == 0 && payload.NextQuestion.Category == "Follow-up" {
			t.Fatalf("answer %d returned another follow-up", answerNumber)
		}
		currentQuestion = *payload.NextQuestion
	}
}

func TestAnswerRequestValidation(t *testing.T) {
	t.Parallel()

	application, _ := newTestApp(t, "env-key", func(http.ResponseWriter, *http.Request) {})
	handler := application.Handler()
	request(t, handler, http.MethodPost, "/api/start", `{}`)

	tests := []struct {
		name   string
		method string
		body   string
		status int
	}{
		{name: "wrong method", method: http.MethodGet, status: http.StatusMethodNotAllowed},
		{name: "malformed JSON", method: http.MethodPost, body: `{`, status: http.StatusBadRequest},
		{name: "blank answer", method: http.MethodPost, body: `{"answer":"  "}`, status: http.StatusBadRequest},
		{name: "wrong question ID", method: http.MethodPost, body: `{"questionId":999,"answer":"Answer"}`, status: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			response := request(t, handler, tc.method, "/api/answer", tc.body)
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d; body = %q", response.Code, tc.status, response.Body.String())
			}
		})
	}
}
