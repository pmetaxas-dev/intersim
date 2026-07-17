package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultGroqURL = "https://api.groq.com/openai/v1/chat/completions"
	defaultModel   = "llama-3.3-70b-versatile"
	maxRequestBody = 1 << 20
)

// App owns the single-user interview state and HTTP dependencies.
type App struct {
	mu          sync.Mutex
	questions   []Question
	fallbackKey string
	httpClient  *http.Client
	groqURL     string
	model       string
	logger      *log.Logger
	random      *rand.Rand
	state       *interviewState
}

// New validates configuration and constructs an interview application.
func New(config Config) (*App, error) {
	if err := validateQuestions(config.Questions); err != nil {
		return nil, err
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	groqURL := strings.TrimSpace(config.GroqURL)
	if groqURL == "" {
		groqURL = defaultGroqURL
	}
	model := strings.TrimSpace(config.Model)
	if model == "" {
		model = defaultModel
	}
	logger := config.Logger
	if logger == nil {
		logger = log.Default()
	}

	return &App{
		questions:   append([]Question(nil), config.Questions...),
		fallbackKey: strings.TrimSpace(config.FallbackKey),
		httpClient:  client,
		groqURL:     groqURL,
		model:       model,
		logger:      logger,
		random:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

// Handler returns the application's API routes.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/config", a.configHandler)
	mux.HandleFunc("/api/start", a.startInterviewHandler)
	mux.HandleFunc("/api/answer", a.answerHandler)
	mux.HandleFunc("/api/report", a.reportHandler)
	return noStore(mux)
}

// configHandler returns the API configuration for the frontend.
func (a *App) configHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"requiresApiKey": a.fallbackKey == ""})
}

// startInterviewHandler initializes a new interview session.
func (a *App) startInterviewHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var payload struct {
		APIKey string `json:"apiKey"`
	}
	if err := decodeJSONBody(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	apiKey := strings.TrimSpace(payload.APIKey)
	if apiKey == "" {
		apiKey = a.fallbackKey
	}
	if apiKey == "" {
		writeError(w, http.StatusBadRequest, "an API key is required")
		return
	}

	a.mu.Lock()
	permutation := a.random.Perm(len(a.questions))
	selected := make([]Question, interviewQuestionCount)
	for index := range selected {
		selected[index] = a.questions[permutation[index]]
	}
	a.state = &interviewState{
		questions: selected,
		history:   make([]interviewFeedback, 0, interviewQuestionCount*2),
		apiKey:    apiKey,
	}
	firstQuestion := selected[0]
	a.mu.Unlock()

	writeJSON(w, http.StatusOK, firstQuestion)
}

// answerHandler processes the user's answer and returns the next interview step.
func (a *App) answerHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var payload AnswerPayload
	if err := decodeJSONBody(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(payload.Answer) == "" {
		writeError(w, http.StatusBadRequest, "answer must not be empty")
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state == nil {
		writeError(w, http.StatusConflict, "start an interview first")
		return
	}
	if a.state.finished {
		writeError(w, http.StatusConflict, "the interview is already complete")
		return
	}

	currentQuestion := a.state.questions[a.state.currentIndex]
	contextLabel := "Main"
	if a.state.isFollowUpActive {
		currentQuestion = a.state.lastFollowUp
		contextLabel = "Follow-up"
	}
	if payload.QuestionID != currentQuestion.ID {
		writeError(w, http.StatusBadRequest, "questionId does not match the current question")
		return
	}

	evaluation, err := a.evaluateAnswer(r.Context(), a.state.apiKey, contextLabel, currentQuestion.Question, strings.TrimSpace(payload.Answer))
	if err != nil {
		a.logger.Printf("answer evaluation failed: %v", err)
		status, message := aiClientError(err, "evaluation")
		writeError(w, status, message)
		return
	}
	a.state.history = append(a.state.history, interviewFeedback{
		Question:     currentQuestion.Question,
		Score:        evaluation.Score,
		FeedbackGood: evaluation.FeedbackGood,
		FeedbackBad:  evaluation.FeedbackBad,
	})

	var nextQuestion *Question
	if len(a.state.history) >= interviewQuestionCount*2 {
		a.state.finished = true
	} else if !a.state.isFollowUpActive {
		a.state.isFollowUpActive = true
		a.state.lastFollowUp = Question{
			ID:       -(a.state.currentIndex + 1),
			Category: "Follow-up",
			Question: evaluation.FollowUpQuestion,
		}
		question := a.state.lastFollowUp
		nextQuestion = &question
	} else {
		a.state.isFollowUpActive = false
		a.state.currentIndex++
		question := a.state.questions[a.state.currentIndex]
		nextQuestion = &question
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"evaluation":   evaluation,
		"nextQuestion": nextQuestion,
		"isFinished":   a.state.finished,
	})
}

// reportHandler returns the final interview report once the session is complete.
func (a *App) reportHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state == nil || !a.state.finished {
		writeError(w, http.StatusConflict, "complete the interview before requesting a report")
		return
	}

	report, err := a.generateReport(r.Context(), a.state.apiKey, a.state.history)
	if err != nil {
		a.logger.Printf("report generation failed: %v", err)
		status, message := aiClientError(err, "report")
		writeError(w, status, message)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// requireMethod enforces the expected HTTP method for a request.
func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	return false
}

// decodeJSONBody parses and validates a single JSON request body.
func decodeJSONBody(r *http.Request, destination any) error {
	data, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBody+1))
	if err != nil {
		return fmt.Errorf("read request body: %w", err)
	}
	if len(data) > maxRequestBody {
		return fmt.Errorf("request body exceeds %d bytes", maxRequestBody)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("request body must contain one JSON object")
	}
	return nil
}

// writeError sends a JSON error response with the provided status.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// writeJSON serializes the response value as JSON and writes it.
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// noStore wraps a handler to disable client-side caching.
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// aiClientError maps Groq errors to an HTTP status code and user-friendly message.
func aiClientError(err error, operation string) (int, string) {
	var upstream *groqHTTPError
	if !errors.As(err, &upstream) {
		var transport *groqTransportError
		if errors.As(err, &transport) {
			return http.StatusServiceUnavailable, "Groq is temporarily unavailable. Please retry shortly."
		}
		return http.StatusBadGateway, "AI service returned an unexpected response. Please retry."
	}

	switch upstream.statusCode {
	case http.StatusUnauthorized:
		return http.StatusUnauthorized, "Groq API key was rejected. Reload and start with a valid key."
	case http.StatusForbidden:
		return http.StatusForbidden, "Groq denied permission to use the selected model."
	case http.StatusTooManyRequests:
		return http.StatusTooManyRequests, "Groq rate limit reached. Please retry shortly."
	default:
		if upstream.statusCode >= http.StatusInternalServerError {
			return http.StatusServiceUnavailable, "Groq is temporarily unavailable. Please retry shortly."
		}
		return http.StatusBadGateway, fmt.Sprintf("Groq rejected the %s request.", operation)
	}
}
