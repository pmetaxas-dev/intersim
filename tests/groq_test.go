package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
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

func TestGroqFailuresDoNotAdvanceInterview(t *testing.T) {
	t.Parallel()

	call := 0
	application, _ := newTestApp(t, "env-key", func(w http.ResponseWriter, _ *http.Request) {
		call++
		switch call {
		case 1:
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		case 2:
			fmt.Fprint(w, `{"choices":[]}`)
		default:
			fmt.Fprint(w, evaluationResponse("Follow-up"))
		}
	})
	handler := application.Handler()
	start := request(t, handler, http.MethodPost, "/api/start", `{}`)
	question := decodeJSON[app.Question](t, start)

	for attempt := 1; attempt <= 2; attempt++ {
		response := request(t, handler, http.MethodPost, "/api/answer", fmt.Sprintf(`{"questionId":%d,"answer":"Answer"}`, question.ID))
		if response.Code != http.StatusBadGateway {
			t.Fatalf("attempt %d status = %d, want %d; body = %q", attempt, response.Code, http.StatusBadGateway, response.Body.String())
		}
	}

	response := request(t, handler, http.MethodPost, "/api/answer", fmt.Sprintf(`{"questionId":%d,"answer":"Answer"}`, question.ID))
	if response.Code != http.StatusOK {
		t.Fatalf("recovery status = %d, want 200; body = %q", response.Code, response.Body.String())
	}
}

func TestReportRequiresCompletedInterviewAndHandlesGroqResponse(t *testing.T) {
	t.Parallel()

	application, _ := newTestApp(t, "env-key", func(w http.ResponseWriter, r *http.Request) {
		var payload groqRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(payload.Messages) > 0 && payload.Messages[0].Content == "Generate interview report JSON." {
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

	if wrongMethod := request(t, handler, http.MethodPost, "/api/report", `{}`); wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("report POST status = %d, want %d", wrongMethod.Code, http.StatusMethodNotAllowed)
	}
}
