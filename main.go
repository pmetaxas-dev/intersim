package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"
)

// Δομές Δεδομένων
type Question struct {
	ID       int    `json:"id"`
	Category string `json:"category"`
	Question string `json:"question"`
}

type AnswerPayload struct {
	QuestionID int    `json:"questionId"`
	Answer     string `json:"answer"`
}

type AIResponse struct {
	Score            int    `json:"score"`
	FeedbackGood     string `json:"feedbackGood"`
	FeedbackBad      string `json:"feedbackBad"`
	FollowUpQuestion string `json:"followUpQuestion"`
}

type InterviewState struct {
	TotalQuestions int
	CurrentIndex   int
	Questions      []Question
	History        []string // Κρατάει το ιστορικό για το τελικό report
	TotalScore     int
}

// Δομές για το Groq API (OpenAI Compatible Format)
type GroqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type GroqResponseFormat struct {
	Type string `json:"type"`
}

type GroqRequest struct {
	Model          string              `json:"model"`
	Messages       []GroqMessage       `json:"messages"`
	Temperature    float64             `json:"temperature"`
	ResponseFormat *GroqResponseFormat `json:"response_format,omitempty"`
}

type GroqChoice struct {
	Message GroqMessage `json:"message"`
}

type GroqChatResponse struct {
	Choices []GroqChoice `json:"choices"`
}

var (
	questionPool []Question
	state        *InterviewState
	groqAPIKey   string
)

const (
	groqAPIURL = "https://api.groq.com/openai/v1/chat/completions"
	groqModel  = "llama-3.3-70b-versatile" // Το κορυφαίο, γρήγορο μοντέλο της Groq
)

func main() {
	// Αρχικοποίηση Groq API Key
	groqAPIKey = os.Getenv("GROQ_API_KEY")
	if groqAPIKey == "" {
		// Αν δεν έχει οριστεί στα env, εμφάνισε λάθος
		log.Fatal("Missing GROQ_API_KEY Environment Variable!")
	}

	// Φόρτωση ερωτήσεων από το JSON
	loadQuestions()

	// Handlers για το API και το Front-end
	http.Handle("/", http.FileServer(http.Dir("./public")))
	http.HandleFunc("/api/start", startInterviewHandler)
	http.HandleFunc("/api/answer", answerHandler)
	http.HandleFunc("/api/report", generateReportHandler)

	fmt.Println("Server running on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func loadQuestions() {
	file, err := os.ReadFile("questions.json")
	if err != nil {
		log.Fatalf("Error reading questions file: %v", err)
	}
	if err := json.Unmarshal(file, &questionPool); err != nil {
		log.Fatalf("Error parsing JSON: %v", err)
	}
}

// Ξεκινάει μια νέα συνέντευξη επιλέγοντας τυχαίες ερωτήσεις
func startInterviewHandler(w http.ResponseWriter, r *http.Request) {
	count := 5 // Ή 10 ανάλογα με την επιλογή του χρήστη

	// Ανακάτεμα και επιλογή ερωτήσεων
	rSource := rand.New(rand.NewSource(time.Now().UnixNano()))
	perm := rSource.Perm(len(questionPool))

	selected := make([]Question, count)
	for i := 0; i < count; i++ {
		selected[i] = questionPool[perm[i]]
	}

	state = &InterviewState{
		TotalQuestions: count,
		CurrentIndex:   0,
		Questions:      selected,
		History:        []string{},
		TotalScore:     0,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state.Questions[0])
}

// Διαχειρίζεται την απάντηση και καλεί το Groq API
func answerHandler(w http.ResponseWriter, r *http.Request) {
	if state == nil || state.CurrentIndex >= state.TotalQuestions {
		log.Println("[WARN] Attempted to submit answer with no active session or session completed.")
		http.Error(w, "No active interview session", http.StatusBadRequest)
		return
	}

	var payload AnswerPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Printf("[ERROR] Failed to decode request body: %v", err)
		http.Error(w, fmt.Sprintf("Invalid request payload: %v", err), http.StatusBadRequest)
		return
	}

	currentQ := state.Questions[state.CurrentIndex]

	// Σχεδιασμός του Prompt με Structure Output (JSON)
	systemPrompt := `You are an expert tech interviewer. Evaluate the candidate's answer based on the question provided.
You MUST respond with a valid, raw JSON object. Do not include any markdown styling, explanation, or wrap it in triple backticks.
The JSON must have this exact structure:
{
    "score": (integer from 1 to 100),
    "feedbackGood": "What they did well in 1-2 sentences",
    "feedbackBad": "What they missed or could improve in 1-2 sentences",
    "followUpQuestion": "A natural follow-up question strictly based on their answer to deep dive into their knowledge"
}`

	userPrompt := fmt.Sprintf("Question: \"%s\"\nCandidate Answer: \"%s\"", currentQ.Question, payload.Answer)

	log.Printf("[INFO] Sending question evaluation to Groq for index %d...", state.CurrentIndex)

	// Προετοιμασία Groq API Request
	groqReqBody := GroqRequest{
		Model: groqModel,
		Messages: []GroqMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature:    0.2,
		ResponseFormat: &GroqResponseFormat{Type: "json_object"},
	}

	responseText, err := callGroqAPI(groqReqBody)
	if err != nil {
		log.Printf("[ERROR] Groq API call failed: %v", err)
		http.Error(w, fmt.Sprintf("AI evaluation failed: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("[DEBUG] Raw Groq response text: %s", responseText)

	var aiResult AIResponse
	if err := json.Unmarshal([]byte(responseText), &aiResult); err != nil {
		log.Printf("[ERROR] Failed to parse Groq response JSON: %v. Raw body was: %s", err, responseText)
		http.Error(w, fmt.Sprintf("Failed to parse AI response: %v", err), http.StatusInternalServerError)
		return
	}

	// Αποθήκευση στο ιστορικό για το τελικό report
	state.TotalScore += aiResult.Score
	state.History = append(state.History, fmt.Sprintf("Q: %s\nA: %s\nScore: %d\n", currentQ.Question, payload.Answer, aiResult.Score))

	state.CurrentIndex++

	// Προετοιμασία της επόμενης ερώτησης (αν υπάρχει)
	var nextQ *Question
	if state.CurrentIndex < state.TotalQuestions {
		nextQ = &state.Questions[state.CurrentIndex]
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"evaluation":   aiResult,
		"nextQuestion": nextQ,
		"isFinished":   nextQ == nil,
	}); err != nil {
		log.Printf("[ERROR] Failed to encode response: %v", err)
	}
}

// Δημιουργεί το τελικό Report μέσω Groq
func generateReportHandler(w http.ResponseWriter, r *http.Request) {
	if state == nil {
		http.Error(w, "No session data found", http.StatusBadRequest)
		return
	}

	avgScore := state.TotalScore / state.TotalQuestions

	historyText := ""
	for _, h := range state.History {
		historyText += h + "\n---\n"
	}

	systemPrompt := `Analyze this complete job interview history and compile a final feedback report.
You MUST respond with a valid, raw JSON object. Do not include any markdown styling, explanation, or wrap it in triple backticks.
The JSON must have this exact structure:
{
    "weaknesses": ["list", "of", "core", "weaknesses", "identified"],
    "studySuggestions": ["concrete", "topics", "or", "areas", "to", "study", "for", "improvement"]
}`

	userPrompt := fmt.Sprintf("The candidate achieved an average score of %d/100.\n\nInterview History:\n%s", avgScore, historyText)

	log.Printf("[INFO] Sending history analysis to Groq for final report...")

	groqReqBody := GroqRequest{
		Model: groqModel,
		Messages: []GroqMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature:    0.3,
		ResponseFormat: &GroqResponseFormat{Type: "json_object"},
	}

	responseText, err := callGroqAPI(groqReqBody)
	if err != nil {
		log.Printf("[ERROR] Groq API call failed: %v", err)
		http.Error(w, "Failed to generate final report", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(responseText))
}

// Helper συνάρτηση για την εκτέλεση HTTP Request στο API της Groq
func callGroqAPI(reqBody GroqRequest) (string, error) {
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", groqAPIURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+groqAPIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("groq API returned status %s: %s", resp.Status, string(respBody))
	}

	var groqResp GroqChatResponse
	if err := json.Unmarshal(respBody, &groqResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal groq response: %w", err)
	}

	if len(groqResp.Choices) == 0 {
		return "", fmt.Errorf("received empty choices from Groq")
	}

	return groqResp.Choices[0].Message.Content, nil
}
