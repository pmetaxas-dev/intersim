package tests

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"intersim/internal/app"
)

func testQuestions() []app.Question {
	questions := make([]app.Question, 6)
	for i := range questions {
		questions[i] = app.Question{
			ID:       i + 1,
			Category: "Technical - Go",
			Question: "Question " + string(rune('A'+i)),
		}
	}
	return questions
}

func newTestApp(t *testing.T, fallbackKey string, groqHandler http.HandlerFunc) (*app.App, *httptest.Server) {
	t.Helper()

	server := httptest.NewServer(groqHandler)
	t.Cleanup(server.Close)

	application, err := app.New(app.Config{
		Questions:   testQuestions(),
		FallbackKey: fallbackKey,
		HTTPClient:  server.Client(),
		GroqURL:     server.URL,
		Logger:      log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	return application, server
}

func request(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func decodeJSON[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()

	var value T
	if err := json.NewDecoder(recorder.Body).Decode(&value); err != nil {
		t.Fatalf("decode response: %v; body = %q", err, recorder.Body.String())
	}
	return value
}
