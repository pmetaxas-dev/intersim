# Zone01 Project Instructions
These instructions supplement the global Codex directives with rules specific
to this Go repository.

## Architecture and Scope
- 1st action is to read `docs/info.txt` for the exercise details,
  & `docs/audit.txt` for the audit prerequisites.
- Inspect the relevant source, `PRD.md`, and documentation before proposing
  implementation changes.
- Follow the architecture documented in `docs/architecture.md` unless the
  approved task explicitly changes it.
- Keep backend packages focused and preserve existing public behavior unless a
  change is explicitly approved.

## Go Coding Standards
- Format Go code with `gofmt` and follow idiomatic Go conventions.
- Prefer small, testable functions and descriptive names.
- Always implement one-line comments on important functions.
- All tests must be created inside the `/tests` folder.

## Test-Driven Workflow
For approved implementation work, follow Red → Green → Refactor when practical:
1. Add or update a failing test based on `PRD.md`, `docs/audit.txt`, or the
   approved requirement.
2. Implement the smallest correct change that passes the test.
3. Refactor without changing behavior.
4. Run the relevant tests, followed by `go test ./...` when appropriate.

## Documentation and Change Log
- Append every completed project change to `docs/changes_log.txt` using its
  existing `Date`, `Time`, `My Prompt`, and `Agent Action` format.
- Update `docs/architecture.md`, `docs/tasks.md`, or other affected documentation
  when an approved change makes it inaccurate.
- Record what changed, why it changed, and how it was verified.
