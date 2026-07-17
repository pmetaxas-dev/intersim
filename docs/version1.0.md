# Intersim 1.0 Product Requirements

## 1. Product Summary

Intersim is a single-user local web application for configurable technical interview practice.

The user chooses an interview category, target role, and number of main questions, with optional context to narrow the focus. Groq generates the complete main-question set at startup, evaluates answers, creates follow-ups when required, and produces a final study report.

Version 1.0 removes the production dependency on `questions.json` while preserving the existing in-memory interview flow.

---

## 2. Goals

- Provide a concise Setup Page.
- Generate 5, 10, or 15 main questions in one Groq request.
- Adapt question depth to the selected target role.
- Use optional context to focus the interview.
- Give immediate scores and constructive feedback.
- Generate one follow-up when a main answer scores below 100.
- Award one star and skip the follow-up when a main answer scores 100/100.
- Display earned stars in a **Hall of Stars** section.
- Produce a final score, strengths, weaknesses, and study suggestions.
- Keep API keys and interview state in server memory only.

## 3. Out of Scope

- Multiple users, accounts, databases, or interview history.
- Recovery after server restart.
- Uploaded question banks.
- Voice or video interviews.
- More than one follow-up per main question.
- Persistent storage of questions, answers, or stars.

---

## 4. Setup Page

The Setup Page asks for:

1. **Interview type**
2. **Target role**
3. **Number of main questions**
4. **Additional context** — optional
5. **Groq API key** — only when no environment fallback exists

The **Start Interview** button remains disabled until required values are valid.

### Interview categories

| Category | Scope |
|---|---|
| **Artificial Intelligence** | Machine learning, deep learning, natural language processing, and computer vision. |
| **Blockchain & Crypto** | Smart contracts, tokens, and on-chain systems using Solidity and Ethereum. |
| **Cloud & DevOps** | Networks, servers, containers, Kubernetes, and cloud automation. |
| **Cybersecurity** | Offensive security, penetration testing, reverse engineering, and web exploitation. |
| **Java Full-Stack Development** | Spring Boot backends, Angular frontends, and a complete DevOps toolchain. |
| **Mobile Application Development** | Responsive iOS and Android applications using Dart and Flutter. |
| **Video Game Development** | Gameplay programming in Unreal Engine 5 using Blueprints and C++. |

Stored values:

- `artificial-intelligence`
- `blockchain-crypto`
- `cloud-devops`
- `cybersecurity`
- `java-full-stack`
- `mobile-development`
- `game-development`

### Target roles

Target role replaces a separate difficulty setting.

| Role | Expected depth |
|---|---|
| **Internship** | Fundamentals, reasoning, and learning potential. |
| **Junior** | Fundamentals and basic practical application. |
| **Mid-level** | Independent implementation, debugging, and trade-offs. |
| **Senior** | Architecture, scalability, leadership, and complex trade-offs. |

Stored values: `internship`, `junior`, `mid-level`, `senior`.

### Interview length

Supported values are `5`, `10`, or `15` main questions.

The final answer count is variable:

```text
minimum = main questions
maximum = main questions * 2
actual  = main questions + required follow-ups
```

### Additional context

Additional context is optional free text used in the question-generation prompt.

Examples:

- Focus on PyTorch and model evaluation.
- Include Docker, Linux, and CI/CD.
- Avoid database questions.

The server must trim whitespace, enforce a 500-character limit, and treat the value as untrusted input. It may focus the interview but cannot override the selected category, role, question count, or required output format.

---

## 5. Interview Flow

1. Load server configuration.
2. Submit the Setup Page.
3. Generate and validate all main questions in one Groq request.
4. Present and evaluate one main question.
5. Apply the score rule:
   - `100`: award one star, skip the follow-up, and advance.
   - `< 100`: generate and evaluate one follow-up, then advance.
6. Continue until all main questions and required follow-ups are complete.
7. Generate the final report.

Only main-question scores of exactly `100` award stars. Follow-up scores never award stars.

---

## 6. API Requirements

### `GET /api/config`

Returns whether a browser API key is required. It must never expose the configured key and must use `Cache-Control: no-store`.

### `POST /api/start`

Example request:

```json
{
  "apiKey": "optional-browser-key",
  "interviewType": "cloud-devops",
  "targetRole": "junior",
  "questionCount": 10,
  "additionalContext": "Focus on Docker, Linux, and CI/CD."
}
```

Requirements:

- A submitted key overrides `GROQ_API_KEY`.
- Reject missing key sources or invalid setup values.
- Generate all main questions in one request.
- Validate the complete result before creating state.
- Failed startup must leave no partial interview.
- Starting again replaces the previous state.
- Return only the first question and public progress data.

### `POST /api/answer`

Requires the current `questionId` and a non-empty answer.

Reject missing interviews, completed interviews, stale question IDs, oversized answers, and invalid state transitions.

A successful response includes the score, feedback, star result, total stars, progress, and either the next question or completion status.

### `GET /api/report`

Available only after completion. The report includes:

- Overall score.
- Selected category, role, and interview length.
- Strengths, weaknesses, and study suggestions.
- Topic observations when enough evidence exists.
- Total stars and Hall of Stars entries.

Invalid Groq report output returns HTTP 502.

---

## 7. Question Generation

Groq must return exactly the selected number of structured main questions. Each question contains:

- A unique identifier.
- Question text.
- A topic or competency label.

Questions must match the selected category, role, and relevant additional context. They must avoid duplicates and must not expose answers, hints, or grading criteria.

Malformed output, incorrect counts, missing fields, duplicate IDs, or duplicate questions are retried once. A second failure returns a controlled startup error.

---

## 8. Evaluation and Hall of Stars

A main-answer evaluation returns:

- Score from 0 to 100.
- Positive feedback.
- Improvement feedback.
- A follow-up only when the score is below 100.

Rules:

- `score == 100`: award one star and ignore any returned follow-up.
- `score < 100`: require exactly one valid follow-up.
- Failed evaluations do not advance state or award stars.
- Repeated submissions cannot duplicate stars.
- Follow-up evaluations return score and feedback only.

The interview page must contain a **Hall of Stars** section. Each entry represents one perfect main answer and contains its topic, short question text or safe summary, and position in the interview.

The section must show an empty state before the first award, update only after server confirmation, display the total star count, and appear in the final report.

---

## 9. State and Completion

Server memory stores:

- Interview configuration and generated questions.
- Current question index and expected question type.
- Answers, evaluations, and follow-ups.
- Awarded stars and completion state.
- The resolved API key.

Completion requires every main question and every required follow-up to be evaluated successfully. It must not assume `questionCount * 2` answers.

Questions, answers, additional context, stars, and keys must not be written to `questions.json`, browser storage, logs, or runtime files.

---

## 10. Security and Reliability

- Never persist, log, or return API keys.
- Use `Cache-Control: no-store` for API responses.
- Render AI text as text, not HTML.
- Bound request and upstream response sizes.
- Configure explicit HTTP timeouts.
- Parse Groq output into strict typed structures.
- Keep startup atomic and state transitions deterministic.
- Preserve state after failed evaluations.
- Make star awards idempotent per main question.
- Exclude answers, context, and full AI output from logs.
- Use mock servers and placeholder keys in tests.

---

## 11. Testing Requirements

Tests must cover:

- Every category, role, and supported question count.
- Additional-context validation and prompt inclusion.
- Missing keys and key precedence.
- Correct generation of 5, 10, and 15 questions.
- Wrong counts, duplicates, malformed output, retries, and timeouts.
- Scores below 100 creating exactly one follow-up.
- Scores of 100 skipping follow-ups and awarding exactly one star.
- Follow-up scores never awarding stars.
- Failed or repeated requests not duplicating stars.
- Completion with zero, some, or all perfect main answers.
- Accurate Hall of Stars data in the final report.
- Rejection of stale or out-of-order submissions.
- Safe handling of oversized input and HTML injection attempts.

Tests must not call the external Groq service.

---

## 12. Migration from Version 0.8

Remove:

- Production loading of `questions.json`.
- Random selection of five local questions.
- Fixed assumptions of five main questions and ten answers.
- Unconditional follow-up generation.

Retain:

- Single-user in-memory state.
- Browser API key with environment fallback.
- Immediate scoring and feedback.
- Dynamic follow-up generation.
- Final report generation.
- Safe Groq error mapping and one retry for malformed output.
- Mock-based tests and same-process browser/API serving.

---

## 13. Acceptance Criteria

Version 1.0 is complete when:

- Users can select one of seven categories, one target role, and 5, 10, or 15 main questions.
- Optional context of up to 500 characters is supported.
- All main questions are generated and validated before the interview begins.
- Failed startup creates no partial state.
- A score below 100 creates exactly one follow-up.
- A score of 100 creates no follow-up and awards exactly one star.
- Hall of Stars updates only after server confirmation.
- Retries and repeated submissions cannot duplicate stars.
- Completion works for every combination of perfect and non-perfect answers.
- The final report reflects the setup and accurate star data.
- Sensitive values and interview content are not persisted or logged.
- Invalid local or upstream input returns controlled JSON errors without panic.
- `go test ./...`, `go vet ./...`, and `go build ./...` pass.