package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maxGroqResponseBytes = 1 << 20

const structuredResponseAttempts = 2

type groqHTTPError struct {
	statusCode int
}

func (e *groqHTTPError) Error() string {
	return fmt.Sprintf("Groq returned HTTP %d", e.statusCode)
}

type groqTransportError struct {
	cause error
}

func (e *groqTransportError) Error() string {
	return fmt.Sprintf("call Groq: %v", e.cause)
}

func (e *groqTransportError) Unwrap() error {
	return e.cause
}

type malformedGroqResponseError struct {
	cause error
}

func (e *malformedGroqResponseError) Error() string {
	return fmt.Sprintf("malformed Groq response: %v", e.cause)
}

func (e *malformedGroqResponseError) Unwrap() error {
	return e.cause
}

func (a *App) evaluateAnswer(ctx context.Context, apiKey, contextLabel, question, answer string) (AIResponse, error) {
	const systemPrompt = `You are an expert technical interviewer evaluating a Junior Developer.
1. Score the answer from 0 to 100. Correct, professional, accurate answers should receive 90-100.
2. Deduct points only for misinformation or important omissions.
3. For a main question, generate one specific technical follow-up. For a follow-up, return an empty followUpQuestion.
4. Return only JSON: {"score":int,"feedbackGood":"str","feedbackBad":"str","followUpQuestion":"str"}`
	userPrompt := fmt.Sprintf("Context: %s. Question: %s, Answer: %s", contextLabel, question, answer)

	var lastErr error
	for attempt := 0; attempt < structuredResponseAttempts; attempt++ {
		content, err := a.complete(ctx, apiKey, systemPrompt, userPrompt, 0.1)
		if err != nil {
			var malformed *malformedGroqResponseError
			if errors.As(err, &malformed) {
				lastErr = err
				continue
			}
			return AIResponse{}, err
		}

		result, err := parseEvaluation(content, contextLabel)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return AIResponse{}, fmt.Errorf("evaluation invalid after %d attempts: %w", structuredResponseAttempts, lastErr)
}

func parseEvaluation(content, contextLabel string) (AIResponse, error) {
	var result AIResponse
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return AIResponse{}, fmt.Errorf("decode evaluation: %w", err)
	}
	if result.Score < 0 || result.Score > 100 {
		return AIResponse{}, fmt.Errorf("evaluation score %d is outside 0-100", result.Score)
	}
	result.FeedbackGood = strings.TrimSpace(result.FeedbackGood)
	if result.FeedbackGood == "" {
		result.FeedbackGood = "No specific strengths were identified."
	}
	result.FeedbackBad = strings.TrimSpace(result.FeedbackBad)
	if result.FeedbackBad == "" {
		result.FeedbackBad = "No significant issues were identified."
	}
	result.FollowUpQuestion = strings.TrimSpace(result.FollowUpQuestion)
	if contextLabel == "Main" && result.FollowUpQuestion == "" {
		return AIResponse{}, fmt.Errorf("evaluation follow-up question is empty")
	}
	return result, nil
}

func (a *App) generateReport(ctx context.Context, apiKey string, history []interviewFeedback) (FinalReport, error) {
	data, err := json.Marshal(history)
	if err != nil {
		return FinalReport{}, fmt.Errorf("encode interview history: %w", err)
	}
	userPrompt := `Analyze the interview history and return only JSON: {"weaknesses":["str"],"studySuggestions":["str"],"finalScore":int}. History: ` + string(data)
	var lastErr error
	for attempt := 0; attempt < structuredResponseAttempts; attempt++ {
		content, err := a.complete(ctx, apiKey, "Generate interview report JSON.", userPrompt, 0.3)
		if err != nil {
			var malformed *malformedGroqResponseError
			if errors.As(err, &malformed) {
				lastErr = err
				continue
			}
			return FinalReport{}, err
		}

		report, err := parseReport(content)
		if err == nil {
			return report, nil
		}
		lastErr = err
	}
	return FinalReport{}, fmt.Errorf("report invalid after %d attempts: %w", structuredResponseAttempts, lastErr)
}

func parseReport(content string) (FinalReport, error) {
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
		return "", &groqTransportError{cause: err}
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		io.Copy(io.Discard, io.LimitReader(response.Body, maxGroqResponseBytes))
		return "", &groqHTTPError{statusCode: response.StatusCode}
	}

	var payload groqChatResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxGroqResponseBytes))
	if err := decoder.Decode(&payload); err != nil {
		return "", &malformedGroqResponseError{cause: fmt.Errorf("decode response: %w", err)}
	}
	if len(payload.Choices) == 0 || strings.TrimSpace(payload.Choices[0].Message.Content) == "" {
		return "", &malformedGroqResponseError{cause: fmt.Errorf("response has no content")}
	}
	return payload.Choices[0].Message.Content, nil
}
