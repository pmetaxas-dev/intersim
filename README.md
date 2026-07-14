# Intersim

![Version](https://img.shields.io/badge/version-v0.9.0-orange) ![Go](https://img.shields.io/badge/Go-1.25.4-00ADD8?logo=go&logoColor=white) ![Tests](https://img.shields.io/badge/tests-passing-brightgreen) ![Coverage](https://img.shields.io/badge/coverage-90.3%25-brightgreen) ![License](https://img.shields.io/badge/license-Zone01%20Educational-6A9E32)

Intersim is a local AI-assisted job interview simulator for junior developers. It
selects five questions, asks one AI-generated follow-up after each answer, scores
the ten responses, and produces a final study report.

## Requirements

- Go 1.25.4 or a compatible newer Go release
- A Groq API key from [Groq](https://console.groq.com/keys)

No third-party Go modules are required.

## Run the application

From the repository root:

```powershell
go run .
```

Open <http://localhost:8080> in a browser.

The setup screen asks for a Groq API key when the server does not already have
one. The submitted key is kept only in server memory for the current interview;
it is not written to a file, browser storage, responses, or logs.

For local development, the key can instead be supplied through the environment:

```powershell
$env:GROQ_API_KEY = "gsk_..."
go run .
```

When `GROQ_API_KEY` exists, the browser field is optional. A key entered in the
browser takes precedence for the newly started interview.

## Interview flow

1. Start an interview with an entered key or the environment fallback.
2. Answer five randomly selected questions from `questions.json`.
3. Answer one generated follow-up after each main question.
4. Review the score, strengths, and weaknesses returned for each answer.
5. Receive a final score and study suggestions after ten responses.

Starting a new interview replaces the current in-memory interview. Version 0.8
is intentionally a single-user local application.

## Question data

`questions.json` must contain at least five entries with unique positive IDs and
non-empty categories and question text:

```json
[
  {
    "id": 1,
    "category": "Technical - Go",
    "question": "What is a goroutine?"
  }
]
```

`docs/hard_questions.json` is an additional question bank for future use and is
not loaded by version 0.8.

## API

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/config` | Reports whether the browser must provide a key. |
| `POST` | `/api/start` | Starts a new interview; accepts `{"apiKey":"..."}`. |
| `POST` | `/api/answer` | Evaluates the answer to the current question. |
| `GET` | `/api/report` | Generates the report after all ten answers. |

API responses are JSON and are marked `Cache-Control: no-store`.

## Verification

Run the full test suite:

```powershell
go test ./...
```

Additional useful checks:

```powershell
go test -race ./...
go vet ./...
go build ./...
```

Groq behavior is tested with local HTTP test servers, so tests do not use a real
API key or make external network calls.

## Troubleshooting

The application distinguishes rejected API keys, model permissions, rate limits,
temporary Groq availability, and malformed AI output. A malformed structured
response is retried once automatically without advancing the interview. If an API
key is rejected, reload the page and start a new interview with a valid key.

## Project layout

```text
main.go              Server entrypoint
internal/app/        Interview logic, HTTP handlers, and Groq client
public/              Browser interface
tests/               Backend and API tests
questions.json       Active interview question pool
docs/                Requirements, architecture, tasks, and future work
```

## License

This project is distributed under the [Zone01 Educational License](license.md).
