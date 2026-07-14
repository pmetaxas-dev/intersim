# Intersim 0.8 Product Requirements

## Product summary

Intersim is a single-user local web application that simulates a junior developer
interview. It evaluates answers with Groq, asks targeted follow-up questions, and
turns the completed interview into an actionable study report.

## Goals

- Provide a predictable five-question, ten-answer interview.
- Give immediate scores and concise constructive feedback.
- Generate one follow-up for each main question.
- Produce a final score, weaknesses, and study suggestions.
- Accept an API key without persisting or exposing it.
- Fail safely when requests, question data, or Groq responses are invalid.

## Version 0.8 scope

- Load and validate the built-in `questions.json` pool at startup.
- Randomly select five unique questions for each interview.
- Use a browser-entered Groq key or `GROQ_API_KEY` fallback.
- Keep the selected key and interview state in server memory only.
- Alternate main and follow-up questions until ten answers are evaluated.
- Generate the report only after the interview is complete.
- Serve the browser client and JSON API from the same Go process.
- Cover question loading, handlers, state transitions, and Groq failures with
  automated tests that do not use the external service.

## Out of scope

- Multiple simultaneous users or persistent sessions.
- Accounts, authentication, databases, or interview history persistence.
- Browser upload of custom question banks.
- Automatic use of `docs/hard_questions.json`.
- Verifying a key with Groq before the first submitted answer.

These items may be considered for version 1.0.

## Functional requirements

### Configuration preflight

1. The browser must request `GET /api/config` when the setup page loads.
2. The response must state whether `GROQ_API_KEY` is absent and a browser key is
   therefore required.
3. The response must never contain the configured key.

### Interview startup

1. `POST /api/start` accepts an optional `apiKey` string.
2. A non-empty submitted key takes precedence over the environment fallback.
3. Startup fails with HTTP 400 when neither key source is available.
4. Startup selects five unique questions and returns the first one.
5. Starting again replaces the previous single-user state.

### Answer evaluation

1. `POST /api/answer` requires the current `questionId` and a non-empty answer.
2. Requests made before startup, after completion, or for another question fail.
3. Groq returns a 0-100 score, positive feedback, improvement feedback, and a
   follow-up for main questions.
4. Invalid or unsuccessful Groq responses return HTTP 502 and do not advance the
   interview.
5. Main and follow-up questions alternate until ten successful evaluations.

### Final report

1. `GET /api/report` is available only after ten successful evaluations.
2. The report contains a 0-100 score, weaknesses, and study suggestions.
3. Invalid or unsuccessful Groq report responses return HTTP 502.

## Security and privacy requirements

- API keys must not be written to disk, browser storage, logs, or API responses.
- API responses must use `Cache-Control: no-store`.
- Untrusted AI text must be rendered with text nodes, not HTML injection.
- Request and upstream response bodies must have bounded read sizes.
- Tests must use mock HTTP servers and placeholder keys only.

## Acceptance criteria

- A user can complete the full interview with either supported key source.
- The browser makes the key field optional only when the server has a fallback.
- Five main questions and five follow-ups produce the final report.
- Invalid local and upstream inputs return controlled JSON errors without panic.
- `go test ./...`, `go vet ./...`, and `go build ./...` pass.

