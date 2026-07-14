package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"intersim/internal/app"
)

type groqRequest struct {
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func readGroqUserMessage(r *http.Request) (string, error) {
	var payload groqRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode Groq request: %w", err)
	}
	if len(payload.Messages) != 2 {
		return "", fmt.Errorf("Groq message count = %d, want 2", len(payload.Messages))
	}
	return payload.Messages[1].Content, nil
}

func TestEmptyEvaluationFeedbackUsesFallbackText(t *testing.T) {
	t.Parallel()

	content := `{"score":100,"feedbackGood":"","feedbackBad":"","followUpQuestion":"Show another approach"}`
	application, _ := newTestApp(t, "env-key", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, content)
	})
	handler := application.Handler()
	start := request(t, handler, http.MethodPost, "/api/start", `{}`)
	question := decodeJSON[app.Question](t, start)

	response := request(t, handler, http.MethodPost, "/api/answer", fmt.Sprintf(`{"questionId":%d,"answer":"Answer"}`, question.ID))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %q", response.Code, response.Body.String())
	}
	payload := decodeJSON[struct {
		Evaluation app.AIResponse `json:"evaluation"`
	}](t, response)
	if payload.Evaluation.FeedbackGood != "No specific strengths were identified." {
		t.Fatalf("feedbackGood = %q, want fallback text", payload.Evaluation.FeedbackGood)
	}
	if payload.Evaluation.FeedbackBad != "No significant issues were identified." {
		t.Fatalf("feedbackBad = %q, want fallback text", payload.Evaluation.FeedbackBad)
	}
}

func TestMalformedEvaluationRetriesOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		firstContent string
	}{
		{name: "invalid JSON", firstContent: `not JSON`},
		{name: "missing follow-up", firstContent: `{"score":90,"feedbackGood":"Correct","feedbackBad":"","followUpQuestion":""}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			application, _ := newTestApp(t, "env-key", func(w http.ResponseWriter, _ *http.Request) {
				content := tc.firstContent
				if calls.Add(1) == 2 {
					content = `{"score":90,"feedbackGood":"Correct","feedbackBad":"","followUpQuestion":"Follow-up"}`
				}
				fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, content)
			})
			handler := application.Handler()
			start := request(t, handler, http.MethodPost, "/api/start", `{}`)
			question := decodeJSON[app.Question](t, start)

			response := request(t, handler, http.MethodPost, "/api/answer", fmt.Sprintf(`{"questionId":%d,"answer":"Answer"}`, question.ID))
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body = %q", response.Code, response.Body.String())
			}
			if calls.Load() != 2 {
				t.Fatalf("Groq calls = %d, want 2", calls.Load())
			}
		})
	}
}

func TestPersistentMalformedEvaluationDoesNotAdvanceInterview(t *testing.T) {
	t.Parallel()

	var valid atomic.Bool
	var calls atomic.Int32
	application, _ := newTestApp(t, "env-key", func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		if valid.Load() {
			fmt.Fprint(w, evaluationResponse("Follow-up"))
			return
		}
		fmt.Fprint(w, `{"choices":[]}`)
	})
	handler := application.Handler()
	start := request(t, handler, http.MethodPost, "/api/start", `{}`)
	question := decodeJSON[app.Question](t, start)

	response := request(t, handler, http.MethodPost, "/api/answer", fmt.Sprintf(`{"questionId":%d,"answer":"Answer"}`, question.ID))
	if response.Code != http.StatusBadGateway {
		t.Fatalf("malformed status = %d, want %d; body = %q", response.Code, http.StatusBadGateway, response.Body.String())
	}
	if calls.Load() != 2 {
		t.Fatalf("Groq calls after malformed response = %d, want 2", calls.Load())
	}

	valid.Store(true)
	response = request(t, handler, http.MethodPost, "/api/answer", fmt.Sprintf(`{"questionId":%d,"answer":"Answer"}`, question.ID))
	if response.Code != http.StatusOK {
		t.Fatalf("recovery status = %d, want 200; body = %q", response.Code, response.Body.String())
	}
}

func TestGroqHTTPErrorMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		groqStatus  int
		wantStatus  int
		wantMessage string
	}{
		{name: "invalid key", groqStatus: http.StatusUnauthorized, wantStatus: http.StatusUnauthorized, wantMessage: "API key"},
		{name: "model forbidden", groqStatus: http.StatusForbidden, wantStatus: http.StatusForbidden, wantMessage: "permission"},
		{name: "rate limited", groqStatus: http.StatusTooManyRequests, wantStatus: http.StatusTooManyRequests, wantMessage: "rate limit"},
		{name: "bad upstream request", groqStatus: http.StatusBadRequest, wantStatus: http.StatusBadGateway, wantMessage: "rejected"},
		{name: "service unavailable", groqStatus: http.StatusInternalServerError, wantStatus: http.StatusServiceUnavailable, wantMessage: "temporarily unavailable"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			application, _ := newTestApp(t, "env-key", func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "upstream details", tc.groqStatus)
			})
			handler := application.Handler()
			start := request(t, handler, http.MethodPost, "/api/start", `{}`)
			question := decodeJSON[app.Question](t, start)

			response := request(t, handler, http.MethodPost, "/api/answer", fmt.Sprintf(`{"questionId":%d,"answer":"Answer"}`, question.ID))
			if response.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body = %q", response.Code, tc.wantStatus, response.Body.String())
			}
			payload := decodeJSON[struct {
				Error string `json:"error"`
			}](t, response)
			if !strings.Contains(payload.Error, tc.wantMessage) {
				t.Fatalf("error = %q, want containing %q", payload.Error, tc.wantMessage)
			}
		})
	}
}

func TestGroqTransportFailureIsTemporarilyUnavailable(t *testing.T) {
	t.Parallel()

	application, groqServer := newTestApp(t, "env-key", func(http.ResponseWriter, *http.Request) {})
	groqServer.Close()
	handler := application.Handler()
	start := request(t, handler, http.MethodPost, "/api/start", `{}`)
	question := decodeJSON[app.Question](t, start)

	response := request(t, handler, http.MethodPost, "/api/answer", fmt.Sprintf(`{"questionId":%d,"answer":"Answer"}`, question.ID))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body = %q", response.Code, http.StatusServiceUnavailable, response.Body.String())
	}
	payload := decodeJSON[struct {
		Error string `json:"error"`
	}](t, response)
	if !strings.Contains(payload.Error, "temporarily unavailable") {
		t.Fatalf("error = %q, want temporary-unavailability message", payload.Error)
	}
}

func TestGroqDiagnosticsDoNotLogSecrets(t *testing.T) {
	t.Parallel()

	const (
		apiKey = "browser-secret-key"
		answer = "private interview answer"
	)
	groqServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream details", http.StatusUnauthorized)
	}))
	t.Cleanup(groqServer.Close)

	var logs bytes.Buffer
	application, err := app.New(app.Config{
		Questions:  testQuestions(),
		HTTPClient: groqServer.Client(),
		GroqURL:    groqServer.URL,
		Logger:     log.New(&logs, "", 0),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	handler := application.Handler()
	start := request(t, handler, http.MethodPost, "/api/start", fmt.Sprintf(`{"apiKey":%q}`, apiKey))
	question := decodeJSON[app.Question](t, start)
	response := request(t, handler, http.MethodPost, "/api/answer", fmt.Sprintf(`{"questionId":%d,"answer":%q}`, question.ID, answer))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}

	logged := logs.String()
	if strings.Contains(logged, apiKey) || strings.Contains(logged, answer) {
		t.Fatalf("diagnostic log contains a submitted secret: %q", logged)
	}
	if !strings.Contains(logged, "HTTP 401") {
		t.Fatalf("diagnostic log = %q, want safe upstream status", logged)
	}
}

func TestReportRequiresCompletedInterviewAndHandlesGroqResponse(t *testing.T) {
	t.Parallel()

	var reportCalls atomic.Int32
	application, _ := newTestApp(t, "env-key", func(w http.ResponseWriter, r *http.Request) {
		var payload groqRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(payload.Messages) > 0 && payload.Messages[0].Content == "Generate interview report JSON." {
			if reportCalls.Add(1) == 1 {
				fmt.Fprint(w, `{"choices":[]}`)
				return
			}
			content := `{"weaknesses":["Concurrency"],"studySuggestions":["Practice channels"],"finalScore":88}`
			fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, content)
			return
		}
		fmt.Fprint(w, evaluationResponse("Follow-up"))
	})
	handler := application.Handler()

	if response := request(t, handler, http.MethodGet, "/api/report", ""); response.Code != http.StatusConflict {
		t.Fatalf("report before start status = %d, want %d", response.Code, http.StatusConflict)
	}
	start := request(t, handler, http.MethodPost, "/api/start", `{}`)
	question := decodeJSON[app.Question](t, start)
	if response := request(t, handler, http.MethodGet, "/api/report", ""); response.Code != http.StatusConflict {
		t.Fatalf("report before completion status = %d, want %d", response.Code, http.StatusConflict)
	}

	for i := 0; i < 10; i++ {
		response := request(t, handler, http.MethodPost, "/api/answer", fmt.Sprintf(`{"questionId":%d,"answer":"Answer"}`, question.ID))
		if response.Code != http.StatusOK {
			t.Fatalf("answer %d status = %d; body = %q", i+1, response.Code, response.Body.String())
		}
		payload := decodeJSON[struct {
			NextQuestion *app.Question `json:"nextQuestion"`
		}](t, response)
		if payload.NextQuestion != nil {
			question = *payload.NextQuestion
		}
	}

	response := request(t, handler, http.MethodGet, "/api/report", "")
	if response.Code != http.StatusOK {
		t.Fatalf("report status = %d, want 200; body = %q", response.Code, response.Body.String())
	}
	report := decodeJSON[struct {
		FinalScore int `json:"finalScore"`
	}](t, response)
	if report.FinalScore != 88 {
		t.Fatalf("final score = %d, want 88", report.FinalScore)
	}
	if reportCalls.Load() != 2 {
		t.Fatalf("report Groq calls = %d, want 2", reportCalls.Load())
	}

	if wrongMethod := request(t, handler, http.MethodPost, "/api/report", `{}`); wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("report POST status = %d, want %d", wrongMethod.Code, http.StatusMethodNotAllowed)
	}
}
