// Package app implements the interview simulator HTTP application.
package app

import "net/http"

// Question is a single interview question.
type Question struct {
	ID       int    `json:"id"`
	Category string `json:"category"`
	Question string `json:"question"`
}

// AnswerPayload is the request body accepted by the answer endpoint.
type AnswerPayload struct {
	QuestionID int    `json:"questionId"`
	Answer     string `json:"answer"`
}

// AIResponse is the structured evaluation returned by Groq.
type AIResponse struct {
	Score            int    `json:"score"`
	FeedbackGood     string `json:"feedbackGood"`
	FeedbackBad      string `json:"feedbackBad"`
	FollowUpQuestion string `json:"followUpQuestion"`
}

// FinalReport is the final structured interview report.
type FinalReport struct {
	Weaknesses       []string `json:"weaknesses"`
	StudySuggestions []string `json:"studySuggestions"`
	FinalScore       int      `json:"finalScore"`
}

// Config contains the dependencies needed by an App.
type Config struct {
	Questions   []Question
	FallbackKey string
	HTTPClient  *http.Client
	GroqURL     string
	Model       string
}

type interviewFeedback struct {
	Question     string `json:"question"`
	Score        int    `json:"score"`
	FeedbackGood string `json:"feedbackGood"`
	FeedbackBad  string `json:"feedbackBad"`
}

type interviewState struct {
	currentIndex     int
	questions        []Question
	history          []interviewFeedback
	isFollowUpActive bool
	lastFollowUp     Question
	apiKey           string
	finished         bool
}

type groqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type groqRequest struct {
	Model          string             `json:"model"`
	Messages       []groqMessage      `json:"messages"`
	Temperature    float64            `json:"temperature"`
	ResponseFormat groqResponseFormat `json:"response_format"`
}

type groqResponseFormat struct {
	Type string `json:"type"`
}

type groqChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}
