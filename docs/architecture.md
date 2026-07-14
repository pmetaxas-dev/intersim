# Architecture

## Overview

Intersim 0.8 is a single-process, single-user Go web application. The browser
loads static assets from the Go server and calls JSON endpoints on the same
origin. The backend calls Groq for answer evaluation and report generation.

```text
Browser UI
   |
   | same-origin HTTP
   v
Go server (main.go)
   |-- static files (public/)
   `-- API (internal/app/)
          |
          `-- Groq chat-completions API
```

## Components

- `main.go` loads and validates questions, reads the optional environment key,
  constructs the application, and serves static and API routes.
- `internal/app/questions.go` owns question-file loading and validation.
- `internal/app/app.go` owns API routing and the in-memory interview state machine.
- `internal/app/groq.go` owns bounded, structured Groq requests and responses.
- `public/` owns preflight, interview interaction, feedback, and report rendering.
- `tests/` exercises the importable application through `httptest` servers.

## State model

An `App` contains one mutex-protected interview state. Starting an interview
replaces that state. A successful interview follows this sequence:

```text
start -> main 1 -> follow-up 1 -> ... -> main 5 -> follow-up 5 -> report
```

Failed Groq calls do not append history or advance the current question. The API
key selected at startup remains only in this in-memory state.

This design intentionally supports one local user. Version 1.0 should introduce
opaque session identifiers and isolated state before supporting concurrent users.

## External boundary

The Groq client uses the OpenAI-compatible chat-completions endpoint and requests
JSON objects from `llama-3.3-70b-versatile`. The endpoint, HTTP client, and model
are injectable through `app.Config`, allowing deterministic local tests.

## Error handling

- Invalid client data returns JSON with HTTP 400.
- Invalid state transitions return JSON with HTTP 409.
- Unsupported methods return JSON with HTTP 405 and an `Allow` header.
- Upstream failures or malformed AI data return JSON with HTTP 502.
- Startup configuration and question-file failures stop the server with a log.

