# Tasks

## Version 0.8

- [x] Validate the built-in question pool at startup.
- [x] Add automated backend and API coverage under `/tests`.
- [x] Extract testable application logic from the executable entrypoint.
- [x] Add API-key configuration preflight.
- [x] Accept a browser key without persistence and retain environment fallback.
- [x] Preserve five main questions with one follow-up each.
- [x] Handle invalid requests and Groq failures without advancing state.
- [x] Normalize empty AI feedback, retry malformed structured output once, and
  return actionable Groq error categories without logging secrets.
- [x] Populate the README and create the product requirements document.
- [x] Document architecture, security boundaries, and verification commands.

## Version 1.0 candidates

- [ ] Replace global single-user state with isolated per-user sessions.
- [ ] Add validated browser upload of a custom JSON question bank.
