package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"
)

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

type FinalReport struct {
	Weaknesses       []string `json:"weaknesses"`
	StudySuggestions []string `json:"studySuggestions"`
	FinalScore       int      `json:"finalScore"`
}

type InterviewState struct {
	CurrentIndex     int
	Questions        []Question
	History          []string
	IsFollowUpActive bool
	LastFollowUpQ    string
}

type GroqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type GroqRequest struct {
	Model          string              `json:"model"`
	Messages       []GroqMessage       `json:"messages"`
	Temperature    float64             `json:"temperature"`
	ResponseFormat *GroqResponseFormat `json:"response_format"`
}

type GroqResponseFormat struct {
	Type string `json:"type"`
}

type GroqChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

var (
	questionPool []Question
	state        *InterviewState
	groqAPIKey   = os.Getenv("GROQ_API_KEY")
)

func main() {
	loadQuestions()
	http.Handle("/", http.FileServer(http.Dir("./public")))
	http.HandleFunc("/api/start", startInterviewHandler)
	http.HandleFunc("/api/answer", answerHandler)
	http.HandleFunc("/api/report", generateReportHandler)
	log.Println("Server running on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func loadQuestions() {
	file, _ := os.ReadFile("questions.json")
	json.Unmarshal(file, &questionPool)
}

func startInterviewHandler(w http.ResponseWriter, r *http.Request) {
	perm := rand.New(rand.NewSource(time.Now().UnixNano())).Perm(len(questionPool))
	selected := []Question{questionPool[perm[0]], questionPool[perm[1]], questionPool[perm[2]], questionPool[perm[3]], questionPool[perm[4]]}
	state = &InterviewState{CurrentIndex: 0, Questions: selected, History: []string{}, IsFollowUpActive: false}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state.Questions[0])
}

func answerHandler(w http.ResponseWriter, r *http.Request) {
	var payload AnswerPayload
	json.NewDecoder(r.Body).Decode(&payload)

	qText := state.Questions[state.CurrentIndex].Question
	if state.IsFollowUpActive {
		qText = state.LastFollowUpQ
	}

	systemPrompt := `You are an expert technical interviewer evaluating a Junior Developer.
    1. Score the answer on a scale of 0-100. Be fair: a correct, professional, and accurate technical answer should receive 90-100. 
    2. Only deduct points for actual misinformation or critical omissions. Do not penalize for "style" or "length" unless it is completely irrelevant.
    3. IF this is a main question, generate ONE specific, technical follow-up. IF follow-up, set "followUpQuestion" to "".
    4. Return ONLY valid JSON: {"score": int, "feedbackGood": "str", "feedbackBad": "str", "followUpQuestion": "str"}`

	reqBody := GroqRequest{
		Model: "llama-3.3-70b-versatile",
		Messages: []GroqMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: fmt.Sprintf("Context: %s. Question: %s, Answer: %s",
				map[bool]string{true: "Follow-up", false: "Main"}[state.IsFollowUpActive], qText, payload.Answer)},
		},
		Temperature:    0.1,
		ResponseFormat: &GroqResponseFormat{Type: "json_object"},
	}

	data, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://api.groq.com/openai/v1/chat/completions", bytes.NewBuffer(data))
	req.Header.Set("Authorization", "Bearer "+groqAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		log.Println("Error calling Groq:", err)
		http.Error(w, "AI Service Error", http.StatusInternalServerError)
		return
	}

	// Τώρα το defer είναι ασφαλές
	defer resp.Body.Close()

	var groqResp GroqChatResponse
	json.NewDecoder(resp.Body).Decode(&groqResp)
	var aiResult AIResponse
	json.Unmarshal([]byte(groqResp.Choices[0].Message.Content), &aiResult)

	// Αποθήκευση ουσιαστικού feedback στο history
	feedbackSummary := fmt.Sprintf("Q: %s, Score: %d, Pros: %s, Cons: %s", qText, aiResult.Score, aiResult.FeedbackGood, aiResult.FeedbackBad)
	state.History = append(state.History, feedbackSummary)

	isFinished := len(state.History) >= 10
	var nextQ *Question
	if !isFinished {
		if !state.IsFollowUpActive {
			state.IsFollowUpActive = true
			state.LastFollowUpQ = aiResult.FollowUpQuestion
			nextQ = &Question{ID: 999, Category: "Follow-up", Question: aiResult.FollowUpQuestion}
		} else {
			state.IsFollowUpActive = false
			state.CurrentIndex++
			if state.CurrentIndex < len(state.Questions) {
				nextQ = &state.Questions[state.CurrentIndex]
			} else {
				isFinished = true
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"evaluation": aiResult, "nextQuestion": nextQ, "isFinished": isFinished})
}

func generateReportHandler(w http.ResponseWriter, r *http.Request) {
	reportContext := ""
	for _, h := range state.History {
		reportContext += h + "\n"
	}

	systemPrompt := `Analyze the interview feedback history and generate JSON: {"weaknesses": ["str"], "studySuggestions": ["str"], "finalScore": int}`
	reqBody := GroqRequest{
		Model: "llama-3.3-70b-versatile",
		Messages: []GroqMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: reportContext},
		},
		Temperature:    0.3,
		ResponseFormat: &GroqResponseFormat{Type: "json_object"},
	}

	data, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://api.groq.com/openai/v1/chat/completions", bytes.NewBuffer(data))
	req.Header.Set("Authorization", "Bearer "+groqAPIKey)
	req.Header.Set("Content-Type", "application/json")

	// ΑΛΛΑΞΕ ΤΟ: resp, _ := http.DefaultClient.Do(req)
	// ΣΕ ΑΥΤΟ (για να πιάνεις το err):
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Println("Error calling Groq API:", err)
		http.Error(w, "AI Service Unavailable", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close() // Τώρα είναι ασφαλές γιατί ξέρουμε ότι resp != nil

	var groqResp GroqChatResponse
	json.NewDecoder(resp.Body).Decode(&groqResp)
	var report FinalReport
	json.Unmarshal([]byte(groqResp.Choices[0].Message.Content), &report)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}
