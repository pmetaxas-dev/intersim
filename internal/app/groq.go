package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maxGroqResponseBytes = 1 << 20

func (a *App) evaluateAnswer(ctx context.Context, apiKey, contextLabel, question, answer string) (AIResponse, error) {
	const systemPrompt = `You are an expert technical interviewer evaluating a Junior Developer.
1. Score the answer from 0 to 100. Correct, professional, accurate answers should receive 90-100.
2. Deduct points only for misinformation or important omissions.
3. For a main question, generate one specific technical follow-up. For a follow-up, return an empty followUpQuestion.
4. Return only JSON: {"score":int,"feedbackGood":"str","feedbackBad":"str","followUpQuestion":"str"}`
	userPrompt := fmt.Sprintf("Context: %s. Question: %s, Answer: %s", contextLabel, question, answer)

	content, err := a.complete(ctx, apiKey, systemPrompt, userPrompt, 0.1)
	if err != nil {
		return AIResponse{}, err
	}

	var result AIResponse
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return AIResponse{}, fmt.Errorf("decode evaluation: %w", err)
	}
	if result.Score < 0 || result.Score > 100 {
		return AIResponse{}, fmt.Errorf("evaluation score %d is outside 0-100", result.Score)
	}
	if strings.TrimSpace(result.FeedbackGood) == "" || strings.TrimSpace(result.FeedbackBad) == "" {
		return AIResponse{}, fmt.Errorf("evaluation feedback is incomplete")
	}
	if contextLabel == "Main" && strings.TrimSpace(result.FollowUpQuestion) == "" {
		return AIResponse{}, fmt.Errorf("evaluation follow-up question is empty")
	}
	return result, nil
}

func (a *App) generateReport(ctx context.Context, apiKey string, history []interviewFeedback) (FinalReport, error) {
	data, err := json.Marshal(history)
	if err != nil {
		return FinalReport{}, fmt.Errorf("encode interview history: %w", err)
	}
	content, err := a.complete(ctx, apiKey, "Generate interview report JSON.",
		`Analyze the interview history and return only JSON: {"weaknesses":["str"],"studySuggestions":["str"],"finalScore":int}. History: `+string(data), 0.3)
	if err != nil {
		return FinalReport{}, err
	}

	var report FinalReport
	if err := json.Unmarshal([]byte(content), &report); err != nil {
		return FinalReport{}, fmt.Errorf("decode report: %w", err)
	}
	if report.FinalScore < 0 || report.FinalScore > 100 {
		return FinalReport{}, fmt.Errorf("report score %d is outside 0-100", report.FinalScore)
	}
	if report.Weaknesses == nil || report.StudySuggestions == nil {
		return FinalReport{}, fmt.Errorf("report lists are missing")
	}
	return report, nil
}

func (a *App) complete(ctx context.Context, apiKey, systemPrompt, userPrompt string, temperature float64) (string, error) {
	body, err := json.Marshal(groqRequest{
		Model: a.model,
		Messages: []groqMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature:    temperature,
		ResponseFormat: groqResponseFormat{Type: "json_object"},
	})
	if err != nil {
		return "", fmt.Errorf("encode Groq request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.groqURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create Groq request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	response, err := a.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call Groq: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		io.Copy(io.Discard, io.LimitReader(response.Body, maxGroqResponseBytes))
		return "", fmt.Errorf("Groq returned HTTP %d", response.StatusCode)
	}

	var payload groqChatResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxGroqResponseBytes))
	if err := decoder.Decode(&payload); err != nil {
		return "", fmt.Errorf("decode Groq response: %w", err)
	}
	if len(payload.Choices) == 0 || strings.TrimSpace(payload.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("Groq response has no content")
	}
	return payload.Choices[0].Message.Content, nil
}
